package updates

import (
	"context"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Interval    time.Duration
	EnableCheck bool
	IsSuspended func() bool
}

func DefaultConfig() Config {
	return Config{
		Interval:    30 * time.Minute,
		EnableCheck: true,
	}
}

type Service struct {
	cfg       Config
	callback  func(map[string]any)
	mu        sync.RWMutex
	lastCount int
	hasBinary bool
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		callback: cb,
	}
}

func (s *Service) Run(ctx context.Context) error {
	// Check if checkupdates binary exists once at startup
	_, err := exec.LookPath("checkupdates")
	if err != nil {
		log.Printf("[updates] checkupdates not found, service disabled")
		<-ctx.Done()
		return nil
	}
	s.hasBinary = true

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	// Initial check
	s.checkAndEmit(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.checkAndEmit(ctx)
		}
	}
}

func (s *Service) checkAndEmit(ctx context.Context) {
	if !s.hasBinary {
		return
	}

	if s.cfg.IsSuspended != nil && s.cfg.IsSuspended() {
		return
	}

	count, err := s.checkUpdates(ctx)
	if err != nil {
		log.Printf("[updates] check error: %v", err)
		return
	}

	s.mu.Lock()
	s.lastCount = count
	s.mu.Unlock()

	s.callback(map[string]any{
		"event": "updates",
		"data": map[string]any{
			"available": count > 0,
			"count":     count,
		},
	})
}

func (s *Service) checkUpdates(ctx context.Context) (int, error) {
	// Run: checkupdates | wc -l
	check := exec.CommandContext(ctx, "checkupdates")
	wc := exec.CommandContext(ctx, "wc", "-l")

	pipe, err := check.StdoutPipe()
	if err != nil {
		return 0, err
	}
	wc.Stdin = pipe

	if err := check.Start(); err != nil {
		return 0, err
	}

	out, err := wc.Output()
	check.Wait()

	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (s *Service) GetSnapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"available": s.lastCount > 0,
		"count":     s.lastCount,
	}
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.mu.RLock()
	count := s.lastCount
	s.mu.RUnlock()

	callback(map[string]any{
		"event": "updates",
		"data": map[string]any{
			"available": count > 0,
			"count":     count,
		},
	})
}
