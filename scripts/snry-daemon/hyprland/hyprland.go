package hyprland

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct{}

func DefaultConfig() Config { return Config{} }

type Service struct {
	cfg      Config
	callback func(map[string]any)

	mu              sync.RWMutex
	windows         []map[string]any
	monitors        []map[string]any
	workspaces      []map[string]any
	layers          map[string]any
	activeWorkspace map[string]any

	fetchMu    sync.Mutex
	debounce   *time.Timer
	debounceCh chan string
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:        cfg,
		callback:   cb,
		layers:     make(map[string]any),
		debounceCh: make(chan string, 64),
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.fetchAll(); err != nil {
		log.Printf("[hyprland] initial fetch failed: %v", err)
	} else {
		s.emit()
	}

	go s.runDebouncer(ctx)
	return s.subscribeEvents(ctx)
}

func (s *Service) fetchAll() error {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	var err error

	s.mu.Lock()
	s.windows, _ = s.fetchHyprctl("clients", true)
	s.monitors, _ = s.fetchHyprctl("monitors", true)
	s.workspaces, _ = s.fetchHyprctl("workspaces", true)
	s.layers, _ = s.fetchLayers()
	s.activeWorkspace, _ = s.fetchHyprctlSingle("activeworkspace")
	s.mu.Unlock()

	return err
}

func (s *Service) fetchHyprctl(subcmd string, isJSON bool) ([]map[string]any, error) {
	cmd := exec.Command("hyprctl", strings.Fields(subcmd)...)
	if isJSON {
		cmd.Args = append(cmd.Args, "-j")
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	if !isJSON {
		return nil, nil
	}

	var result []map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) fetchHyprctlSingle(subcmd string) (map[string]any, error) {
	cmd := exec.Command("hyprctl", strings.Fields(subcmd)...)
	cmd.Args = append(cmd.Args, "-j")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) fetchLayers() (map[string]any, error) {
	out, err := exec.Command("hyprctl", "layers", "-j").Output()
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) subscribeEvents(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "hyprctl", "events")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	defer cmd.Process.Kill()
	defer cmd.Wait()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line != "" {
			s.handleEvent(line)
		}
	}
	return nil
}

func (s *Service) handleEvent(line string) {
	parts := strings.SplitN(line, ">>", 2)
	if len(parts) != 2 {
		return
	}
	eventName := strings.TrimSpace(parts[0])
	_ = strings.TrimSpace(parts[1])

	select {
	case s.debounceCh <- eventName:
	default:
	}

	switch eventName {
	case "openwindow", "closewindow", "movewindow", "activewindow":
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.fetchWindows()
		}()
	case "workspace", "createworkspace", "destroyworkspace":
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.fetchWorkspaces()
		}()
	case "focusedmon":
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.fetchMonitors()
			s.fetchActiveWorkspace()
		}()
	}
}

func (s *Service) fetchWindows() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	s.mu.Lock()
	s.windows, _ = s.fetchHyprctl("clients", true)
	s.mu.Unlock()
	s.emit()
}

func (s *Service) fetchWorkspaces() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	s.mu.Lock()
	s.workspaces, _ = s.fetchHyprctl("workspaces", true)
	s.mu.Unlock()
	s.emit()
}

func (s *Service) fetchMonitors() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	s.mu.Lock()
	s.monitors, _ = s.fetchHyprctl("monitors", true)
	s.mu.Unlock()
	s.emit()
}

func (s *Service) fetchActiveWorkspace() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	s.mu.Lock()
	s.activeWorkspace, _ = s.fetchHyprctlSingle("activeworkspace")
	s.mu.Unlock()
	s.emit()
}

func (s *Service) runDebouncer(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var pending []string
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-s.debounceCh:
			mu.Lock()
			pending = append(pending, ev)
			mu.Unlock()
		case <-ticker.C:
			mu.Lock()
			if len(pending) > 0 {
				s.fetchAll()
				pending = nil
			}
			mu.Unlock()
		}
	}
}

func (s *Service) emit() {
	s.mu.RLock()
	data := map[string]any{
		"windows":         s.windows,
		"monitors":        s.monitors,
		"workspaces":      s.workspaces,
		"layers":          s.layers,
		"activeWorkspace": s.activeWorkspace,
	}
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "hyprland_data",
		"data":  data,
	})
}
