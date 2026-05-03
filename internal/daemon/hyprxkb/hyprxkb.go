package hyprxkb

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type Config struct {
	BaseLayoutPath string
}

func DefaultConfig() Config {
	return Config{BaseLayoutPath: "/usr/share/X11/xkb/rules/base.lst"}
}

type Service struct {
	cfg               Config
	callback          func(map[string]any)
	querySocket       func(string) ([]byte, error)
	mu                sync.RWMutex
	layoutCodes       []string
	currentLayoutName string
	currentLayoutCode string
	cachedCodes       map[string]string
}

func New(cfg Config, querySocket func(string) ([]byte, error), cb func(map[string]any)) *Service {
	return &Service{
		cfg:         cfg,
		querySocket: querySocket,
		callback:    cb,
		cachedCodes: make(map[string]string),
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.refresh()
	<-ctx.Done()
	return ctx.Err()
}

func (s *Service) refresh() {
	data, err := s.querySocket("j/devices")
	if err != nil {
		return
	}

	var devices struct {
		Keyboards []struct {
			Main         bool   `json:"main"`
			Layout       string `json:"layout"`
			ActiveKeymap string `json:"active_keymap"`
		} `json:"keyboards"`
	}
	if err := json.Unmarshal(data, &devices); err != nil {
		return
	}

	for _, kb := range devices.Keyboards {
		if kb.Main {
			s.mu.Lock()
			s.layoutCodes = strings.Split(kb.Layout, ",")
			s.currentLayoutName = kb.ActiveKeymap
			s.mu.Unlock()
			break
		}
	}

	s.resolveLayoutCode()
	s.emit()
}

func (s *Service) resolveLayoutCode() {
	s.mu.RLock()
	name := s.currentLayoutName
	s.mu.RUnlock()

	if name == "" {
		return
	}

	if code, ok := s.cachedCodes[name]; ok {
		s.mu.Lock()
		s.currentLayoutCode = code
		s.mu.Unlock()
		return
	}

	file, err := os.Open(s.cfg.BaseLayoutPath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "!") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 4 {
			desc := strings.Join(fields[2:], " ")
			if strings.HasPrefix(line, " ") && desc == name {
				code := fields[1] + fields[0]
				s.cachedCodes[name] = code
				s.mu.Lock()
				s.currentLayoutCode = code
				s.mu.Unlock()
				return
			}
		}
		if len(fields) >= 2 && !strings.HasPrefix(line, " ") {
			desc := strings.Join(fields[1:], " ")
			if desc == name {
				s.cachedCodes[name] = fields[0]
				s.mu.Lock()
				s.currentLayoutCode = fields[0]
				s.mu.Unlock()
				return
			}
		}
	}
}

func (s *Service) emit() {
	s.mu.RLock()
	codes := make([]string, len(s.layoutCodes))
	copy(codes, s.layoutCodes)
	name := s.currentLayoutName
	code := s.currentLayoutCode
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "hypr_xkb",
		"data": map[string]any{
			"layoutCodes":       codes,
			"currentLayoutName": name,
			"currentLayoutCode": code,
		},
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.emit()
}

func (s *Service) UpdateLayout(name string) {
	s.mu.Lock()
	s.currentLayoutName = name
	s.mu.Unlock()
	s.resolveLayoutCode()
	s.emit()
}
