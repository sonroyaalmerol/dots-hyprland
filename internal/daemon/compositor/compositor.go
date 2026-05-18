// Package compositor launches the Wayland compositor (Hyprland) and waits
// for its IPC socket to become available before the daemon proceeds.
package compositor

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Launch starts the Hyprland compositor and blocks until its IPC socket is
// available (or ctx is cancelled). It returns the HYPRLAND_INSTANCE_SIGNATURE
// that should be set in the environment for subsequent Hyprland IPC calls.
//
// If HYPRLAND_INSTANCE_SIGNATURE is already set AND the compositor's socket is
// reachable, Launch is a no-op and returns the existing signature immediately.
// If the signature is stale (socket unreachable), it is cleared and Hyprland
// is relaunched.
//
// If the env var is NOT set, Launch scans for any live Hyprland instance before
// trying to start a new one. This prevents accidentally launching a second
// compositor on top of an already-running session (e.g. if the systemd
// environment hasn't received import-environment yet at daemon restart).
func Launch(ctx context.Context) (string, error) {
	// If Hyprland is already running and reachable, just return its instance signature.
	if sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); sig != "" {
		if IsAlive(sig) {
			return sig, nil
		}
		log.Printf("[compositor] stale HYPRLAND_INSTANCE_SIGNATURE=%q (socket unreachable), clearing", sig)
		os.Unsetenv("HYPRLAND_INSTANCE_SIGNATURE")
	}

	// Scan for an existing live instance before launching.
	// The env var may be missing if systemd hasn't propagated import-environment
	// yet on daemon restart, but Hyprland is still running in the session.
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/run/user/" + fmt.Sprintf("%d", os.Getuid())
	}
	if existing := findLiveInstance(runtimeDir); existing != "" {
		log.Printf("[compositor] found running Hyprland instance (signature=%s) via socket scan", existing)
		os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", existing)
		ImportEnvironment(runtimeDir)
		return existing, nil
	}

	// Launch start-hyprland as a subprocess.
	bin, err := exec.LookPath("start-hyprland")
	if err != nil {
		return "", fmt.Errorf("start-hyprland not found: %w", err)
	}

	// Determine the system config entry point.
	systemConfig := systemHyprlandConfig()

	args := []string{}
	if systemConfig != "" {
		args = append(args, "--", "--config", systemConfig)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	// Detach from the terminal so the compositor runs independently.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start start-hyprland: %w", err)
	}

	log.Printf("[compositor] started start-hyprland (pid %d), waiting for socket...", cmd.Process.Pid)

	// Wait for the Hyprland socket to appear.
	// start-hyprland creates $XDG_RUNTIME_DIR/hypr/<signature>/.socket.sock

	sig, err := waitForSocket(ctx, runtimeDir)
	if err != nil {
		return "", fmt.Errorf("waiting for hyprland socket: %w", err)
	}

	// Set the environment so all subsequent code can find Hyprland.
	os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", sig)

	// Import WAYLAND_DISPLAY and DISPLAY into systemd --user and D-Bus
	// activation environments so all services (quickshell, wl-paste, etc.)
	// can connect to the compositor. This is the standard pattern for Wayland
	// compositors — the environment is propagated system-wide rather than
	// relying on os.Setenv which only affects the daemon process.
	ImportEnvironment(runtimeDir)

	log.Printf("[compositor] Hyprland ready (signature=%s)", sig)
	return sig, nil
}

// waitForSocket polls the hypr directory under runtimeDir for a new instance
// signature directory containing .socket.sock. It returns the signature once found.
func waitForSocket(ctx context.Context, runtimeDir string) (string, error) {
	hyprDir := filepath.Join(runtimeDir, "hypr")

	// Record existing signatures so we can detect the new one.
	existing := make(map[string]bool)
	if entries, err := os.ReadDir(hyprDir); err == nil {
		for _, e := range entries {
			existing[e.Name()] = true
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout.C:
			return "", fmt.Errorf("timed out waiting for hyprland socket (30s)")
		case <-ticker.C:
			entries, err := os.ReadDir(hyprDir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if existing[e.Name()] {
					continue
				}
				// New directory appeared — check for the socket.
				sockPath := filepath.Join(hyprDir, e.Name(), ".socket.sock")
				if _, err := os.Stat(sockPath); err == nil {
					return e.Name(), nil
				}
			}
		}
	}
}

// IsAlive checks whether the Hyprland compositor with the given signature
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

// findLiveInstance scans the hypr directory for a running Hyprland instance
// by testing each instance directory's IPC socket. Returns the signature of
// the first live instance found, or empty string if none.
func findLiveInstance(runtimeDir string) string {
	hyprDir := filepath.Join(runtimeDir, "hypr")
	entries, err := os.ReadDir(hyprDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sockPath := filepath.Join(hyprDir, e.Name(), ".socket.sock")
		conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond)
		if err != nil {
			continue
		}
		conn.Close()
		return e.Name()
	}
	return ""
}

// importEnvironment detects the WAYLAND_DISPLAY socket created by the compositor,
// sets WAYLAND_DISPLAY and DISPLAY in the current process and imports them into
// systemd --user and D-Bus activation environments. This ensures all child
// processes and restarted services can find the display.
func ImportEnvironment(runtimeDir string) {
	wlDisplay := os.Getenv("WAYLAND_DISPLAY")
	display := os.Getenv("DISPLAY")

	// Detect WAYLAND_DISPLAY if not already set, preferring a live socket.
	if wlDisplay == "" {
		wlDisplay = detectWaylandDisplay(runtimeDir)
	}

	// Default DISPLAY if not set.
	if display == "" {
		display = ":0"
	}

	// Set in current process so os.Environ() includes them for child processes.
	os.Setenv("WAYLAND_DISPLAY", wlDisplay)
	os.Setenv("DISPLAY", display)

	// Import into systemd --user environment so all user services see these vars,
	// including services that are started or restarted after this point.
	if err := exec.Command("systemctl", "--user", "import-environment",
		"WAYLAND_DISPLAY", "DISPLAY", "HYPRLAND_INSTANCE_SIGNATURE").Run(); err != nil {
		log.Printf("[compositor] systemctl import-environment: %v", err)
	}

	// Also update D-Bus activation environment for D-Bus activated services.
	if err := exec.Command("dbus-update-activation-environment", "--systemd",
		"WAYLAND_DISPLAY", "DISPLAY").Run(); err != nil {
		log.Printf("[compositor] dbus-update-activation-environment: %v", err)
	}

	log.Printf("[compositor] imported WAYLAND_DISPLAY=%s DISPLAY=%s", wlDisplay, display)
}

// systemHyprlandConfig returns the path to the system-level Hyprland entry point
// (hyprland.lua preferred, hyprland.conf fallback). Returns empty string if not found.
func systemHyprlandConfig() string {
	systemDir := "/usr/share/snry-shell/configs/hypr"

	for _, name := range []string{"hyprland.lua", "hyprland.conf"} {
		p := systemDir + "/" + name
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	for _, root := range []string{".", ".."} {
		for _, name := range []string{"hyprland.lua", "hyprland.conf"} {
			p := root + "/configs/hypr/" + name
			if _, err := os.Stat(p); err == nil {
				abs, _ := filepath.Abs(p)
				return abs
			}
		}
	}

	return ""
}

// detectWaylandDisplay scans runtimeDir for a wayland-N socket that is actually
// accepting connections (not just a stale socket file), and returns the display
// name (e.g. "wayland-1").
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
		// Verify it's a socket file.
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSocket == 0 {
			continue
		}
		// Verify the socket is actually accepting connections (not stale).
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
