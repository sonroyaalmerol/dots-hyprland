package powersave

import (
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	ppDBusName  = "net.hadess.PowerProfiles"
	ppDBusPath  = "/net/hadess/PowerProfiles"
	ppDBusIface = "net.hadess.PowerProfiles"
)

type StateCallback func(suspended bool)

type Service struct {
	mu             sync.Mutex
	conn           *dbus.Conn
	active         bool
	since          time.Time
	threshold      time.Duration
	onStateChange  StateCallback
	screenOff      bool
	lockedProvider func() bool
}

func New(threshold time.Duration, cb StateCallback) *Service {
	conn, err := dbus.SystemBus()
	if err != nil {
		log.Printf("[powersave] no system bus: %v", err)
		return nil
	}
	return &Service{
		conn:          conn,
		threshold:     threshold,
		onStateChange: cb,
	}
}

func (s *Service) SetLockedProvider(fn func() bool) {
	s.mu.Lock()
	s.lockedProvider = fn
	s.mu.Unlock()
}

func (s *Service) isLocked() bool {
	if s.lockedProvider != nil {
		return s.lockedProvider()
	}
	return false
}

func (s *Service) SetScreenOff(off bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.screenOff
	s.screenOff = off
	if off && !previous && s.isLocked() {
		s.since = time.Now()
	}
	if !off && s.active {
		s.exitPowersave()
	}
}

func (s *Service) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return
	}
	if !s.screenOff || !s.isLocked() || s.since.IsZero() {
		return
	}
	if time.Since(s.since) >= s.threshold {
		s.enterPowersave()
	}
}

func (s *Service) enterPowersave() {
	s.active = true
	log.Printf("[powersave] entering deep idle mode")
	obj := s.conn.Object(ppDBusName, ppDBusPath)
	if err := obj.SetProperty(ppDBusIface+".ActiveProfile", dbus.MakeVariant("power-saver")); err != nil {
		log.Printf("[powersave] set power profile: %v", err)
	}
	exec.Command("bluetoothctl", "noscan").Run()
	if s.onStateChange != nil {
		s.onStateChange(true)
	}
}

func (s *Service) IsSuspended() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *Service) IsScreenOff() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.screenOff
}

func (s *Service) exitPowersave() {
	if !s.active {
		return
	}
	s.active = false
	log.Printf("[powersave] exiting deep idle mode")
	obj := s.conn.Object(ppDBusName, ppDBusPath)
	if err := obj.SetProperty(ppDBusIface+".ActiveProfile", dbus.MakeVariant("balanced")); err != nil {
		log.Printf("[powersave] restore power profile: %v", err)
	}
	if s.onStateChange != nil {
		s.onStateChange(false)
	}
}
