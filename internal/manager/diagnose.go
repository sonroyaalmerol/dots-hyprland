package manager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/platform"
)

// Diagnose collects system information and writes it to a result file.
func Diagnose(ctx context.Context, cfg Config) error {
	outDir := cfg.XDG.CacheHome + "/snry-shell"
	_ = os.MkdirAll(outDir, 0o755)

	var buf bytes.Buffer

	fmt.Fprintln(&buf, "========================================")
	fmt.Fprintln(&buf, "Git Repo Info")
	fmt.Fprintln(&buf, "========================================")

	fmt.Fprintf(&buf, "Remote URL: %s\n", capture(ctx, "git", "-C", cfg.RepoRoot, "remote", "get-url", "origin"))
	fmt.Fprintf(&buf, "HEAD: %s\n", capture(ctx, "git", "-C", cfg.RepoRoot, "rev-parse", "HEAD"))
	fmt.Fprintf(&buf, "Status:\n%s\n", capture(ctx, "git", "-C", cfg.RepoRoot, "status"))
	fmt.Fprintf(&buf, "Submodules:\n%s\n", capture(ctx, "git", "-C", cfg.RepoRoot, "submodule", "status", "--recursive"))

	fmt.Fprintln(&buf, "\n========================================")
	fmt.Fprintln(&buf, "Distro Info")
	fmt.Fprintln(&buf, "========================================")
	fmt.Fprintf(&buf, "Distro: %s\n", platform.Detect())
	fmt.Fprintf(&buf, "/etc/os-release:\n%s\n", capture(ctx, "cat", "/etc/os-release"))

	fmt.Fprintln(&buf, "\n========================================")
	fmt.Fprintln(&buf, "XDG Variables")
	fmt.Fprintln(&buf, "========================================")
	fmt.Fprintf(&buf, "XDG_CACHE_HOME:  %s\n", cfg.XDG.CacheHome)
	fmt.Fprintf(&buf, "XDG_CONFIG_HOME: %s\n", cfg.XDG.ConfigHome)
	fmt.Fprintf(&buf, "XDG_DATA_HOME:   %s\n", cfg.XDG.DataHome)
	fmt.Fprintf(&buf, "XDG_STATE_HOME:  %s\n", cfg.XDG.StateHome)
	fmt.Fprintf(&buf, "XDG_BIN_HOME:    %s\n", cfg.XDG.BinHome)
	fmt.Fprintf(&buf, "Venv path:       %s\n", cfg.VenvPath())

	fmt.Fprintln(&buf, "\n========================================")
	fmt.Fprintln(&buf, "Venv Status")
	fmt.Fprintln(&buf, "========================================")
	if _, err := os.Stat(cfg.VenvPath()); err != nil {
		fmt.Fprintf(&buf, "%s: does not exist\n", cfg.VenvPath())
	} else {
		fmt.Fprintf(&buf, "%s: exists\n", cfg.VenvPath())
	}

	fmt.Fprintln(&buf, "\n========================================")
	fmt.Fprintln(&buf, "Version Info")
	fmt.Fprintln(&buf, "========================================")
	fmt.Fprintf(&buf, "Hyprland: %s", capture(ctx, "Hyprland", "--version"))
	fmt.Fprintf(&buf, "Quickshell: %s", capture(ctx, "pacman", "-Q", "quickshell-git", "quickshell"))
	fmt.Fprintf(&buf, "MicroTeX: %s", capture(ctx, "pacman", "-Q", "snry-shell-microtex-git"))

	resultPath := outDir + "/diagnose.result"
	if err := os.WriteFile(resultPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write diagnose result: %w", err)
	}

	fmt.Print(buf.String())
	fmt.Printf("\nResults written to %s\n", resultPath)
	return nil
}

func capture(ctx context.Context, name string, args ...string) string {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("N/A (%v)", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String())
}
