package hyprland

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net"
	"os"
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

func socketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	return runtimeDir + "/hypr/" + instance + "/.socket.sock"
}

func eventSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	return runtimeDir + "/hypr/" + instance + "/.socket2.sock"
}

func isHyprlandAvailable() bool {
	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if instance == "" {
		return false
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/run/user/" + fmt.Sprintf("%d", os.Getuid())
	}
	return true
}

// querySocket connects to the command socket, sends cmd, reads the full response.
func querySocket(cmd string) ([]byte, error) {
	sockPath := socketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", sockPath, err)
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck // best-effort deadline on Unix socket
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("write %s: %w", cmd, err)
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck // best-effort deadline on Unix socket
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if n < len(tmp) || err != nil {
			break
		}
	}
	return buf, nil
}

func fetchWindows() ([]map[string]any, error) {
	data, err := querySocket("j/clients")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchMonitors() ([]map[string]any, error) {
	data, err := querySocket("j/monitors")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchWorkspaces() ([]map[string]any, error) {
	data, err := querySocket("j/workspaces")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchActiveWorkspace() (map[string]any, error) {
	data, err := querySocket("j/activeworkspace")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchLayers() (map[string]any, error) {
	data, err := querySocket("j/layers")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) fetchAll() error {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	s.mu.Lock()
	s.windows, _ = fetchWindows()
	s.monitors, _ = fetchMonitors()
	s.workspaces, _ = fetchWorkspaces()
	s.layers, _ = fetchLayers()
	s.activeWorkspace, _ = fetchActiveWorkspace()
	s.mu.Unlock()

	return nil
}

func (s *Service) fetchWindowsAndUpdate() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	if w, err := fetchWindows(); err != nil {
		log.Printf("[hyprland] fetchWindows error: %v", err)
	} else {
		s.mu.Lock()
		s.windows = w
		s.mu.Unlock()
		s.emit()
	}
}

func (s *Service) fetchWorkspacesAndUpdate() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	ws, err := fetchWorkspaces()
	if err != nil {
		log.Printf("[hyprland] fetchWorkspaces error: %v", err)
		return
	}
	aw, err := fetchActiveWorkspace()
	if err != nil {
		log.Printf("[hyprland] fetchActiveWorkspace error: %v", err)
	}
	s.mu.Lock()
	s.workspaces = ws
	if aw != nil {
		s.activeWorkspace = aw
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) fetchMonitorsAndUpdate() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	m, err := fetchMonitors()
	if err != nil {
		log.Printf("[hyprland] fetchMonitors error: %v", err)
		return
	}
	aw, err := fetchActiveWorkspace()
	if err != nil {
		log.Printf("[hyprland] fetchActiveWorkspace error: %v", err)
	}
	s.mu.Lock()
	s.monitors = m
	if aw != nil {
		s.activeWorkspace = aw
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) fetchLayersAndUpdate() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	if l, err := fetchLayers(); err != nil {
		log.Printf("[hyprland] fetchLayers error: %v", err)
	} else {
		s.mu.Lock()
		s.layers = l
		s.mu.Unlock()
		s.emit()
	}
}

func (s *Service) fetchAllAndUpdate() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	var errs []string
	w, err := fetchWindows()
	if err != nil {
		errs = append(errs, fmt.Sprintf("windows: %v", err))
	}
	m, err := fetchMonitors()
	if err != nil {
		errs = append(errs, fmt.Sprintf("monitors: %v", err))
	}
	ws, err := fetchWorkspaces()
	if err != nil {
		errs = append(errs, fmt.Sprintf("workspaces: %v", err))
	}
	l, err := fetchLayers()
	if err != nil {
		errs = append(errs, fmt.Sprintf("layers: %v", err))
	}
	aw, err := fetchActiveWorkspace()
	if err != nil {
		errs = append(errs, fmt.Sprintf("activeWorkspace: %v", err))
	}
	if len(errs) > 0 {
		log.Printf("[hyprland] fetchAll errors: %s", strings.Join(errs, "; "))
	}
	s.mu.Lock()
	if w != nil {
		s.windows = w
	}
	if m != nil {
		s.monitors = m
	}
	if ws != nil {
		s.workspaces = ws
	}
	if l != nil {
		s.layers = l
	}
	if aw != nil {
		s.activeWorkspace = aw
	}
	s.mu.Unlock()
	s.emit()
}

// categorizeEvent maps an event name to a data category.
func categorizeEvent(eventName string) string {
	switch eventName {
	case "openwindow", "closewindow", "movewindow", "activewindow", "activewindowv2",
		"fullscreen", "changefloatingmode", "windowtitle", "windowtitlev2",
		"movewindowv2", "pin", "minimized", "urgent", "toggleGroup":
		return "windows"

	case "workspace", "workspacev2", "createworkspace", "createworkspacev2",
		"destroyworkspace", "destroyworkspacev2", "renameworkspace":
		return "workspaces"

	case "focusedmon", "focusedmonv2", "monitoradded", "monitorremoved":
		return "monitors"

	case "openlayer", "closelayer":
		return "layers"

	case "configreloaded":
		return "all"
	}
	return ""
}

func (s *Service) subscribeEvents(ctx context.Context) error {
	sockPath := eventSocketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("connect event socket %s: %w", sockPath, err)
	}
	defer conn.Close()

	// Channel for events read by scanner goroutine.
	eventCh := make(chan string, 64)
	doneCh := make(chan error, 1)

	// Read events in a goroutine, send to channel.
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				select {
				case eventCh <- line:
				default:
				}
			}
		}
		if err := scanner.Err(); err != nil {
			doneCh <- err
		} else {
			doneCh <- fmt.Errorf("event socket closed")
		}
	}()

	// Main loop: select on events, debounce timer, context, and done.
	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}

	pending := make(map[string]bool)

	flush := func() {
		cats := make(map[string]bool, len(pending))
		maps.Copy(cats, pending)
		pending = make(map[string]bool)

		if cats["all"] {
			s.fetchAllAndUpdate()
			return
		}
		if cats["windows"] {
			s.fetchWindowsAndUpdate()
		}
		if cats["workspaces"] {
			s.fetchWorkspacesAndUpdate()
		}
		if cats["monitors"] {
			s.fetchMonitorsAndUpdate()
		}
		if cats["layers"] {
			s.fetchLayersAndUpdate()
		}
	}

	for {
		select {
		case <-ctx.Done():
			debounce.Stop()
			return ctx.Err()

		case err := <-doneCh:
			debounce.Stop()
			// Flush any pending events before reconnecting.
			if len(pending) > 0 {
				flush()
			}
			return err

		case line := <-eventCh:
			parts := strings.SplitN(line, ">>", 2)
			if len(parts) != 2 {
				continue
			}
			category := categorizeEvent(strings.TrimSpace(parts[0]))
			if category == "" {
				continue
			}
			pending[category] = true
			debounce.Reset(150 * time.Millisecond)

		case <-debounce.C:
			if len(pending) > 0 {
				flush()
			}
		}
	}
}

