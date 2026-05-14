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
func Launch(ctx context.Context) (string, error) {
	// If Hyprland is already running and reachable, just return its instance signature.
	if sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); sig != "" {
		if IsAlive(sig) {
			return sig, nil
		}
		log.Printf("[compositor] stale HYPRLAND_INSTANCE_SIGNATURE=%q (socket unreachable), clearing", sig)
		os.Unsetenv("HYPRLAND_INSTANCE_SIGNATURE")
	}

	// Launch start-hyprland as a subprocess.
	bin, err := exec.LookPath("start-hyprland")
	if err != nil {
		return "", fmt.Errorf("start-hyprland not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, bin)
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
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/run/user/" + fmt.Sprintf("%d", os.Getuid())
	}

	sig, err := waitForSocket(ctx, runtimeDir)
	if err != nil {
		return "", fmt.Errorf("waiting for hyprland socket: %w", err)
	}

	// Set the environment so all subsequent code can find Hyprland.
	os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", sig)

	// Detect and set WAYLAND_DISPLAY and DISPLAY so child processes
	// (quickshell, wl-paste, etc.) can connect to the compositor.
	// systemd --user doesn't have these set before the compositor starts.
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		if wl := detectWaylandDisplay(runtimeDir); wl != "" {
			os.Setenv("WAYLAND_DISPLAY", wl)
			log.Printf("[compositor] set WAYLAND_DISPLAY=%s", wl)
		}
	}
	if os.Getenv("DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
		log.Printf("[compositor] set DISPLAY=:0")
	}

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
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// detectWaylandDisplay scans runtimeDir for wayland-N sockets created by
// the compositor and returns the display name (e.g. "wayland-1").
func detectWaylandDisplay(runtimeDir string) string {
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		name := e.Name()
		// wayland sockets are named wayland-<N> or wayland-<N>-lock
		if strings.HasPrefix(name, "wayland-") && !strings.HasSuffix(name, "-lock") {
			// Verify it's a socket, not just a matching filename.
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSocket != 0 {
				return name
			}
		}
	}
	return ""
}
