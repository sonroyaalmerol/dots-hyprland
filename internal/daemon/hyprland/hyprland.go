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

	hyprVersion Version

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
	s := &Service{
		cfg:        cfg,
		callback:   cb,
		layers:     make(map[string]any),
		debounceCh: make(chan string, 64),
	}
	s.detectVersion()
	return s
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
	_ = os.Getenv("XDG_RUNTIME_DIR") // checked elsewhere
	return true
}

// ReloadConfig sends a reload command to Hyprland via IPC.
// Package-level convenience for code that doesn't have an API instance
// (e.g. CLI setup flows). Prefer s.Reload() on Service when available.
func ReloadConfig() error {
	_, err := querySocket("reload")
	return err
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

// parseVersion extracts major.minor from a Hyprland version string.
// Input format: "Hyprland 0.55.1, built from branch main at commit ..."
func parseVersion(output string) Version {
	const prefix = "Hyprland "
	_, after, ok := strings.Cut(output, prefix)
	if !ok {
		return v0_55
	}
	s := after
	major, minor := 0, 0
	// Parse major
	for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		major = major*10 + int(s[0]-'0')
		s = s[1:]
	}
	if len(s) > 0 && s[0] == '.' {
		s = s[1:]
		// Parse minor
		for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
			minor = minor*10 + int(s[0]-'0')
			s = s[1:]
		}
	}
	return Version{Major: major, Minor: minor}
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
	// Also refresh monitors — each monitor's activeWorkspace changes on workspace switch.
	m, err := fetchMonitors()
	if err != nil {
		log.Printf("[hyprland] fetchMonitors error: %v", err)
	}
	s.mu.Lock()
	s.workspaces = ws
	if m != nil {
		s.monitors = m
	}
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

// QuerySocket sends a raw command to the Hyprland IPC socket and returns the response.
// Used by packages that need direct socket access for commands not covered by the API.
func (s *Service) QuerySocket(cmd string) ([]byte, error) {
	return querySocket(cmd)
}

// ── Version-aware dispatch helpers ──────────────────────────────────────────────

// send picks the highest-supported cmdVariant and sends it to the socket.
// Variants are ordered from lowest to highest version — the last matching one wins.
func (s *Service) send(variants ...cmdVariant) error {
	cmd := s.bestCmd(variants)
	if cmd == "" {
		return fmt.Errorf("no command for Hyprland %d.%d", s.hyprVersion.Major, s.hyprVersion.Minor)
	}
	_, err := querySocket(cmd)
	return err
}

// sendResult is like send but returns the socket response bytes.
func (s *Service) sendResult(variants ...cmdVariant) ([]byte, error) {
	cmd := s.bestCmd(variants)
	if cmd == "" {
		return nil, fmt.Errorf("no command for Hyprland %d.%d", s.hyprVersion.Major, s.hyprVersion.Minor)
	}
	return querySocket(cmd)
}

// bestCmd returns the command string from the highest-supported variant.
func (s *Service) bestCmd(variants []cmdVariant) string {
	var result string
	for _, v := range variants {
		if s.hyprVersion.AtLeast(v.minVersion) {
			result = v.cmd
		}
	}
	return result
}

// detectVersion queries the Hyprland IPC socket for the running version.
// Falls back to v0_55 (Lua API) if detection fails.
func (s *Service) detectVersion() {
	data, err := querySocket("version")
	if err != nil || len(data) == 0 {
		s.hyprVersion = v0_55
		return
	}
	s.hyprVersion = parseVersion(string(data))
}

// ── High-level Hyprland API ─────────────────────────────────────────────────────
// All methods use send() which selects the correct command format based on the
// detected Hyprland version. Add new version thresholds by appending cmdVariant entries.

func (s *Service) FocusWorkspace(selector string) error {
	return s.send(
		cmdVariant{vLegacy, "dispatch workspace " + selector},
		cmdVariant{v0_55, fmt.Sprintf("dispatch hl.dsp.focus({ workspace = %q })", selector)},
	)
}

