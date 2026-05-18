package cliphist

import (
	"context"
	"log"
	"math/rand"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Binary string
}

func DefaultConfig() Config {
	return Config{
		Binary: "cliphist",
	}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex
	entries  []string
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		callback: cb,
	}
}

func (s *Service) Run(ctx context.Context) error {
	log.Println("[cliphist] starting wl-paste watcher")

	const (
		minBackoff = 1 * time.Second
		maxBackoff = 30 * time.Second
		resetAfter = 30 * time.Second
	)

	backoff := minBackoff

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		start := time.Now()
		cmd := exec.CommandContext(ctx, "wl-paste", "--watch", s.cfg.Binary, "store")
		err := cmd.Run()

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			// Reset backoff if the process ran long enough before failing
			if time.Since(start) >= resetAfter {
				backoff = minBackoff
			}

			log.Printf("[cliphist] wl-paste watcher exited: %v, restarting in %v", err, backoff)

			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			// Add small jitter to avoid thundering herd
			backoff += time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
		} else {
			// Clean exit is unexpected for a watcher; reset backoff
			backoff = minBackoff
		}
	}
}

func (s *Service) ListEntries(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, s.cfg.Binary, "list").Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	entries := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			entries = append(entries, line)
		}
	}

	s.mu.Lock()
	s.entries = entries
	s.mu.Unlock()

	return entries, nil
}

func (s *Service) EmitList(ctx context.Context) {
	entries, err := s.ListEntries(ctx)
	if err != nil {
		log.Printf("[cliphist] list error: %v", err)
		return
	}

	s.callback(map[string]any{
		"event": "cliphist_list",
		"data": map[string]any{
			"entries": entries,
		},
	})
}

func (s *Service) DeleteEntry(ctx context.Context, entry string) {
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo "+shellQuote(entry)+" | "+s.cfg.Binary+" delete")
	if err := cmd.Run(); err != nil {
		log.Printf("[cliphist] delete error: %v", err)
		return
	}
	s.EmitList(ctx)
}

func (s *Service) Wipe(ctx context.Context) {
	if err := exec.CommandContext(ctx, s.cfg.Binary, "wipe").Run(); err != nil {
		log.Printf("[cliphist] wipe error: %v", err)
		return
	}
	s.EmitList(ctx)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
