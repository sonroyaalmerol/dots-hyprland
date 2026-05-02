package cliphist

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
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
	// Start wl-paste --watch cliphist store as a long-running process
	log.Println("[cliphist] starting wl-paste watcher")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cmd := exec.CommandContext(ctx, "wl-paste", "--watch", s.cfg.Binary, "store")
		if err := cmd.Run(); err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				log.Printf("[cliphist] wl-paste watcher exited: %v, restarting...", err)
			}
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
