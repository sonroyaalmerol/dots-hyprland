package gamemode

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	CheckInterval time.Duration
}

func DefaultConfig() Config {
	return Config{CheckInterval: 5 * time.Second}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex
	enabled  bool
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		callback: cb,
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.detectState()
	ticker := time.NewTicker(s.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.detectState()
		}
	}
}

func (s *Service) detectState() {
	out, err := exec.Command("hyprctl", "getoption", "animations:enabled", "-j").Output()
	if err != nil {
		log.Printf("[gamemode] detect state: %v", err)
		return
	}

	// Parse {"int": 0} or {"int": 1}
	output := strings.TrimSpace(string(out))
	animationsDisabled := strings.Contains(output, `"int": 0`)

	s.mu.Lock()
	s.enabled = animationsDisabled // game mode ON when animations are disabled
	s.mu.Unlock()

	s.emit()
}

func (s *Service) emit() {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "game_mode",
		"data":  map[string]any{"enabled": enabled},
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.emit()
}

func (s *Service) Enable() {
	_, err := exec.Command("hyprctl", "--batch",
		"keyword animations:enabled 0;"+
			" keyword decoration:shadow:enabled 0;"+
			" keyword decoration:blur:enabled 0;"+
			" keyword general:gaps_in 0;"+
			" keyword general:gaps_out 0;"+
			" keyword general:border_size 1;"+
			" keyword decoration:rounding 0;"+
			" keyword general:allow_tearing 1").CombinedOutput()
	if err != nil {
		log.Printf("[gamemode] enable failed: %v", err)
		return
	}

	s.mu.Lock()
	s.enabled = true
	s.mu.Unlock()

	log.Printf("[gamemode] game mode enabled (performance mode)")
	s.emit()
}

func (s *Service) Disable() {
	out, err := exec.Command("hyprctl", "reload").CombinedOutput()
	if err != nil {
		log.Printf("[gamemode] disable (reload) failed: %v: %s", err, strings.TrimSpace(string(out)))
		return
	}

	s.mu.Lock()
	s.enabled = false
	s.mu.Unlock()

	log.Printf("[gamemode] game mode disabled (restored defaults)")
	s.emit()
}

func (s *Service) Toggle() {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if enabled {
		s.Disable()
	} else {
		s.Enable()
	}
}
