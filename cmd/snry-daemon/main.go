package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/app"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/manager"
)

func main() {
	if len(os.Args) < 2 {
		runDaemon()
		return
	}

	switch os.Args[1] {
	case "daemon":
		runDaemon()
	case "setup":
		runSetup()
	case "deps":
		runDeps()
	case "files":
		runFiles()
	case "setups":
		runSetups()
	case "diagnose":
		runDiagnose()
	case "checkdeps":
		runCheckDeps()
	case "autoscale":
		runAutoscale()
	case "uninstall":
		runUninstall()
	case "send":
		runSend()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`snry-daemon - Snry Shell manager and runtime daemon

Usage:
  snry-daemon              Start daemon (default)
  snry-daemon daemon       Start daemon explicitly
  snry-daemon setup        Full installation (deps + files + setups)
  snry-daemon deps         Install packages only
  snry-daemon files        Sync config files only
  snry-daemon setups       System setup only (groups, systemd, PAM)
  snry-daemon diagnose     Collect system diagnostics
  snry-daemon checkdeps    Check for missing packages
  snry-daemon autoscale    Auto-set monitor scale
  snry-daemon uninstall    Remove installed files and revert changes
  snry-daemon send <cmd>   Send command to running daemon
  snry-daemon help         Show this help`)
}

func repoRoot() string {
	// Try executable location first (when installed via package)
	exe, err := os.Executable()
	if err == nil {
		shareDir := filepath.Join(filepath.Dir(exe), "..", "share", "snry-shell")
		if _, err := os.Stat(shareDir); err == nil {
			return shareDir
		}
	}

	// Try source repo root (when building from source)
	if _, err := os.Stat("go.mod"); err == nil {
		abs, _ := filepath.Abs(".")
		return abs
	}

	// Try parent directory
	if _, err := os.Stat("../go.mod"); err == nil {
		abs, _ := filepath.Abs("..")
		return abs
	}

	// Fallback to current directory
	abs, _ := filepath.Abs(".")
	return abs
}

func runDaemon() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.New(app.DefaultConfig()).Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
}

func runSetup() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := manager.DefaultConfig(repoRoot())
	if err := manager.Setup(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "setup error: %v\n", err)
		os.Exit(1)
	}
}

func runDeps() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := manager.DefaultConfig(repoRoot())
	if err := manager.Deps(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "deps error: %v\n", err)
		os.Exit(1)
	}
}

func runFiles() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := manager.DefaultConfig(repoRoot())
	if err := manager.Files(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "files error: %v\n", err)
		os.Exit(1)
	}
}

func runSetups() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := manager.DefaultConfig(repoRoot())
	if err := manager.Setups(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "setups error: %v\n", err)
		os.Exit(1)
	}
}

func runDiagnose() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := manager.DefaultConfig(repoRoot())
	if err := manager.Diagnose(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "diagnose error: %v\n", err)
		os.Exit(1)
	}
}

func runCheckDeps() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := manager.DefaultConfig(repoRoot())
	if err := manager.CheckDeps(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "checkdeps error: %v\n", err)
		os.Exit(1)
	}
}

func runAutoscale() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := manager.Autoscale(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "autoscale error: %v\n", err)
		os.Exit(1)
	}
}

func runUninstall() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := manager.DefaultConfig(repoRoot())
	if err := manager.Uninstall(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "uninstall error: %v\n", err)
		os.Exit(1)
	}
}

func runSend() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: snry-daemon send <command>")
		os.Exit(1)
	}

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		fmt.Fprintln(os.Stderr, "error: XDG_RUNTIME_DIR not set")
		os.Exit(1)
	}

	conn, err := net.Dial("unix", runtimeDir+"/snry-daemon.sock")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "%s\n", os.Args[2])
	time.Sleep(100 * time.Millisecond)
}
