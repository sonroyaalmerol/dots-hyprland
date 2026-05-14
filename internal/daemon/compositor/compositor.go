// Package compositor launches the Wayland compositor (Hyprland) and waits
// for its IPC socket to become available before the daemon proceeds.
package compositor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Launch starts the Hyprland compositor and blocks until its IPC socket is
// available (or ctx is cancelled). It returns the HYPRLAND_INSTANCE_SIGNATURE
// that should be set in the environment for subsequent Hyprland IPC calls.
//
// If HYPRLAND_INSTANCE_SIGNATURE is already set (compositor already running),
// Launch is a no-op and returns the existing signature immediately.
func Launch(ctx context.Context) (string, error) {
	// If Hyprland is already running, just return its instance signature.
	if sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); sig != "" {
		return sig, nil
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
