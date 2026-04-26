// Package idle monitors user inactivity via Wayland ext_idle_notifier_v1
// and triggers screen locking, display power management, and optional suspend.
// It replaces hypridle as a built-in idle manager for snry-shell.
package idle

import (
	"context"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/rajveermalviya/go-wayland/wayland/client"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/idle/dbusutil"
	protocol "github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/idle/protocol"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/waylandutil"
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
	bus  *bus
	conn dbusutil.DBusConn
	cfg  Config

	mu             sync.Mutex
	locked         bool
	idleStarted    time.Time
	inhibited      bool
	onLockCallback func() // called by doLock instead of publishing to internal bus

	// Wayland fields
	waylandMu       sync.Mutex
	display         *client.Display
	manager         *protocol.ExtIdleNotifierV1
	seat            *client.Seat
	displayOffNotif *protocol.ExtIdleNotificationV1
	lockCancel      context.CancelFunc // cancels pending lock-after-display-off goroutine
}

// New creates the idle service.
func New(conn dbusutil.DBusConn, cfg Config) *Service {
	return &Service{
		bus:  newBus(),
		conn: conn,
		cfg:  cfg,
	}
}

// SetOnLock registers a callback invoked by doLock instead of publishing to the internal bus.
func (s *Service) SetOnLock(fn func()) {
	s.mu.Lock()
	s.onLockCallback = fn
	s.mu.Unlock()
}

// SetLocked is the exported wrapper around setLocked.
func (s *Service) SetLocked(locked bool) {
	s.setLocked(locked)
}

// SetDisplay is the exported wrapper around setDisplay.
func (s *Service) SetDisplay(on bool) {
	s.setDisplay(on)
}

func (s *Service) recreateTimers() {
	if s.displayOffNotif != nil {
		s.displayOffNotif.Destroy()
		s.displayOffNotif = nil
	}

	s.mu.Lock()
	cfg := s.cfg
	locked := s.locked
	s.mu.Unlock()

	if locked {
		// Locked: one timer at LockDisplayOffTimeout
		if cfg.LockDisplayOffTimeout > 0 {
			ms := uint32(cfg.LockDisplayOffTimeout.Milliseconds())
			notif, err := s.manager.GetIdleNotification(ms, s.seat)
			if err == nil {
				notif.SetIdledHandler(func(protocol.ExtIdleNotificationV1IdledEvent) {
					s.setDisplay(false)
				})
				notif.SetResumedHandler(func(protocol.ExtIdleNotificationV1ResumedEvent) {
					s.setDisplay(true)
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
				notif.SetResumedHandler(func(protocol.ExtIdleNotificationV1ResumedEvent) {
					s.setDisplay(true)
				})
				s.displayOffNotif = notif
			}
		}
	}
}

func (s *Service) setDisplay(on bool) {
	s.mu.Lock()
	// Cancel pending lock goroutine if display is turned back on
	if on && s.lockCancel != nil {
		s.lockCancel()
		s.lockCancel = nil
	}
	s.mu.Unlock()

	if on {
		log.Printf("[idle] turning display ON")
		exec.Command("hyprctl", "dispatch", "dpms", "on").Run()
	} else {
		log.Printf("[idle] turning display OFF")
		exec.Command("hyprctl", "dispatch", "dpms", "off").Run()
	}
}

// Run starts monitoring. Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	// Start logind monitor and ScreenSaver D-Bus
	if realConn, ok := s.conn.(*dbusutil.RealConn); ok && realConn.Conn != nil {
		go s.monitorLogind(ctx)
		if err := registerScreenSaver(realConn.Conn, newScreenSaver(s.bus)); err != nil {
			log.Printf("[idle] screensaver dbus: %v", err)
		}
	}

	// Subscribe to internal bus events
	s.bus.subscribe(topicScreenLock, func(e busEvent) {
		locked, _ := e.Data.(bool)
		s.setLocked(locked)
	})

	s.bus.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		s.mu.Lock()
		s.inhibited = active
		s.mu.Unlock()
		log.Printf("[idle] inhibition: %v", active)
	})

	// Inhibit logind from handling power button / lid switch
	if realConn, ok := s.conn.(*dbusutil.RealConn); ok && realConn.Conn != nil {
		_, err := dbusutil.LogindInhibit(realConn.Conn,
			"handle-power-key:handle-lid-switch",
			"snry-idle", "Shell handling system buttons", "block")
		if err != nil {
			log.Printf("[idle] logind inhibit: %v", err)
		} else {
			log.Printf("[idle] logind button handling inhibited")
		}
	}

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

func (s *Service) setLocked(locked bool) {
	s.mu.Lock()
	changed := s.locked != locked
	s.locked = locked
	if locked {
		s.idleStarted = time.Now()
	} else {
		s.idleStarted = time.Time{}
	}
	s.mu.Unlock()

	if !locked {
		s.setDisplay(true)
	}
	if changed {
		log.Printf("[idle] lock state: %v", locked)
		s.waylandMu.Lock()
		if s.manager != nil {
			s.recreateTimers()
		}
		s.waylandMu.Unlock()
	}
}

func (s *Service) tick() {
	s.mu.Lock()
	cfg := s.cfg
	locked := s.locked
	started := s.idleStarted
	inhibited := s.inhibited
	s.mu.Unlock()

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

	s.waylandMu.Lock()
	s.recreateTimers()
	s.waylandMu.Unlock()

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
	if s.locked || s.inhibited {
		s.mu.Unlock()
		return
	}
	cb := s.onLockCallback
	s.mu.Unlock()

	log.Printf("[idle] idle lock triggered")

	if cb != nil {
		cb()
		return
	}

	// Legacy fallback: try quickshell lock, then hyprlock.
	if exec.Command("pidof", "qs", "quickshell").Run() == nil {
		if err := exec.Command("hyprctl", "dispatch", "global", "quickshell:lock").Run(); err == nil {
			s.bus.publish(topicScreenLock, true)
			return
		}
		log.Printf("[idle] quickshell lock dispatch failed, falling back to hyprlock")
	}

	lockCmd := exec.Command("hyprlock")
	if err := lockCmd.Start(); err != nil {
		log.Printf("[idle] hyprlock failed: %v", err)
		return
	}
	go func() {
		if err := lockCmd.Wait(); err != nil {
			log.Printf("[idle] hyprlock exited: %v", err)
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
	s.mu.Lock()
	cb := s.onLockCallback
	s.mu.Unlock()

	if cb != nil {
		// The callback (set by main.go) handles lockscreenSvc.Unlock().
		s.setLocked(false)
	} else {
		s.bus.publish(topicScreenLock, false)
	}
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
				s.bus.publish(topicScreenLock, false)
			}
		}
	}
}
