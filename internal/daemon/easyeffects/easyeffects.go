package easyeffects

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
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

func isProcessRunning(name string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile("/proc/" + entry.Name() + "/comm")
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == name {
			return true
		}
	}
	return false
}

func lookPath(file string) (string, error) {
	path := os.Getenv("PATH")
	for dir := range strings.SplitSeq(path, ":") {
		if dir == "" {
			dir = "."
		}
		p := dir + "/" + file
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

func isBinaryAvailable(name string) bool {
	_, err := lookPath(name)
	return err == nil
}

func (s *Service) check() {
	avail := isBinaryAvailable("easyeffects")
	active := isProcessRunning("easyeffects")

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
		entries, _ := os.ReadDir("/proc")
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			data, err := os.ReadFile("/proc/" + entry.Name() + "/comm")
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(data)) == "easyeffects" {
				if p, err := os.FindProcess(atoi(entry.Name())); err == nil {
					p.Signal(os.Interrupt)
				}
			}
		}
		s.mu.Lock()
		s.active = false
		s.mu.Unlock()
		s.emit()
	}()
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
