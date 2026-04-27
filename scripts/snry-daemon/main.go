package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/idle"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/idle/dbusutil"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/inputmethod"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/lockscreen"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/quickshell"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/tabletmode"
	"golang.org/x/sys/unix"
)

// ── Shared event emitter ──────────────────────────────────────────────────

type event struct {
	Event  string `json:"event"`
	Active bool   `json:"active"`
}

var clients sync.Map // map[net.Conn]struct{}
var idleSvc *idle.Service
var lockscreenSvc *lockscreen.Service

func emit(evt event) {
	data, err := json.Marshal(evt)
	if err != nil {
		log.Printf("failed to marshal event: %v", err)
		return
	}
	data = append(data, '\n')
	clients.Range(func(key, _ any) bool {
		conn := key.(net.Conn)
		if _, werr := conn.Write(data); werr != nil {
			log.Printf("client write error: %v", werr)
			conn.Close()
			clients.Delete(conn)
		}
		return true
	})
}

func emitMap(m map[string]any) {
	data, err := json.Marshal(m)
	if err != nil {
		log.Printf("failed to marshal event map: %v", err)
		return
	}
	data = append(data, '\n')
	clients.Range(func(key, _ any) bool {
		conn := key.(net.Conn)
		if _, werr := conn.Write(data); werr != nil {
			log.Printf("client write error: %v", werr)
			conn.Close()
			clients.Delete(conn)
		}
		return true
	})
}

// ── Uinput virtual keyboard ──────────────────────────────────────────────

type inputEvent struct {
	sec  int64
	usec int64
	typ  uint16
	code uint16
	val  int32
}

type uinputSetup struct {
	id struct {
		bustype uint16
		vendor  uint16
		product uint16
		version uint16
	}
	name          [80]byte
	ff_effect_max uint32
}

const (
	evSyn  uint16 = 0
	evKey  uint16 = 1
	maxKey        = 248

	uiSetEvBit   = 0x40045564
	uiSetKeyBit  = 0x40045565
	uiDevSetup   = 0x405C5503
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
)

var uinputFd int = -1

func uinputSend(evType, code uint16, value int32) {
	if uinputFd < 0 {
		return
	}
	var ev inputEvent
	ev.typ = evType
	ev.code = code
	ev.val = value
	syscall.Write(uinputFd, (*[24]byte)(unsafe.Pointer(&ev))[:])
}

func uinputSyn() { uinputSend(evSyn, 0, 0) }

func initUinput() error {
	fd, err := unix.Open("/dev/uinput", unix.O_WRONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("open /dev/uinput: %w", err)
	}

	if err := unix.IoctlSetInt(fd, uiSetEvBit, int(evKey)); err != nil {
		unix.Close(fd)
		return fmt.Errorf("UI_SET_EVBIT EV_KEY: %w", err)
	}
	if err := unix.IoctlSetInt(fd, uiSetEvBit, int(evSyn)); err != nil {
		unix.Close(fd)
		return fmt.Errorf("UI_SET_EVBIT EV_SYN: %w", err)
	}
	for code := 0; code <= maxKey; code++ {
		if err := unix.IoctlSetInt(fd, uiSetKeyBit, code); err != nil {
			unix.Close(fd)
			return fmt.Errorf("UI_SET_KEYBIT %d: %w", code, err)
		}
	}

	var setup uinputSetup
	copy(setup.name[:], "snry-osk-virtual\x00")
	setup.id.bustype = 0x06
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uiDevSetup, uintptr(unsafe.Pointer(&setup))); errno != 0 {
		unix.Close(fd)
		return fmt.Errorf("UI_DEV_SETUP: %v", errno)
	}
	if err := unix.IoctlSetInt(fd, uiDevCreate, 0); err != nil {
		unix.Close(fd)
		return fmt.Errorf("UI_DEV_CREATE: %w", err)
	}

	uinputFd = fd
	log.Printf("uinput: virtual keyboard ready (fd %d)", fd)
	return nil
}

func destroyUinput() {
	if uinputFd >= 0 {
		unix.IoctlSetInt(uinputFd, uiDevDestroy, 0)
		unix.Close(uinputFd)
		uinputFd = -1
		log.Printf("uinput: device destroyed")
	}
}

