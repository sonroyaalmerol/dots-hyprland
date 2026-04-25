// Package idle monitors user inactivity via Wayland ext_idle_notifier_v1
// and triggers screen locking, display power management, and optional suspend.
// It replaces hypridle as a built-in idle manager for snry-shell.
package idle

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
	LockTimeout           time.Duration
	IdleDisplayOffTimeout time.Duration
	LockDisplayOffTimeout time.Duration
	SuspendTimeout        time.Duration
}

// DefaultConfig returns the factory defaults matching snry-idle requirements.
func DefaultConfig() Config {
	return Config{
		LockTimeout:           5 * time.Minute,
		IdleDisplayOffTimeout: 5 * time.Minute,
		LockDisplayOffTimeout: 3 * time.Second,
		SuspendTimeout:        0,
	}
}

// Service detects user inactivity and triggers locking and optional suspend.
type Service struct {
	bus  *bus
	conn dbusutil.DBusConn
	cfg  Config

	mu          sync.Mutex
	locked      bool
	displayOff  bool
	idleStarted time.Time
	inhibited   bool

	// Stamp file path for lock state
	stampFile string

	// Wayland fields
	waylandMu       sync.Mutex
	display         *client.Display
	manager         *protocol.ExtIdleNotifierV1
	seat            *client.Seat
	lockNotif       *protocol.ExtIdleNotificationV1
	displayOffNotif *protocol.ExtIdleNotificationV1
}

// New creates the idle service.
func New(conn dbusutil.DBusConn, cfg Config) *Service {
	stamp := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "snry-locked")
	return &Service{
		bus:       newBus(),
		conn:      conn,
		cfg:       cfg,
		stampFile: stamp,
	}
}

func (s *Service) recreateTimers() {
	if s.lockNotif != nil {
		s.lockNotif.Destroy()
		s.lockNotif = nil
	}
	if s.displayOffNotif != nil {
		s.displayOffNotif.Destroy()
		s.displayOffNotif = nil
	}

	s.mu.Lock()
	cfg := s.cfg
	locked := s.locked
	s.mu.Unlock()

	// 1. Lock Timer (only if not already locked)
	if !locked && cfg.LockTimeout > 0 {
		ms := uint32(cfg.LockTimeout.Milliseconds())
		notif, err := s.manager.GetIdleNotification(ms, s.seat)
		if err == nil {
			notif.SetIdledHandler(func(protocol.ExtIdleNotificationV1IdledEvent) {
				s.doLock()
			})
			s.lockNotif = notif
		}
	}

	// 2. Display Off Timer
	displayTimeout := cfg.IdleDisplayOffTimeout
	if locked {
		displayTimeout = cfg.LockDisplayOffTimeout
	}

	if displayTimeout > 0 {
		ms := uint32(displayTimeout.Milliseconds())
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
}

func (s *Service) setDisplay(on bool) {
	s.mu.Lock()
	if s.displayOff == !on {
		s.mu.Unlock()
		return
	}
	s.displayOff = !on
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
	// Read initial lock state from stamp file
	if _, err := os.Stat(s.stampFile); err == nil {
		s.mu.Lock()
		s.locked = true
		s.mu.Unlock()
		log.Printf("[idle] detected existing lock stamp at %s", s.stampFile)
	}

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

	// Update stamp file
	if locked {
		os.WriteFile(s.stampFile, []byte{}, 0644)
	} else {
		os.Remove(s.stampFile)
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
	if s.lockNotif != nil {
		s.lockNotif = nil
	}
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
	s.mu.Unlock()

	log.Printf("[idle] idle lock triggered")

	// Try quickshell lock first, fall back to hyprlock
	lockCmd := exec.Command("sh", "-c",
		"pidof qs quickshell >/dev/null 2>&1 && hyprctl dispatch global quickshell:lock || hyprlock")
	if err := lockCmd.Run(); err != nil {
		log.Printf("[idle] lock command failed: %v", err)
	}

	s.bus.publish(topicScreenLock, true)
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
