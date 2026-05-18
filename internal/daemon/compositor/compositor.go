// Package compositor defines the interface for Wayland compositor detection
// and environment management. The daemon does NOT manage the compositor
// lifecycle — it reacts to compositor state changes (available/unavailable).
//
// Hyprland is the default and currently only supported compositor.
package compositor

// Compositor detects and manages environment for a Wayland compositor.
// Implementations handle detecting running instances and importing display
// environment variables into systemd/D-Bus activation environments.
//
// The daemon never launches or stops the compositor. It polls for
// availability and reacts when the compositor appears or disappears.
type Compositor interface {
	// ImportEnvironment sets WAYLAND_DISPLAY, DISPLAY, and the compositor's
	// instance variable in the process environment and imports them into
	// systemd --user and D-Bus activation environments.
	ImportEnvironment(runtimeDir string)

	// Name returns a human-readable name for this compositor (e.g. "hyprland").
	Name() string
}
