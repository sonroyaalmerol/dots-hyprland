// Package idle monitors user inactivity via Wayland ext_idle_notifier_v1
// and triggers screen locking, display power management, and optional suspend.
// It replaces hypridle as a built-in idle manager for snry-shell.
package idle

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
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
	conn        dbusutil.DBusConn
	sessionConn dbusutil.DBusConn
	cfg         Config
	hl          hyprland.API

	mu             sync.Mutex
	idleStarted    time.Time
	inhibited      bool
	onLockCallback func() // called by doLock instead of publishing to internal bus
	lockedProvider func() bool
	lastLocked     bool
	onLogindUnlock func()

	// Wayland fields
	waylandMu        sync.Mutex
	display          *client.Display
	manager          *protocol.ExtIdleNotifierV1
	seat             *client.Seat
	displayOffNotif  *protocol.ExtIdleNotificationV1
	lockCancel       context.CancelFunc // cancels pending lock-after-display-off goroutine
	displayOffForced bool               // suppress setDisplay(true) when true
	onDisplayChange  func(on bool)

	// test hooks
	onRecreateTimers   func() // called at the end of recreateTimers for testing
	testResumedHandler func() // latest resumed handler set by recreateTimers
}

// New creates the idle service.
// sysConn is the system bus (for logind).
// sessionConn is the session bus (for ScreenSaver).
func New(sysConn dbusutil.DBusConn, sessionConn dbusutil.DBusConn, cfg Config, hl hyprland.API) *Service {
	return &Service{
		bus:         newBus(),
		conn:        sysConn,
		sessionConn: sessionConn,
		cfg:         cfg,
		hl:          hl,
	}
}

// SuppressDisplayOn prevents setDisplay(true) from running when on=true.
// Used by power-button/lid-close to prevent Wayland resumed events from waking the display.
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

// SetDisplay is the exported wrapper around setDisplay.
func (s *Service) SetDisplay(on bool) {
	s.setDisplay(on)
}

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
					s.mu.Lock()
					inhibited := s.inhibited
					s.mu.Unlock()
					if inhibited {
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
					s.mu.Lock()
					inhibited := s.inhibited
					s.mu.Unlock()
					if inhibited {
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
	// Suppress display-on when forced off (power button / lid close)
	if on && s.displayOffForced {
		s.mu.Unlock()
		return
	}
	// Cancel pending lock goroutine if display is turned back on
	if on && s.lockCancel != nil {
		s.lockCancel()
		s.lockCancel = nil
	}
	s.mu.Unlock()

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
		if err := registerScreenSaver(sessConn.Conn, newScreenSaver(s.bus)); err != nil {
			log.Printf("[idle] screensaver dbus: %v", err)
		}
	}

	// Subscribe to internal bus events
	s.bus.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		s.mu.Lock()
		s.inhibited = active
		s.mu.Unlock()
		log.Printf("[idle] inhibition: %v (active inhibitors)", active)

		// Recreate timers when inhibition state changes.
		// If inhibition became active while we already idled, the display
		// is off — turn it back on. If inhibition dropped while idled,
		// restart the timers so the lock delay runs again.
		s.recreateTimers()
	})

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
		// The locked-mode idle timer (LockDisplayOffTimeout) will turn it
		// back off after the user stops interacting.
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
	s.mu.Lock()
	cfg := s.cfg
	started := s.idleStarted
	inhibited := s.inhibited
	s.mu.Unlock()
	locked := s.isLocked()

	if inhibited || !locked || started.IsZero() {
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

func (s *Service) doLock() {
	s.mu.Lock()
	if s.isLocked() || s.inhibited {
		s.mu.Unlock()
		return
	}
	cb := s.onLockCallback
	s.mu.Unlock()

	log.Printf("[idle] idle lock triggered")

	// Set daemon lock state first (for idle timer tracking)
	if cb != nil {
		cb()
	}

	// Dispatch to Quickshell via IPC for reliable lock screen display.
	// This bypasses the potentially unreliable socket connection.
	go func() {
		if err := exec.Command("qs", "-c", "ii", "ipc", "call", "lock", "activate").Run(); err != nil {
			log.Printf("[idle] qs ipc lock failed: %v", err)
			// Fallback: activate global shortcut via IPC
			if err := s.hl.ActivateGlobalShortcut("quickshell:lock"); err != nil {
				log.Printf("[idle] global shortcut fallback failed, falling back to hyprlock")
				// Last resort: hyprlock
				exec.Command("hyprlock").Run()
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
// The actual lock state is managed by the lockscreen service via the provider.
func (s *Service) Unlock() {
	// Ensure display is on when unlocking.
	s.setDisplay(true)
}

// monitorLogindInhibit watches the logind BlockInhibited property for
// systemd-inhibit --what=idle inhibitors. When :idle: appears in the
// colon-separated list, we treat it as active idle inhibition.
func (s *Service) monitorLogindInhibit(ctx context.Context, conn *dbus.Conn) {
	// Get initial value
	s.updateLogindInhibit(conn)

	// Watch for property changes
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

// updateLogindInhibit fetches the current BlockInhibited property from logind.
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
// and activates/deactivates idle inhibition.
func (s *Service) handleBlockInhibited(inhibits string) {
	// BlockInhibited is colon-separated. Wrap in colons for easier checking.
	wrapped := ":" + inhibits + ":"
	active := strings.Contains(wrapped, ":idle:")
	log.Printf("[idle] systemd inhibit: %q idle-active=%v", inhibits, active)
	s.bus.publish(topicIdleInhibit, active)
}

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