func dispatchCommand(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	switch fields[0] {
	case "press":
		if len(fields) != 2 {
			return
		}
		code, err := strconv.ParseUint(fields[1], 10, 16)
		if err != nil || code > maxKey {
			return
		}
		uinputSend(evKey, uint16(code), 1)
		uinputSyn()
	case "release":
		if len(fields) != 2 {
			return
		}
		code, err := strconv.ParseUint(fields[1], 10, 16)
		if err != nil || code > maxKey {
			return
		}
		uinputSend(evKey, uint16(code), 0)
		uinputSyn()
	case "releaseall":
		for code := uint16(0); code <= maxKey; code++ {
			uinputSend(evKey, code, 0)
		}
		uinputSyn()
	case "auth":
		if len(fields) < 2 || lockscreenSvc == nil {
			return
		}
		password := strings.Join(fields[1:], " ")
		go lockscreenSvc.Authenticate(password)
	case "lock":
		if lockscreenSvc != nil {
			lockscreenSvc.LockWithAutoUnlock()
		} else if idleSvc != nil {
			idleSvc.Lock()
		}
	case "unlock":
		if lockscreenSvc != nil {
			lockscreenSvc.Unlock()
		} else if idleSvc != nil {
			idleSvc.Unlock()
		}
	case "auto-unlock":
		if lockscreenSvc != nil {
			lockscreenSvc.TryAutoUnlock()
		} else if idleSvc != nil {
			idleSvc.Unlock()
		}
	case "power-button", "lid-close":
		if idleSvc != nil {
			idleSvc.SuppressDisplayOn(true)
		}
		if lockscreenSvc != nil {
			lockscreenSvc.Lock()
		}
		if idleSvc != nil {
			idleSvc.SetDisplay(false)
		}
	case "combo":
		if len(fields) < 2 {
			return
		}
		var codes []uint16
		for i := 1; i < len(fields); i++ {
			code, err := strconv.ParseUint(fields[i], 10, 16)
			if err != nil || code > maxKey {
				return
			}
			codes = append(codes, uint16(code))
		}
		for _, code := range codes {
			uinputSend(evKey, code, 1)
			uinputSyn()
		}
		for i := len(codes) - 1; i >= 0; i-- {
			uinputSend(evKey, codes[i], 0)
			uinputSyn()
		}
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	clients.Store(conn, struct{}{})
	defer clients.Delete(conn)

	log.Printf("client connected: %s", conn.RemoteAddr())
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		dispatchCommand(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("client read error: %v", err)
	}
	log.Printf("client disconnected: %s", conn.RemoteAddr())
}

func socketPath() string {
	return filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "snry-daemon.sock")
}

func socketListener(ctx context.Context) error {
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		return fmt.Errorf("XDG_RUNTIME_DIR not set")
	}
	sockPath := socketPath()

	// Remove stale socket file
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", sockPath, err)
	}
	defer listener.Close()
	log.Printf("listening on %s", sockPath)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			log.Printf("accept error: %v", err)
			continue
		}
		go handleClient(conn)
	}
}

// ── Main ──────────────────────────────────────────────────────────────────

