// Package idle monitors user inactivity via Wayland ext_idle_notifier_v1
// and triggers screen locking, display power management, and optional suspend.
//
// Inhibition is tracked from three independent sources, each updating its own
// flag. A computed inhibited state is derived by OR-ing all flags. This
// prevents any single source from overriding another.
//
// Lock ordering: waylandMu → mu.  recomputeInhibition always releases mu
// before calling recreateTimers (which acquires waylandMu), so no deadlock.
package idle

import (
	"bufio"
	"context"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/rajveermalviya/go-wayland/wayland/client"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprland"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/idle/dbusutil"
	protocol "github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/idle/protocol"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/waylandutil"
)

// Config holds tunable idle parameters.
type Config struct {
	IdleTimeout           time.Duration // time before display turns off when unlocked
	LockDelay             time.Duration // delay after display off before locking (when unlocked)
	LockDisplayOffTimeout time.Duration // idle time before display off when already locked
	SuspendTimeout        time.Duration

	QsConfigPath string // quickshell config path for IPC calls (e.g. /usr/share/snry-shell/frontend/ii)
}

// DefaultConfig returns the factory defaults matching snry-idle requirements.
func DefaultConfig() Config {
	return Config{
		IdleTimeout:           5 * time.Minute,
		LockDelay:             3 * time.Second,
		LockDisplayOffTimeout: 3 * time.Second,
		SuspendTimeout:        0,
	}
}

// Service detects user inactivity and triggers locking and optional suspend.
type Service struct {
	bus         *bus
	conn        dbusutil.DBusConn // system bus (logind)
	sessionConn dbusutil.DBusConn // session bus (ScreenSaver)
	cfg         Config
	hl          hyprland.API

	// mu protects source flags, config, callback pointers, and mutable state.
	// Lock ordering: waylandMu → mu.  Never hold mu while acquiring waylandMu.
	mu sync.Mutex

	// Inhibition source flags (protected by mu).
	screensaverInhibited bool
	logindIdleInhibited  bool
	fullscreenCount      int // number of currently fullscreened windows

	// Computed inhibition state — written under mu, readable atomically.
	inhibited atomic.Bool

	// Display power state — written in setDisplay, readable atomically.
	displayOn atomic.Bool

	// Other mutable state (protected by mu).
	idleStarted      time.Time
	lastLocked       bool
	displayOffForced bool
	lockCancel       context.CancelFunc
	onLockCallback   func()
	lockedProvider   func() bool
	onLogindUnlock   func()
	onDisplayChange  func(on bool)

	// Wayland fields (protected by waylandMu).
	waylandMu       sync.Mutex
	display         *client.Display
	manager         *protocol.ExtIdleNotifierV1
	seat            *client.Seat
	displayOffNotif *protocol.ExtIdleNotificationV1

	// test hooks
	onRecreateTimers   func()
	testResumedHandler func()
}

// New creates the idle service.
func New(sysConn dbusutil.DBusConn, sessionConn dbusutil.DBusConn, cfg Config, hl hyprland.API) *Service {
	s := &Service{
		bus:         newBus(),
		conn:        sysConn,
		sessionConn: sessionConn,
		cfg:         cfg,
		hl:          hl,
	}
	s.displayOn.Store(true)
	return s
}

// ── Inhibition recomputation ────────────────────────────────────────────────

// recomputeInhibition derives the inhibited state from all source flags and
// takes side-effects when the state transitions.  Must NOT be called with mu
// or waylandMu held.
func (s *Service) recomputeInhibition() {
	s.mu.Lock()
	newInhibited := s.screensaverInhibited || s.logindIdleInhibited || s.fullscreenCount > 0
	changed := newInhibited != s.inhibited.Load()
	wasDisplayOn := s.displayOn.Load()
	locked := s.isLocked()
	ss := s.screensaverInhibited
	li := s.logindIdleInhibited
	fc := s.fullscreenCount
	s.inhibited.Store(newInhibited)
	s.mu.Unlock()

	if !changed {
		return
	}

	log.Printf("[idle] inhibition: %v (screensaver=%v logind=%v fullscreen=%d)",
		newInhibited, ss, li, fc)

	// If inhibition activated while display is off and screen is not locked,
	// turn the display back on (e.g. video started playing via remote/cast).
	if newInhibited && !wasDisplayOn && !locked {
		s.setDisplay(true)
	}

	s.recreateTimers()
}

