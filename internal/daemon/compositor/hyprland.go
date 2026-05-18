package compositor

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// HyprlandCompositor implements Compositor for the Hyprland Wayland compositor.
type HyprlandCompositor struct{}

// NewHyprland returns a Compositor that works with a running Hyprland instance.
func NewHyprland() *HyprlandCompositor {
	return &HyprlandCompositor{}
}

func (*HyprlandCompositor) Name() string { return "hyprland" }

// ImportEnvironment detects the WAYLAND_DISPLAY socket, sets WAYLAND_DISPLAY
// and DISPLAY in the current process, and imports them into systemd --user
// and D-Bus activation environments so all services can find the display.
func (h *HyprlandCompositor) ImportEnvironment(runtimeDir string) {
	wlDisplay := os.Getenv("WAYLAND_DISPLAY")
	display := os.Getenv("DISPLAY")

	if wlDisplay == "" {
		wlDisplay = detectWaylandDisplay(runtimeDir)
	}

	if display == "" {
		display = ":0"
	}

	os.Setenv("WAYLAND_DISPLAY", wlDisplay)
	os.Setenv("DISPLAY", display)

	if err := exec.Command("systemctl", "--user", "import-environment",
		"WAYLAND_DISPLAY", "DISPLAY", "HYPRLAND_INSTANCE_SIGNATURE").Run(); err != nil {
		log.Printf("[compositor] systemctl import-environment: %v", err)
	}

	if err := exec.Command("dbus-update-activation-environment", "--systemd",
		"WAYLAND_DISPLAY", "DISPLAY").Run(); err != nil {
		log.Printf("[compositor] dbus-update-activation-environment: %v", err)
	}

	log.Printf("[compositor] imported WAYLAND_DISPLAY=%s DISPLAY=%s", wlDisplay, display)
}

// IsAliveCompat checks whether the Hyprland instance with the given signature
// is reachable by testing its IPC socket.
func IsAlive(sig string) bool {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/run/user/" + fmt.Sprintf("%d", os.Getuid())
	}
	sockPath := filepath.Join(runtimeDir, "hypr", sig, ".socket.sock")
	conn, err := net.DialTimeout("unix", sockPath, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// detectWaylandDisplay scans runtimeDir for a wayland-N socket that is actually
// accepting connections (not just a stale socket file).
func detectWaylandDisplay(runtimeDir string) string {
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "wayland-") || strings.HasSuffix(name, "-lock") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSocket == 0 {
			continue
		}
		sockPath := filepath.Join(runtimeDir, name)
		conn, err := net.DialTimeout("unix", sockPath, time.Second)
		if err != nil {
			continue
		}
		conn.Close()
		return name
	}
	return ""
}
