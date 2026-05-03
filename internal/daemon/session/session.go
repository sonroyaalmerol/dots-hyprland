package session

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Interval time.Duration
}

func DefaultConfig() Config {
	return Config{Interval: 10 * time.Second}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex
	lastData map[string]any
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{cfg: cfg, callback: cb}
}

var pkgManagerNames = []string{"yay", "paru", "dnf", "zypper", "apt", "apx", "xbps-install", "snap", "apk", "yum", "epsi", "pikman"}
var downloadNames = []string{"curl", "wget", "aria2c", "yt-dlp"}

func (s *Service) Run(ctx context.Context) error {
	s.check()
	ticker := time.NewTicker(s.cfg.Interval)
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
	pkgRunning := false
	dlRunning := false

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cmdlinePath := filepath.Join("/proc", entry.Name(), "comm")
		data, err := os.ReadFile(cmdlinePath)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(data))

		if slices.Contains(pkgManagerNames, name) {
			pkgRunning = true
		}
		if slices.Contains(downloadNames, name) {
			dlRunning = true
		}

		if pkgRunning && dlRunning {
			break
		}
	}

	if !pkgRunning {
		if _, err := os.Stat("/var/lib/pacman/db.lck"); err == nil {
			pkgRunning = true
		}
	}

	if !dlRunning {
		home, _ := os.UserHomeDir()
		if home != "" {
			downloads := filepath.Join(home, "Downloads")
			if dlEntries, err := os.ReadDir(downloads); err == nil {
				for _, e := range dlEntries {
					n := e.Name()
					if strings.HasSuffix(n, ".crdownload") || strings.HasSuffix(n, ".part") {
						dlRunning = true
						break
					}
				}
			}
		}
	}

	data := map[string]any{
		"packageManagerRunning": pkgRunning,
		"downloadRunning":       dlRunning,
	}

	s.mu.Lock()
	s.lastData = data
	s.mu.Unlock()

	s.callback(map[string]any{
		"event": "session_warnings",
		"data":  data,
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.mu.RLock()
	data := make(map[string]any, len(s.lastData))
	maps.Copy(data, s.lastData)
	s.mu.RUnlock()
	callback(map[string]any{
		"event": "session_warnings",
		"data":  data,
	})
}