func (s *Service) Run(ctx context.Context) error {
	if !isHyprlandAvailable() {
		log.Println("[hyprland] HYPRLAND_INSTANCE_SIGNATURE not set, skipping")
		<-ctx.Done()
		return nil
	}

	log.Println("[hyprland] fetching initial data via sockets...")
	if err := s.fetchAll(); err != nil {
		log.Printf("[hyprland] initial fetch failed: %v", err)
	} else {
		s.emit()
	}

	// Event loop with reconnection.
	backoff := 500 * time.Millisecond
	maxBackoff := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Println("[hyprland] subscribing to events...")
		err := s.subscribeEvents(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		log.Printf("[hyprland] event subscription ended: %v, reconnecting in %v", err, backoff)
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
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

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.mu.RLock()
	data := map[string]any{
		"windows":         s.windows,
		"monitors":        s.monitors,
		"workspaces":      s.workspaces,
		"layers":          s.layers,
		"activeWorkspace": s.activeWorkspace,
	}
	s.mu.RUnlock()

	callback(map[string]any{
		"event": "hyprland_data",
		"data":  data,
	})
}

func (s *Service) GetSnapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"windows":         s.windows,
		"monitors":        s.monitors,
		"workspaces":      s.workspaces,
		"layers":          s.layers,
		"activeWorkspace": s.activeWorkspace,
	}
}