func setupHyprlandSystemBinds() func() {
	binds := []struct{ key, cmd string }{
		{"XF86PowerOff", "~/.local/bin/snry-daemon send power-button"},
		{"switch:on:Lid Switch", "~/.local/bin/snry-daemon send lid-close"},
	}
	for _, b := range binds {
		val := ", " + b.key + ", exec, " + b.cmd
		out, err := exec.Command("hyprctl", "keyword", "bindl", val).CombinedOutput()
		if err != nil {
			log.Printf("hyprland bindl %s: %v: %s", b.key, err, string(out))
		} else {
			log.Printf("hyprland bindl registered: %s", b.key)
		}
	}
	return func() {
		for _, b := range binds {
			out, err := exec.Command("hyprctl", "keyword", "unbind", ", "+b.key).CombinedOutput()
			if err != nil {
				log.Printf("hyprland unbind %s: %v: %s", b.key, err, string(out))
			}
		}
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: snry-daemon [send <command>]")
}

func sendCommand(cmd string) int {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		fmt.Fprintln(os.Stderr, "error: XDG_RUNTIME_DIR not set")
		return 1
	}
	sockPath := socketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer conn.Close()
	fmt.Fprintf(conn, "%s\n", cmd)
	// Brief pause to let the daemon process the command
	time.Sleep(100 * time.Millisecond)
	return 0
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("")

	// ── Subcommand detection ──────────────────────────────────────────
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "send":
			if len(os.Args) < 3 {
				usage()
				os.Exit(1)
			}
			os.Exit(sendCommand(strings.Join(os.Args[2:], " ")))
		default:
			usage()
			os.Exit(1)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// ── Uinput virtual keyboard ─────────────────────────────────────────
	if err := initUinput(); err != nil {
		log.Printf("uinput: %v (virtual keyboard disabled)", err)
	}

	// ── Unix socket listener ────────────────────────────────────────────
	wg.Go(func() {
		if err := socketListener(ctx); err != nil {
			log.Printf("socket listener: %v", err)
			cancel()
		}
	})

	// ── Idle manager (own dbus connection to avoid Signal channel conflict) ──
	idleConn, err := dbus.SystemBus()
	if err != nil {
		log.Printf("system bus: %v (idle service disabled)", err)
	}
	if idleConn != nil {
		idleSvc = idle.New(dbusutil.NewRealConn(idleConn), idle.DefaultConfig())
		wg.Go(func() {
			if err := idleSvc.Run(ctx); err != nil && err != context.Canceled {
				log.Printf("idle service: %v", err)
			}
		})
	}

	// ── Lockscreen service ────────────────────────────────────────────────
	lockscreenSvc = lockscreen.New(lockscreen.DefaultConfig(), func(et lockscreen.EventType, data any) {
		switch et {
		case lockscreen.EventLockState:
			locked := data.(bool)
			if idleSvc != nil {
				idleSvc.SetLocked(locked)
			}
			emitMap(map[string]any{
				"event": "lock_state",
				"data": map[string]any{
					"locked": locked,
				},
			})
		case lockscreen.EventAuthResult:
			r := data.(lockscreen.AuthResult)
			emitMap(map[string]any{
				"event": "auth_result",
				"data": map[string]any{
					"success":   r.Success,
					"remaining": r.Remaining,
					"lockedOut": r.LockedOut,
					"message":   r.Message,
				},
			})
		case lockscreen.EventLockoutTick:
			emitMap(map[string]any{
				"event": "lockout_tick",
				"data": map[string]any{
					"remainingSeconds": data.(int),
				},
			})
		}
	})

	// Wire idle service to use lockscreen for lock/unlock
	if idleSvc != nil {
		idleSvc.SetOnLock(func() {
			if lockscreenSvc != nil {
				lockscreenSvc.Lock()
			}
		})
	}

	// ── Tablet mode detection (own dbus connection) ─────────────────────
	tabletConn, err := dbus.SystemBus()
	var conn *dbus.Conn
	if err != nil {
		log.Printf("system bus: %v (tablet mode logind disabled)", err)
	} else {
		conn = tabletConn
	}
	tm := tabletmode.New(conn, func(tablet bool) {
		emit(event{Event: "tablet_mode", Active: tablet})
	})
	wg.Go(func() {
		tm.Run(ctx)
	})

	// ── Input method (text focus) watcher ──────────────────────────────
	im, err := inputmethod.New(func(active bool) {
		emit(event{Event: "text_focus", Active: active})
	})
	if err != nil {
		log.Printf("inputmethod: %v", err)
	}
	if im != nil {
		wg.Go(func() {
			im.Run(ctx)
		})
	} else {
		log.Printf("zwp_input_method_v2 not available, text focus events disabled")
	}

	// ── Hyprland dynamic system binds (power button, lid switch) ────────
	var bindCleanup func()
	wg.Go(func() {
		time.Sleep(3 * time.Second)
		bindCleanup = setupHyprlandSystemBinds()
	})
	defer func() {
		if bindCleanup != nil {
			bindCleanup()
		}
	}()

	// ── QuickShell process manager ──────────────────────────────────────
	qsSvc := quickshell.New(quickshell.DefaultConfig())
	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("quickshell: panic: %v", r)
			}
		}()
		log.Printf("quickshell: service goroutine starting")
		if err := qsSvc.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("quickshell: %v", err)
		}
	})

	// ── Wait for shutdown ──────────────────────────────────────────────
	<-ctx.Done()
	log.Printf("shutting down...")
	qsSvc.Stop()
	destroyUinput()
	wg.Wait()
	fmt.Fprintln(os.Stderr, "exited")
}
