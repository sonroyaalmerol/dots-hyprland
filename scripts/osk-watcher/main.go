package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/godbus/dbus/v5"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/osk-watcher/inputmethod"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/osk-watcher/tabletmode"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/osk-watcher/uinput"
)

type event struct {
	Event  string `json:"event"`
	Active bool   `json:"active"`
}

var (
	mu     sync.Mutex
	writer = os.Stdout
)

func emit(evt event) {
	mu.Lock()
	defer mu.Unlock()

	enc := json.NewEncoder(writer)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(evt); err != nil {
		log.Printf("failed to encode event: %v", err)
		return
	}
	writer.Sync()
}

func main() {
	// All logs go to stderr.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("")

	// Subcommand dispatch: default = watcher, "uinput" = virtual keyboard
	if len(os.Args) > 1 && os.Args[1] == "uinput" {
		uinput.Run()
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// ── Tablet mode detection ────────────────────────────────────────────
	var conn *dbus.Conn
	dbusConn, err := dbus.SystemBus()
	if err != nil {
		log.Printf("cannot connect to system bus: %v (logind fallback disabled)", err)
	} else {
		conn = dbusConn
	}

	tm := tabletmode.New(conn, func(tablet bool) {
		emit(event{Event: "tablet_mode", Active: tablet})
	})
	wg.Go(func() {
		tm.Run(ctx)
	})

	// ── Input method (text focus) watcher ───────────────────────────────
	im, err := inputmethod.New(func(active bool) {
		emit(event{Event: "text_focus", Active: active})
	})
	if err != nil {
		log.Printf("inputmethod watcher error: %v", err)
	}
	if im != nil {
		wg.Go(func() {
			im.Run(ctx)
		})
	} else {
		log.Printf("zwp_input_method_v2 not available, text focus events disabled")
	}

	// Wait for shutdown.
	<-ctx.Done()
	log.Printf("shutting down...")
	wg.Wait()

	if conn != nil {
		conn.Close()
	}

	fmt.Fprintln(os.Stderr, "exited")
}