func (s *Service) ToggleSpecialWorkspace(name string) error {
	legacy := "dispatch togglespecialworkspace"
	lua := "dispatch hl.dsp.workspace.toggle_special()"
	if name != "" {
		legacy = "dispatch togglespecialworkspace " + name
		lua = fmt.Sprintf("dispatch hl.dsp.workspace.toggle_special(%q)", name)
	}
	return s.send(
		cmdVariant{vLegacy, legacy},
		cmdVariant{v0_55, lua},
	)
}

func (s *Service) FocusMonitor(name string) error {
	return s.send(
		cmdVariant{vLegacy, "dispatch focusmonitor " + name},
		cmdVariant{v0_55, fmt.Sprintf("dispatch hl.dsp.focus({ monitor = %q })", name)},
	)
}

func (s *Service) FocusWindow(selector string) error {
	return s.send(
		cmdVariant{vLegacy, "dispatch focuswindow " + selector},
		cmdVariant{v0_55, fmt.Sprintf("dispatch hl.dsp.focus({ window = %q })", selector)},
	)
}

func (s *Service) CloseWindow(selector string) error {
	return s.send(
		cmdVariant{vLegacy, "dispatch closewindow " + selector},
		cmdVariant{v0_55, fmt.Sprintf("dispatch hl.dsp.window.close(%q)", selector)},
	)
}

func (s *Service) MoveWindowToWorkspace(ws, window string) error {
	var legacy, lua string
	if window != "" {
		legacy = "dispatch movetoworkspacesilent " + ws + "," + window
		lua = fmt.Sprintf("dispatch hl.dsp.window.move({ workspace = %q, follow = false, window = %q })", ws, window)
	} else {
		legacy = "dispatch movetoworkspacesilent " + ws
		lua = fmt.Sprintf("dispatch hl.dsp.window.move({ workspace = %q, follow = false })", ws)
	}
	return s.send(
		cmdVariant{vLegacy, legacy},
		cmdVariant{v0_55, lua},
	)
}

func (s *Service) MoveWindowToCoords(x, y, window string) error {
	var legacy, lua string
	if window != "" {
		legacy = "dispatch movewindowpixel exact " + x + " " + y + "," + window
		lua = fmt.Sprintf("dispatch hl.dsp.window.move({ x = %q, y = %q, relative = false, window = %q })", x, y, window)
	} else {
		legacy = "dispatch movewindowpixel exact " + x + " " + y
		lua = fmt.Sprintf("dispatch hl.dsp.window.move({ x = %q, y = %q, relative = false })", x, y)
	}
	return s.send(
		cmdVariant{vLegacy, legacy},
		cmdVariant{v0_55, lua},
	)
}

func (s *Service) ExecCommand(cmd string) error {
	return s.send(
		cmdVariant{vLegacy, "dispatch exec " + cmd},
		cmdVariant{v0_55, fmt.Sprintf("dispatch hl.dsp.exec_cmd(%q)", cmd)},
	)
}

func (s *Service) ActivateGlobalShortcut(name string) error {
	return s.send(
		cmdVariant{vLegacy, "dispatch global " + name},
		cmdVariant{v0_55, fmt.Sprintf("dispatch hl.dsp.global(%q)", name)},
	)
}

func (s *Service) SetDPMS(action string) error {
	return s.send(
		cmdVariant{vLegacy, "dispatch dpms " + action},
		cmdVariant{v0_55, fmt.Sprintf("dispatch hl.dsp.dpms({ action = %q })", action)},
	)
}

func (s *Service) SetSubmap(name string) error {
	legacy := "dispatch submap " + name
	lua := fmt.Sprintf("dispatch hl.dsp.submap(%q)", name)
	if name == "" || name == "reset" {
		legacy = "dispatch submap reset"
		lua = `dispatch hl.dsp.submap("reset")`
	}
	return s.send(
		cmdVariant{vLegacy, legacy},
		cmdVariant{v0_55, lua},
	)
}

func (s *Service) Reload() error {
	// reload is the same in all versions.
	_, err := querySocket("reload")
	return err
}

