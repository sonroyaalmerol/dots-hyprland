package warp

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	PollInterval time.Duration
}

func DefaultConfig() Config {
	return Config{PollInterval: 30 * time.Second}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex
	enabled  bool
	status   string
	visible  bool // warp-cli installed
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		callback: cb,
		status:   "Unknown",
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.detectInstalled()
	if s.visible {
		s.pollStatus()
	}
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if s.visible {
				s.pollStatus()
			}
		}
	}
}

func (s *Service) detectInstalled() {
	_, err := exec.LookPath("warp-cli")
	installed := err == nil

	s.mu.Lock()
	s.visible = installed
	s.mu.Unlock()

	if !installed {
		log.Printf("[warp] warp-cli not found in PATH")
	}
}

func (s *Service) pollStatus() {
	out, err := exec.Command("warp-cli", "status").Output()
	if err != nil {
		log.Printf("[warp] poll status: %v", err)
		return
	}

	output := strings.TrimSpace(string(out))
	status := "Unknown"
	connected := false

	switch {
	case strings.Contains(output, "Connected"):
		status = "Connected"
		connected = true
	case strings.Contains(output, "Disconnected"):
		status = "Disconnected"
	case strings.Contains(output, "Unable to connect"):
		status = "Unable to connect"
	}

	s.mu.Lock()
	s.enabled = connected
	s.status = status
	s.mu.Unlock()

	s.emit()
}

func (s *Service) emit() {
	s.mu.RLock()
	installed := s.visible
	connected := s.enabled
	status := s.status
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "warp_status",
		"data": map[string]any{
			"installed": installed,
			"connected": connected,
			"status":    status,
		},
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.emit()
}

func (s *Service) Connect() {
	s.mu.RLock()
	visible := s.visible
	s.mu.RUnlock()
	if !visible {
		log.Printf("[warp] warp-cli not installed, cannot connect")
		return
	}

	out, err := exec.Command("warp-cli", "connect").CombinedOutput()
	if err != nil {
		log.Printf("[warp] connect failed: %v: %s", err, strings.TrimSpace(string(out)))
		return
	}
	s.pollStatus()
}

func (s *Service) Disconnect() {
	s.mu.RLock()
	visible := s.visible
	s.mu.RUnlock()
	if !visible {
		log.Printf("[warp] warp-cli not installed, cannot disconnect")
		return
	}

	_, err := exec.Command("warp-cli", "disconnect").CombinedOutput()
	if err != nil {
		log.Printf("[warp] disconnect failed: %v", err)
		return
	}
	s.pollStatus()
}

func (s *Service) Register() {
	s.mu.RLock()
	visible := s.visible
	s.mu.RUnlock()
	if !visible {
		log.Printf("[warp] warp-cli not installed, cannot register")
		return
	}

	out, err := exec.Command("warp-cli", "registration", "new").CombinedOutput()
	if err != nil {
		log.Printf("[warp] registration failed: %v: %s", err, strings.TrimSpace(string(out)))
		return
	}
	log.Printf("[warp] registration successful, auto-connecting")
	s.Connect()
}

func (s *Service) Toggle() {
	s.mu.RLock()
	connected := s.enabled
	s.mu.RUnlock()

	if connected {
		s.Disconnect()
	} else {
		s.Connect()
	}
}
