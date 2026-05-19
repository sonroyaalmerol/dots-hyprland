package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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
		if len(os.Args) >= 3 && os.Args[2] == "--check" {
			runCheckDeps()
		} else {
			runDeps()
		}
	case "sync":
		runSync()
	case "install":
		runInstall()
	case "diagnose":
		runDiagnose()
	case "autoscale":
		runAutoscale()
	case "uninstall":
		runUninstall()
	case "start-hyprland":
		runStartHyprland()
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
  snry-daemon setup        Full installation (deps + install + sync + system setup)
  snry-daemon deps         Install packages
  snry-daemon deps --check Check for missing packages
  snry-daemon sync         Sync config files to XDG directories
  snry-daemon install      Install binaries, plugins, fonts, venv, systemd unit
  snry-daemon diagnose     Collect system diagnostics
  snry-daemon autoscale    Auto-set monitor scale
  snry-daemon uninstall    Remove installed files and revert changes
  snry-daemon start-hyprland Start Hyprland compositor with snry-shell config
  snry-daemon send <cmd>   Send command to running daemon
  snry-daemon help         Show this help`)
}

func runDaemon() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.New(app.DefaultConfig()).Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
}

func runManagerCommand(name string, fn func(context.Context, manager.Config) error) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	cfg := manager.DefaultConfig(manager.FindRepoRoot())
	if err := fn(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "%s error: %v\n", name, err)
		os.Exit(1)
	}
}

func runSetup()     { runManagerCommand("setup", manager.Setup) }
func runDeps()      { runManagerCommand("deps", manager.Deps) }
func runSync()      { runManagerCommand("sync", manager.Files) }
func runInstall()   { runManagerCommand("install", manager.Install) }
func runDiagnose()  { runManagerCommand("diagnose", manager.Diagnose) }
func runCheckDeps() { runManagerCommand("checkdeps", manager.CheckDeps) }
func runUninstall() { runManagerCommand("uninstall", manager.Uninstall) }

func runAutoscale() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := manager.Autoscale(ctx, nil); err != nil {
		fmt.Fprintf(os.Stderr, "autoscale error: %v\n", err)
		os.Exit(1)
	}
}

func runStartHyprland() {
	configPath := "/usr/share/snry-shell/configs/hypr/hyprland.lua"

	// Allow override via env var or CLI arg.
	if env := os.Getenv("SNRY_HYPRLAND_CONFIG"); env != "" {
		configPath = env
	}
	if len(os.Args) >= 3 {
		configPath = os.Args[2]
	}

	if _, err := os.Stat(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: config not found: %s\n", configPath)
		os.Exit(1)
	}

	fmt.Printf("starting Hyprland with config: %s\n", configPath)

	// Detach Hyprland into its own session so it survives the caller exiting.
	cmd := exec.Command("start-hyprland", "--", "-c", configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to start Hyprland: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Hyprland started (pid %d)\n", cmd.Process.Pid)
	os.Exit(0)
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

	fmt.Fprintf(conn, "%s\n", strings.Join(os.Args[2:], " "))

	// Drain server responses (snapshot data, events) to prevent the
	// server's write buffer from filling up and blocking command processing.
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 4096)
	for {
		if _, err := conn.Read(buf); err != nil {
			break
		}
	}
}
