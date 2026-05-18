package hyprland

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

type Config struct{}

func DefaultConfig() Config { return Config{} }

// Service provides Hyprland IPC operations (config, dispatch, queries).
// Event tracking and data relay have been removed — QuickShell's Hyprland module
// handles that directly in QML.
type Service struct {
	cfg Config

	hyprVersion Version
}

func New(cfg Config, _ func(map[string]any)) *Service {
	s := &Service{
		cfg: cfg,
	}
	s.detectVersion()
	return s
}

func socketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	return runtimeDir + "/hypr/" + instance + "/.socket.sock"
}

func isHyprlandAvailable() bool {
	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if instance == "" {
		return false
	}
	_ = os.Getenv("XDG_RUNTIME_DIR")
	return true
}

// ReloadConfig sends a reload command to Hyprland via IPC.
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

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("write %s: %w", cmd, err)
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
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
func parseVersion(output string) Version {
	const prefix = "Hyprland "
	_, after, ok := strings.Cut(output, prefix)
	if !ok {
		return v0_55
	}
	s := after
	major, minor := 0, 0
	for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		major = major*10 + int(s[0]-'0')
		s = s[1:]
	}
	if len(s) > 0 && s[0] == '.' {
		s = s[1:]
		for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
			minor = minor*10 + int(s[0]-'0')
			s = s[1:]
		}
	}
	return Version{Major: major, Minor: minor}
}

// QuerySocket sends a raw command to the Hyprland IPC socket and returns the response.
func (s *Service) QuerySocket(cmd string) ([]byte, error) {
	return querySocket(cmd)
}

// Run blocks until ctx is cancelled. Event tracking has been removed.
func (s *Service) Run(ctx context.Context) error {
	if !isHyprlandAvailable() {
		log.Println("[hyprland] HYPRLAND_INSTANCE_SIGNATURE not set, skipping")
		<-ctx.Done()
		return nil
	}
	log.Println("[hyprland] service started (data relay removed — handled by QuickShell)")
	<-ctx.Done()
	return ctx.Err()
}

// EmitSnapshot is a no-op kept for interface compatibility.
func (s *Service) EmitSnapshot(func(map[string]any)) {}

// GetSnapshot returns nil — data relay removed.
func (s *Service) GetSnapshot() map[string]any { return nil }

// ── Version-aware dispatch helpers ──────────────────────────────────────────────

func (s *Service) send(variants ...cmdVariant) error {
	cmd := s.bestCmd(variants)
	if cmd == "" {
		return fmt.Errorf("no command for Hyprland %d.%d", s.hyprVersion.Major, s.hyprVersion.Minor)
	}
	_, err := querySocket(cmd)
	return err
}

func (s *Service) sendResult(variants ...cmdVariant) ([]byte, error) {
	cmd := s.bestCmd(variants)
	if cmd == "" {
		return nil, fmt.Errorf("no command for Hyprland %d.%d", s.hyprVersion.Major, s.hyprVersion.Minor)
	}
	return querySocket(cmd)
}

func (s *Service) bestCmd(variants []cmdVariant) string {
	var result string
	for _, v := range variants {
		if s.hyprVersion.AtLeast(v.minVersion) {
			result = v.cmd
		}
	}
	return result
}

func (s *Service) detectVersion() {
	data, err := querySocket("version")
	if err != nil || len(data) == 0 {
		s.hyprVersion = v0_55
		return
	}
	s.hyprVersion = parseVersion(string(data))
}

// ── High-level Hyprland API ─────────────────────────────────────────────────────

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
