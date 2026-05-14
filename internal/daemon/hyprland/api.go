package hyprland

import (
	"fmt"
	"strings"
)

// ── Version support ───────────────────────────────────────────────────────────

// Version represents a Hyprland version (major.minor).
type Version struct{ Major, Minor int }

// AtLeast reports whether this version is >= other.
func (v Version) AtLeast(other Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor >= other.Minor
}

// Known version thresholds.
var (
	vLegacy = Version{0, 0}  // pre-0.55 hyprlang dispatch
	v0_55   = Version{0, 55} // Lua API
)

// cmdVariant maps a minimum version to a socket command string.
// When dispatching, the variant with the highest supported minVersion wins.
type cmdVariant struct {
	minVersion Version
	cmd        string
}

// API defines high-level Hyprland compositor operations.
// All IPC details are encapsulated — callers never deal with sockets or Lua expressions.
type API interface {
	// ── Workspace ──
	FocusWorkspace(selector string) error
	ToggleSpecialWorkspace(name string) error
	ActiveWorkspaceID() (int, error)

	// ── Window ──
	FocusWindow(selector string) error
	CloseWindow(selector string) error
	MoveWindowToWorkspace(ws, window string) error
	MoveWindowToCoords(x, y, window string) error

	// ── Monitor ──
	FocusMonitor(name string) error
	SetMonitor(output, mode, position string, scale float64) error

	// ── Config ──
	SetOption(key, value string) error
	ResetOption(key string) error
	GetOption(key string) ([]byte, error)
	SetAnimation(leaf string, enabled bool, speed float64, curve, style string) error
	Reload() error

	// ── Actions ──
	ExecCommand(cmd string) error
	ActivateGlobalShortcut(name string) error
	SetDPMS(action string) error
	BindKey(key, cmd string, locked bool) error
	UnbindKey(key string) error

	// ── Data ──
	GetMonitors() ([]byte, error)
	GetClients() ([]byte, error)
	GetDevices() ([]byte, error)
	IsRunning() bool
}

// BuildConfigLua translates a dot-separated key and value into a nested hl.config() Lua expression.
// e.g. BuildConfigLua("cursor.zoom_factor", "1.5") → `hl.config({ cursor = { zoom_factor = 1.5 } })`
func BuildConfigLua(key, value string) string {
	parts := splitConfigKey(key)
	if len(parts) == 1 {
		return fmt.Sprintf("hl.config({ %s = %s })", parts[0], value)
	}
	inner := fmt.Sprintf("%s = %s", parts[len(parts)-1], value)
	for i := len(parts) - 2; i >= 0; i-- {
		inner = fmt.Sprintf("%s = { %s }", parts[i], inner)
	}
	return fmt.Sprintf("hl.config({ %s })", inner)
}

// splitConfigKey normalizes a config key to dot-separated parts. Accepts both dot and colon notation.
func splitConfigKey(key string) []string {
	normalized := strings.ReplaceAll(key, ":", ".")
	return strings.Split(normalized, ".")
}
