package weather

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Interval    time.Duration
	City        string
	UseUSCS     bool
	EnableGPS   bool
	IsSuspended func() bool
}

func DefaultConfig() Config {
	return Config{
		Interval: 10 * time.Minute,
		City:     "",
	}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex
	lastRaw  string
	lastCity string
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		callback: cb,
	}
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	// Initial fetch
	s.fetchAndEmit(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.fetchAndEmit(ctx)
		}
	}
}

func (s *Service) fetchAndEmit(ctx context.Context) {
	if s.cfg.IsSuspended != nil && s.cfg.IsSuspended() {
		return
	}

	city := s.cfg.City
	if city == "" {
		city = ""
	}

	raw, err := s.fetchWeather(ctx, city)
	if err != nil {
		log.Printf("[weather] fetch error: %v", err)
		return
	}

	s.mu.Lock()
	s.lastRaw = raw
	s.lastCity = city
	s.mu.Unlock()

	s.callback(map[string]any{
		"event": "weather",
		"data": map[string]any{
			"raw":  raw,
			"city": city,
		},
	})
}

func (s *Service) fetchWeather(ctx context.Context, city string) (string, error) {
	url := "wttr.in/"
	if city != "" {
		url += city
	}
	url += "?format=j1"

	curl := exec.CommandContext(ctx, "curl", "-s", url)
	jq := exec.CommandContext(ctx, "jq", "{current: .current_condition[0], location: .nearest_area[0], astronomy: .weather[0].astronomy[0]}")

	pipe, err := curl.StdoutPipe()
	if err != nil {
		return "", err
	}
	jq.Stdin = pipe

	if err := curl.Start(); err != nil {
		return "", err
	}

	out, err := jq.Output()
	curl.Wait()

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func (s *Service) RefreshNow(ctx context.Context) {
	s.fetchAndEmit(ctx)
}

func (s *Service) GetSnapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"raw":  s.lastRaw,
		"city": s.lastCity,
	}
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.mu.RLock()
	raw := s.lastRaw
	city := s.lastCity
	s.mu.RUnlock()

	if raw == "" {
		return
	}

	callback(map[string]any{
		"event": "weather",
		"data": map[string]any{
			"raw":  raw,
			"city": city,
		},
	})
}