// ── Exported setters ────────────────────────────────────────────────────────

// SuppressDisplayOn prevents setDisplay(true) from running when on=true.
func (s *Service) SuppressDisplayOn(suppress bool) {
	s.mu.Lock()
	s.displayOffForced = suppress
	s.mu.Unlock()
}

func (s *Service) SetOnDisplayChange(fn func(on bool)) {
	s.mu.Lock()
	s.onDisplayChange = fn
	s.mu.Unlock()
}

// SetOnLock registers a callback invoked by doLock instead of publishing to the internal bus.
func (s *Service) SetOnLock(fn func()) {
	s.mu.Lock()
	s.onLockCallback = fn
	s.mu.Unlock()
}

// SetLockedProvider registers a function that returns the authoritative locked state.
func (s *Service) SetLockedProvider(fn func() bool) {
	s.mu.Lock()
	s.lockedProvider = fn
	s.mu.Unlock()
}

// SetOnLogindUnlock registers a callback invoked when logind sends an Unlock signal.
func (s *Service) SetOnLogindUnlock(fn func()) {
	s.mu.Lock()
	s.onLogindUnlock = fn
	s.mu.Unlock()
}

// SetOnRecreateTimers registers a test hook called at the end of every recreateTimers call.
func (s *Service) SetOnRecreateTimers(fn func()) {
	s.mu.Lock()
	s.onRecreateTimers = fn
	s.mu.Unlock()
}

// SetDisplay is the exported wrapper around setDisplay.
func (s *Service) SetDisplay(on bool) {
	s.setDisplay(on)
}

// ── Internal helpers ────────────────────────────────────────────────────────

// isLocked reads the locked provider. Must be called with mu held.
func (s *Service) isLocked() bool {
	if s.lockedProvider != nil {
		return s.lockedProvider()
	}
	return false
}

func (s *Service) setDPMS(action string) {
	if s.hl == nil {
		return
	}
	s.hl.SetDPMS(action)
}

// ── Timer management ────────────────────────────────────────────────────────

func (s *Service) recreateTimers() {
	s.waylandMu.Lock()
	defer s.waylandMu.Unlock()

	defer func() {
		if s.onRecreateTimers != nil {
			s.onRecreateTimers()
		}
	}()

	if s.manager == nil {
		return
	}

	if s.displayOffNotif != nil {
		s.displayOffNotif.Destroy()
		s.displayOffNotif = nil
	}

	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	locked := s.isLocked()

	if locked {
		// Locked: one timer at LockDisplayOffTimeout
		if cfg.LockDisplayOffTimeout > 0 {
			ms := uint32(cfg.LockDisplayOffTimeout.Milliseconds())
			notif, err := s.manager.GetIdleNotification(ms, s.seat)
			if err == nil {
				notif.SetIdledHandler(func(protocol.ExtIdleNotificationV1IdledEvent) {
					if s.inhibited.Load() {
						return
					}
					s.setDisplay(false)
				})
				resumed := func() {
					s.setDisplay(true)
					s.recreateTimers()
				}
				s.testResumedHandler = resumed
				notif.SetResumedHandler(func(protocol.ExtIdleNotificationV1ResumedEvent) {
					resumed()
				})
				s.displayOffNotif = notif
			}
		}
	} else {
		// Unlocked: one timer at IdleTimeout
		if cfg.IdleTimeout > 0 {
			ms := uint32(cfg.IdleTimeout.Milliseconds())
			notif, err := s.manager.GetIdleNotification(ms, s.seat)
			if err == nil {
				notif.SetIdledHandler(func(protocol.ExtIdleNotificationV1IdledEvent) {
					if s.inhibited.Load() {
						return
					}
					s.setDisplay(false)
					// Start cancellable lock goroutine
					var ctx context.Context
					s.mu.Lock()
					ctx, s.lockCancel = context.WithCancel(context.Background())
					s.mu.Unlock()
					go func() {
						select {
						case <-ctx.Done():
							return
						case <-time.After(cfg.LockDelay):
							s.doLock()
						}
					}()
				})
				resumed := func() {
					s.setDisplay(true)
					s.recreateTimers()
				}
				s.testResumedHandler = resumed
				notif.SetResumedHandler(func(protocol.ExtIdleNotificationV1ResumedEvent) {
					resumed()
				})
				s.displayOffNotif = notif
			}
		}
	}
}

