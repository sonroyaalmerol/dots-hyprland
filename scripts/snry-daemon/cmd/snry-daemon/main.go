package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/app"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "send" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: snry-daemon send <command>")
			os.Exit(1)
		}
		os.Exit(sendCommand(os.Args[2]))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.New(app.DefaultConfig()).Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
}

func sendCommand(cmd string) int {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		fmt.Fprintln(os.Stderr, "error: XDG_RUNTIME_DIR not set")
		return 1
	}
	conn, err := net.Dial("unix", runtimeDir+"/snry-daemon.sock")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer conn.Close()
	fmt.Fprintf(conn, "%s\n", cmd)
	time.Sleep(100 * time.Millisecond)
	return 0
}
