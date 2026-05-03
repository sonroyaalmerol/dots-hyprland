package hyprsunset

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	CheckInterval    time.Duration
	ColorTemperature int
}

func DefaultConfig() Config {
	return Config{
		CheckInterval:    30 * time.Second,
		ColorTemperature: 5000,
	}
}

type Service struct {
	cfg               Config
	querySocket       func(string) ([]byte, error)
	dispatchSocket    func(string) error
	callback          func(map[string]any)
	mu                sync.RWMutex
	temperatureActive bool
	gamma             int
}

func New(cfg Config, querySocket func(string) ([]byte, error), dispatchSocket func(string) error, cb func(map[string]any)) *Service {
	return &Service{
		cfg:            cfg,
		querySocket:    querySocket,
		dispatchSocket: dispatchSocket,
		callback:       cb,
		gamma:          100,
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.fetchState()
	ticker := time.NewTicker(s.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.fetchState()
		}
	}
}

func (s *Service) fetchState() {
	data, err := s.querySocket("hyprsunset temperature")
	if err != nil {
		return
	}
	output := strings.TrimSpace(string(data))
	active := output != "" && !strings.HasPrefix(output, "Couldn't") && output != "6500"

	s.mu.Lock()
	s.temperatureActive = active
	s.mu.Unlock()

	s.emit()
}

func (s *Service) emit() {
	s.mu.RLock()
	active := s.temperatureActive
	gamma := s.gamma
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "hyprsunset",
		"data": map[string]any{
			"temperatureActive": active,
			"gamma":             gamma,
		},
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.emit()
}

func (s *Service) EnableTemperature() {
	s.dispatchSocket("hyprsunset temperature " + strconv.Itoa(s.cfg.ColorTemperature))
	s.mu.Lock()
	s.temperatureActive = true
	s.mu.Unlock()
	s.emit()
}

func (s *Service) DisableTemperature() {
	s.dispatchSocket("hyprsunset identity")
	s.mu.Lock()
	s.temperatureActive = false
	s.mu.Unlock()
	s.emit()
}

func (s *Service) SetGamma(gamma int) {
	if gamma < 25 {
		gamma = 25
	}
	if gamma > 100 {
		gamma = 100
	}
	s.dispatchSocket("hyprsunset gamma " + strconv.Itoa(gamma))
	s.mu.Lock()
	s.gamma = gamma
	s.mu.Unlock()
	s.emit()
}

func (s *Service) ToggleTemperature(active bool) {
	if active {
		s.EnableTemperature()
	} else {
		s.DisableTemperature()
	}
}
