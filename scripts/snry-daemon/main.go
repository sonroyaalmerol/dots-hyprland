package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/idle"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/idle/dbusutil"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/inputmethod"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/tabletmode"
	"golang.org/x/sys/unix"
)

// ── Shared event emitter ──────────────────────────────────────────────────

type event struct {
	Event  string `json:"event"`
	Active bool   `json:"active"`
}

var clients sync.Map // map[net.Conn]struct{}

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

func socketListener(ctx context.Context) error {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return fmt.Errorf("XDG_RUNTIME_DIR not set")
	}
	sockPath := filepath.Join(runtimeDir, "snry-daemon.sock")

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

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("")

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
		idleSvc := idle.New(dbusutil.NewRealConn(idleConn), idle.DefaultConfig())
		wg.Go(func() {
			if err := idleSvc.Run(ctx); err != nil && err != context.Canceled {
				log.Printf("idle service: %v", err)
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

	// ── Wait for shutdown ──────────────────────────────────────────────
	<-ctx.Done()
	log.Printf("shutting down...")
	destroyUinput()
	wg.Wait()
	fmt.Fprintln(os.Stderr, "exited")
}