func (s *Service) setDisplay(on bool) {
	s.mu.Lock()
	if on && s.displayOffForced {
		s.mu.Unlock()
		return
	}
	if on && s.lockCancel != nil {
		s.lockCancel()
		s.lockCancel = nil
	}
	s.mu.Unlock()

	// Update tracked state before issuing DPMS so recomputeInhibition sees it.
	s.displayOn.Store(on)

	if on {
		log.Printf("[idle] turning display ON")
		s.setDPMS("on")
	} else {
		log.Printf("[idle] turning display OFF")
		s.setDPMS("off")
	}

	s.mu.Lock()
	cb := s.onDisplayChange
	s.mu.Unlock()
	if cb != nil {
		cb(on)
	}
}

// ── Main entry point ────────────────────────────────────────────────────────

// Run starts monitoring. Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	// Start logind monitor on system bus
	if realConn, ok := s.conn.(*dbusutil.RealConn); ok && realConn.Conn != nil {
		go s.monitorLogind(ctx)
		go s.monitorLogindInhibit(ctx, realConn.Conn)

		// Inhibit logind from handling power button / lid switch
		_, err := dbusutil.LogindInhibit(realConn.Conn,
			"handle-power-key:handle-lid-switch",
			"snry-idle", "Shell handling system buttons", "block")
		if err != nil {
			log.Printf("[idle] logind inhibit: %v", err)
		} else {
			log.Printf("[idle] logind button handling inhibited")
		}
	}

	// Register ScreenSaver on session bus (where browsers/media players call Inhibit)
	if sessConn, ok := s.sessionConn.(*dbusutil.RealConn); ok && sessConn.Conn != nil {
		onChange := func(inhibited bool) {
			s.mu.Lock()
			s.screensaverInhibited = inhibited
			s.mu.Unlock()
			s.recomputeInhibition()
		}
		if err := registerScreenSaver(sessConn.Conn, newScreenSaver(onChange)); err != nil {
			log.Printf("[idle] screensaver dbus: %v", err)
		}
	}

	// Monitor Hyprland events for fullscreen state (event-driven, no polling)
	go s.monitorHyprlandEvents(ctx)

	go s.waylandLoop(ctx)

	// Suspend ticker
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.tick()
		}
	}
}

// ── Lock state management ───────────────────────────────────────────────────

// NotifyLockChanged re-reads the locked state from the provider and
// triggers side effects (display on, timer recreation) if the state changed.
func (s *Service) NotifyLockChanged() {
	locked := s.isLocked()

	s.mu.Lock()
	changed := s.lastLocked != locked
	s.lastLocked = locked
	if locked {
		s.idleStarted = time.Now()
	} else {
		s.idleStarted = time.Time{}
		s.displayOffForced = false // clear forced-off on unlock
	}
	s.mu.Unlock()

	if locked {
		// Turn display ON when locking so the lock screen is visible.
		s.setDisplay(true)
	} else {
		s.setDisplay(true)
	}
	if changed {
		log.Printf("[idle] lock state: %v", locked)
		s.recreateTimers()
	}
}

func (s *Service) tick() {
	if s.inhibited.Load() {
		return
	}

	s.mu.Lock()
	cfg := s.cfg
	started := s.idleStarted
	s.mu.Unlock()
	locked := s.isLocked()

	if !locked || started.IsZero() {
		return
	}

	elapsed := time.Since(started)
	if cfg.SuspendTimeout > 0 && elapsed >= cfg.SuspendTimeout {
		s.mu.Lock()
		s.idleStarted = time.Time{}
		s.mu.Unlock()
		log.Printf("[idle] suspending system")
		go func() {
			if realConn, ok := s.conn.(*dbusutil.RealConn); ok && realConn.Conn != nil {
				dbusutil.LogindSuspend(realConn.Conn)
			}
		}()
	}
}

