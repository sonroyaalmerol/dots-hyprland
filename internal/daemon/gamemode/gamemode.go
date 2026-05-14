package gamemode

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprland"
)

type Config struct {
	CheckInterval time.Duration
}

func DefaultConfig() Config {
	return Config{CheckInterval: 5 * time.Second}
}

type Service struct {
	cfg      Config
	hl       hyprland.API
	callback func(map[string]any)
	mu       sync.RWMutex
	enabled  bool
}

func New(cfg Config, hl hyprland.API, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		hl:       hl,
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
	out, err := s.hl.GetOption("animations.enabled")
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
	configs := []string{
		"animations:enabled 0",
		"decoration:shadow:enabled 0",
		"decoration:blur:enabled 0",
		"general:gaps_in 0",
		"general:gaps_out 0",
		"general:border_size 1",
		"decoration:rounding 0",
		"general:allow_tearing 1",
	}
	for _, cfg := range configs {
		parts := strings.SplitN(cfg, " ", 2)
		key := strings.ReplaceAll(parts[0], ":", ".")
		val := parts[1]
		if err := s.hl.SetOption(key, val); err != nil {
			log.Printf("[gamemode] set %s: %v", cfg, err)
		}
	}

	s.mu.Lock()
	s.enabled = true
	s.mu.Unlock()

	log.Printf("[gamemode] game mode enabled (performance mode)")
	s.emit()
}

func (s *Service) Disable() {
	err := s.hl.Reload()
	if err != nil {
		log.Printf("[gamemode] disable (reload) failed: %v", err)
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
