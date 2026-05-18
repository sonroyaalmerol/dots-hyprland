package easyeffects

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/proc"
)

type Config struct {
	CheckInterval time.Duration
}

func DefaultConfig() Config {
	return Config{CheckInterval: 10 * time.Second}
}

type Service struct {
	cfg       Config
	callback  func(map[string]any)
	mu        sync.RWMutex
	available bool
	active    bool
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{cfg: cfg, callback: cb}
}

func (s *Service) Run(ctx context.Context) error {
	s.check()
	ticker := time.NewTicker(s.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.check()
		}
	}
}

func (s *Service) check() {
	_, err := exec.LookPath("easyeffects")
	avail := err == nil
	active := proc.Running("easyeffects")

	s.mu.Lock()
	s.available = avail
	s.active = active
	s.mu.Unlock()

	s.emit()
}

func (s *Service) emit() {
	s.mu.RLock()
	avail := s.available
	active := s.active
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "easyeffects",
		"data": map[string]any{
			"available": avail,
			"active":    active,
		},
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.emit()
}

func (s *Service) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

func (s *Service) Enable() {
	go func() {
		proc, err := os.StartProcess("/usr/bin/easyeffects", []string{"easyeffects", "--hide-window", "--service-mode"}, &os.ProcAttr{
			Files: []*os.File{nil, nil, nil},
		})
		if err != nil {
			return
		}
		proc.Release()
		s.mu.Lock()
		s.active = true
		s.mu.Unlock()
		s.emit()
	}()
}

func (s *Service) Disable() {
	go func() {
		if pid := proc.FindPID("easyeffects"); pid > 0 {
			if p, err := os.FindProcess(pid); err == nil {
				p.Signal(os.Interrupt)
			}
		}
		s.mu.Lock()
		s.active = false
		s.mu.Unlock()
		s.emit()
	}()
}
