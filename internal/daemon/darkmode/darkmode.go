package darkmode

import (
	"bufio"
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	PollInterval time.Duration // default 0 (uses monitor, no polling)
}

func DefaultConfig() Config {
	return Config{}
}

type Service struct {
	cfg  Config
	mu   sync.RWMutex
	dark bool
	cb   func(map[string]any)
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{cfg: cfg, cb: cb}
}

func (s *Service) Run(ctx context.Context) error {
	// Get initial value.
	s.readInitialState()

	for {
		if err := s.monitor(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("[darkmode] monitor exited: %v, restarting in 2s", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (s *Service) readInitialState() {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		log.Printf("[darkmode] initial read: %v", err)
		return
	}
	val := strings.TrimSpace(string(out))
	dark := strings.Contains(val, "prefer-dark")

	s.mu.Lock()
	s.dark = dark
	s.mu.Unlock()

	s.emit()
}

func (s *Service) monitor(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "gsettings", "monitor", "org.gnome.desktop.interface", "color-scheme")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		dark := strings.Contains(line, "prefer-dark")

		s.mu.Lock()
		s.dark = dark
		s.mu.Unlock()

		s.emit()
	}

	cmd.Wait()
	return scanner.Err()
}

func (s *Service) emit() {
	s.mu.RLock()
	dark := s.dark
	s.mu.RUnlock()

	s.cb(map[string]any{
		"event": "dark_mode",
		"data":  map[string]any{"dark": dark},
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.emit()
}

func (s *Service) IsDark() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dark
}