func (s *Service) doLock() {
	if s.inhibited.Load() {
		return
	}
	s.mu.Lock()
	if s.isLocked() {
		s.mu.Unlock()
		return
	}
	cb := s.onLockCallback
	s.mu.Unlock()

	log.Printf("[idle] idle lock triggered")

	if cb != nil {
		cb()
	}

	// Dispatch to Quickshell via IPC for reliable lock screen display.
	go func() {
		qsPath := s.cfg.QsConfigPath
		if qsPath == "" {
			qsPath = "/usr/share/snry-shell/frontend/ii"
		}
		if err := exec.Command("qs", "-p", qsPath, "ipc", "call", "lock", "activate").Run(); err != nil {
			log.Printf("[idle] qs ipc lock failed: %v", err)
			// Fallback: activate global shortcut via IPC
			if err := s.hl.ActivateGlobalShortcut("quickshell:lock"); err != nil {
				log.Printf("[idle] global shortcut fallback failed, no more fallbacks")
			}
		}
	}()

	s.bus.publish(topicScreenLock, true)
}

// Lock triggers a lock externally (e.g. from a socket command).
func (s *Service) Lock() {
	s.doLock()
}

// Unlock triggers an unlock externally (e.g. from a socket command).
func (s *Service) Unlock() {
	s.setDisplay(true)
}

// ── Wayland ─────────────────────────────────────────────────────────────────

func (s *Service) waylandLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := s.initAndDispatch(ctx); err != nil {
				log.Printf("[idle] wayland error: %v, retrying in 5s", err)
				s.cleanupWayland()
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (s *Service) cleanupWayland() {
	s.waylandMu.Lock()
	defer s.waylandMu.Unlock()
	if s.displayOffNotif != nil {
		s.displayOffNotif = nil
	}
	if s.display != nil {
		s.display.Context().Close()
		s.display = nil
	}
	s.manager = nil
	s.seat = nil
}

func (s *Service) initAndDispatch(ctx context.Context) error {
	display, err := client.Connect("")
	if err != nil {
		return err
	}

	registry, err := display.GetRegistry()
	if err != nil {
		display.Destroy()
		return err
	}

	var extIdleName, extIdleVer, seatName, seatVer uint32
	registry.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case "ext_idle_notifier_v1":
			extIdleName, extIdleVer = e.Name, e.Version
		case "wl_seat":
			if seatName == 0 {
				seatName, seatVer = e.Name, e.Version
			}
		}
	})

	if err := waylandutil.Roundtrip(display); err != nil {
		display.Destroy()
		return err
	}

	if extIdleName == 0 {
		display.Destroy()
		log.Printf("[idle] ext_idle_notifier_v1 not available from compositor")
		return nil
	}

	s.waylandMu.Lock()
	s.display = display
	s.manager = protocol.NewExtIdleNotifierV1(display.Context())
	waylandutil.FixedBind(registry, extIdleName, "ext_idle_notifier_v1", extIdleVer, s.manager)
	s.seat = client.NewSeat(display.Context())
	waylandutil.FixedBind(registry, seatName, "wl_seat", seatVer, s.seat)
	s.waylandMu.Unlock()

	if err := waylandutil.Roundtrip(display); err != nil {
		display.Destroy()
		return err
	}

	s.recreateTimers()

	for {
		s.waylandMu.Lock()
		if s.display == nil {
			s.waylandMu.Unlock()
			return nil
		}
		dispCtx := s.display.Context()
		s.waylandMu.Unlock()
		if err := dispCtx.Dispatch(); err != nil {
			return err
		}
	}
}

// ── Logind monitors ─────────────────────────────────────────────────────────

func (s *Service) monitorLogind(ctx context.Context) {
	var rawConn *dbus.Conn
	if realConn, ok := s.conn.(*dbusutil.RealConn); ok && realConn.Conn != nil {
		rawConn = realConn.Conn
	}

	if err := s.conn.AddMatchSignal(
		dbus.WithMatchInterface(dbusutil.LogindManager),
		dbus.WithMatchMember("PrepareForSleep"),
	); err != nil {
		log.Printf("[idle] PrepareForSleep match: %v", err)
	}

	session, _ := dbusutil.GetSessionPath(rawConn)
	if session != "" {
		_ = s.conn.AddMatchSignal(dbus.WithMatchObjectPath(session), dbus.WithMatchInterface(dbusutil.LogindSession), dbus.WithMatchMember("Lock"))
		_ = s.conn.AddMatchSignal(dbus.WithMatchObjectPath(session), dbus.WithMatchInterface(dbusutil.LogindSession), dbus.WithMatchMember("Unlock"))
	}

	ch := make(chan *dbus.Signal, 16)
	s.conn.Signal(ch)
	defer s.conn.RemoveSignal(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			switch sig.Name {
			case dbusutil.LogindManager + ".PrepareForSleep":
				if active, ok := sig.Body[0].(bool); ok && active {
					s.doLock()
				}
			case dbusutil.LogindSession + ".Lock":
				s.doLock()
			case dbusutil.LogindSession + ".Unlock":
				if s.onLogindUnlock != nil {
					s.onLogindUnlock()
				}
				s.setDisplay(true)
			}
		}
	}
}