func (s *Service) SetOption(key, value string) error {
	legacyKey := strings.ReplaceAll(key, ".", ":")
	return s.send(
		cmdVariant{vLegacy, fmt.Sprintf("keyword %s %s", legacyKey, value)},
		cmdVariant{v0_55, "eval " + BuildConfigLua(key, value)},
	)
}

// ResetOption resets a config key to its default value.
//
// On legacy (pre-0.55): uses "keyword <key> default" which is the documented reset mechanism.
//
// On v0.55+ (Lua config): there is no per-key reset API. "keyword" returns an error
// on non-legacy parsers, and hl.config({key = nil}) silently skips nil values during
// table iteration. The only correct way to reset runtime overrides is a full reload,
// which resets all values to their compiled defaults then re-parses config files.
// Since runtime overrides set via hl.config() are not persisted, the user's config
// files don't contain them, so reload effectively clears them.
func (s *Service) ResetOption(key string) error {
	legacyKey := strings.ReplaceAll(key, ".", ":")
	return s.send(
		cmdVariant{vLegacy, fmt.Sprintf("keyword %s default", legacyKey)},
		cmdVariant{v0_55, "reload"},
	)
}

func (s *Service) GetOption(key string) ([]byte, error) {
	normalized := strings.ReplaceAll(key, ":", ".")
	legacyKey := strings.ReplaceAll(key, ".", ":")
	return s.sendResult(
		cmdVariant{vLegacy, "j/getoption " + legacyKey},
		cmdVariant{v0_55, "j/getoption " + normalized},
	)
}

func (s *Service) SetAnimation(leaf string, enabled bool, speed float64, curve, style string) error {
	expr := fmt.Sprintf("eval hl.animation({ leaf = %q, enabled = %t, speed = %g, curve = %q", leaf, enabled, speed, curve)
	if style != "" {
		expr += fmt.Sprintf(", style = %q", style)
	}
	expr += " })"
	return s.send(cmdVariant{v0_55, expr})
}

func (s *Service) SetMonitor(output, mode, position string, scale float64) error {
	legacy := fmt.Sprintf("keyword monitor %s,%s,%s,%.2f", output, mode, position, scale)
	lua := fmt.Sprintf("eval hl.monitor({ output = %q, mode = %q, position = %q, scale = %g })", output, mode, position, scale)
	return s.send(
		cmdVariant{vLegacy, legacy},
		cmdVariant{v0_55, lua},
	)
}

func (s *Service) GetMonitors() ([]byte, error) {
	return querySocket("j/monitors")
}

func (s *Service) GetClients() ([]byte, error) {
	return querySocket("j/clients")
}

func (s *Service) GetDevices() ([]byte, error) {
	return querySocket("devices")
}

func (s *Service) IsRunning() bool {
	return isHyprlandAvailable()
}

func (s *Service) BindKey(key, cmd string, locked bool) error {
	legacy := fmt.Sprintf("keyword bindl ,%s,exec,%s", key, cmd)
	var lua string
	if locked {
		lua = fmt.Sprintf("eval hl.bind(%q, hl.dsp.exec_cmd(%q), { locked = true })", key, cmd)
	} else {
		lua = fmt.Sprintf("eval hl.bind(%q, hl.dsp.exec_cmd(%q))", key, cmd)
	}
	return s.send(
		cmdVariant{vLegacy, legacy},
		cmdVariant{v0_55, lua},
	)
}

func (s *Service) UnbindKey(key string) error {
	return s.send(
		cmdVariant{vLegacy, fmt.Sprintf("keyword unbind ,%s", key)},
		cmdVariant{v0_55, fmt.Sprintf("eval hl.unbind(%q)", key)},
	)
}

func (s *Service) EnterKillMode() error {
	_, err := querySocket("kill")
	return err
}

func (s *Service) Exit() error {
	return s.send(
		cmdVariant{vLegacy, "dispatch exit"},
		cmdVariant{v0_55, "dispatch hl.dsp.exit()"},
	)
}

func (s *Service) ActiveWorkspaceID() (int, error) {
	data, err := querySocket("j/activeworkspace")
	if err != nil {
		return 0, err
	}
	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, err
	}
	return result.ID, nil
}