// monitorLogindInhibit watches the logind BlockInhibited property for
// systemd-inhibit --what=idle inhibitors.
func (s *Service) monitorLogindInhibit(ctx context.Context, conn *dbus.Conn) {
	s.updateLogindInhibit(conn)

	if err := s.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath(dbus.ObjectPath(dbusutil.LogindPath)),
	); err != nil {
		log.Printf("[idle] logind PropertiesChanged match: %v", err)
		return
	}

	ch := make(chan *dbus.Signal, 16)
	s.conn.Signal(ch)
	defer s.conn.RemoveSignal(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if sig.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" &&
				string(sig.Path) == dbusutil.LogindPath &&
				len(sig.Body) >= 2 {
				if iface, _ := sig.Body[0].(string); iface == dbusutil.LogindManager {
					if changed, ok := sig.Body[1].(map[string]dbus.Variant); ok {
						if v, exists := changed["BlockInhibited"]; exists {
							s.handleBlockInhibited(v.String())
						}
					}
				}
			}
		}
	}
}

func (s *Service) updateLogindInhibit(conn *dbus.Conn) {
	v, err := conn.Object(dbusutil.LogindDest, dbus.ObjectPath(dbusutil.LogindPath)).
		GetProperty(dbusutil.LogindManager + ".BlockInhibited")
	if err != nil {
		log.Printf("[idle] logind BlockInhibited get: %v", err)
		return
	}
	s.handleBlockInhibited(v.String())
}

// handleBlockInhibited parses the BlockInhibited colon-separated string
// and updates the logind inhibition source flag.
func (s *Service) handleBlockInhibited(inhibits string) {
	wrapped := ":" + inhibits + ":"
	active := strings.Contains(wrapped, ":idle:")
	log.Printf("[idle] systemd inhibit: %q idle-active=%v", inhibits, active)
	s.mu.Lock()
	s.logindIdleInhibited = active
	s.mu.Unlock()
	s.recomputeInhibition()
}

// ── Hyprland event monitor ──────────────────────────────────────────────────

// monitorHyprlandEvents connects to the Hyprland event socket and watches for
// fullscreen state changes. Fully event-driven — no polling.
func (s *Service) monitorHyprlandEvents(ctx context.Context) {
	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if instance == "" {
		return
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return
	}
	sockPath := runtimeDir + "/hypr/" + instance + "/.socket2.sock"

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			log.Printf("[idle] hyprland event socket: %v, retrying in 5s", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Reset fullscreen count on reconnect — we don't know the current state
		// until we receive events.
		s.mu.Lock()
		s.fullscreenCount = 0
		s.mu.Unlock()
		s.recomputeInhibition()

		err = s.readHyprlandEvents(ctx, conn)
		conn.Close()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("[idle] hyprland event reader: %v, reconnecting", err)
		}
		time.Sleep(time.Second)
	}
}

// readHyprlandEvents reads events from the Hyprland event socket until the
// connection closes or ctx is cancelled.
func (s *Service) readHyprlandEvents(ctx context.Context, conn net.Conn) error {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ">>", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "fullscreen":
			// Hyprland fullscreen event data: "1" or "ADDRESS,1" for fullscreen,
			// "0" or "ADDRESS,0" for unfullscreen.
			data := parts[1]
			fields := strings.Split(data, ",")
			state := fields[len(fields)-1]

			s.mu.Lock()
			if state == "1" {
				s.fullscreenCount++
			} else if s.fullscreenCount > 0 {
				s.fullscreenCount--
			}
			s.mu.Unlock()
			s.recomputeInhibition()
		}
	}
	return scanner.Err()
}
