package uinput

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// inputEvent mirrors struct input_event from <linux/input.h>.
// On x86_64: timeval is 16 bytes, type+code+value = 8 bytes, total 24.
type inputEvent struct {
	sec  int64
	usec int64
	typ  uint16
	code uint16
	val  int32
}

// uinputSetup mirrors struct uinput_setup from <linux/uinput.h>.
type uinputSetup struct {
	id struct {
		bustype uint16
		vendor  uint16
		product uint16
		version uint16
	}
	name           [80]byte
	ff_effects_max uint32
}

const (
	evSyn uint16 = 0
	evKey uint16 = 1

	uiSetEvBit   = 0x40045564
	uiSetKeyBit  = 0x40045565
	uiDevSetup   = 0x405C5503
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
)

const maxKeyCode = 248

func send(fd int, evType, code uint16, value int32) {
	var ev inputEvent
	ev.typ = evType
	ev.code = code
	ev.val = value
	syscall.Write(fd, (*[24]byte)(unsafe.Pointer(&ev))[:])
}

func sendSyn(fd int) {
	send(fd, evSyn, 0, 0)
}

// Run reads commands from stdin and writes input events to a virtual uinput
// keyboard device. Supported commands: press <code>, release <code>, releaseall.
func Run() {
	fd, err := unix.Open("/dev/uinput", unix.O_WRONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		log.Fatalf("open /dev/uinput: %v", err)
	}

	// Enable EV_KEY and EV_SYN event types.
	if err := unix.IoctlSetInt(fd, uiSetEvBit, int(evKey)); err != nil {
		unix.Close(fd)
		log.Fatalf("UI_SET_EVBIT EV_KEY: %v", err)
	}
	if err := unix.IoctlSetInt(fd, uiSetEvBit, int(evSyn)); err != nil {
		unix.Close(fd)
		log.Fatalf("UI_SET_EVBIT EV_SYN: %v", err)
	}

	// Enable keycodes 0–maxKeyCode.
	for code := 0; code <= maxKeyCode; code++ {
		if err := unix.IoctlSetInt(fd, uiSetKeyBit, code); err != nil {
			unix.Close(fd)
			log.Fatalf("UI_SET_KEYBIT %d: %v", code, err)
		}
	}

	// Create the virtual device.
	var setup uinputSetup
	copy(setup.name[:], "snry-osk-virtual\x00")
	setup.id.bustype = 0x06 // BUS_VIRTUAL
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd),
		uiDevSetup, uintptr(unsafe.Pointer(&setup))); errno != 0 {
		unix.Close(fd)
		log.Fatalf("UI_DEV_SETUP: %v", errno)
	}
	if err := unix.IoctlSetInt(fd, uiDevCreate, 0); err != nil {
		unix.Close(fd)
		log.Fatalf("UI_DEV_CREATE: %v", err)
	}

	log.Printf("osk-input: virtual keyboard ready (fd %d)", fd)

	// Cleanup on signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		unix.IoctlSetInt(fd, uiDevDestroy, 0)
		unix.Close(fd)
		log.Printf("osk-input: device destroyed (signal)")
		os.Exit(0)
	}()

	// Read commands from stdin.
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		cmd := fields[0]

		switch cmd {
		case "press":
			if len(fields) != 2 {
				fmt.Fprintf(os.Stderr, "usage: press <keycode>\n")
				continue
			}
			code, err := strconv.ParseUint(fields[1], 10, 16)
			if err != nil || code > maxKeyCode {
				fmt.Fprintf(os.Stderr, "invalid keycode: %s\n", fields[1])
				continue
			}
			send(fd, evKey, uint16(code), 1)
			sendSyn(fd)

		case "release":
			if len(fields) != 2 {
				fmt.Fprintf(os.Stderr, "usage: release <keycode>\n")
				continue
			}
			code, err := strconv.ParseUint(fields[1], 10, 16)
			if err != nil || code > maxKeyCode {
				fmt.Fprintf(os.Stderr, "invalid keycode: %s\n", fields[1])
				continue
			}
			send(fd, evKey, uint16(code), 0)
			sendSyn(fd)

		case "releaseall":
			for code := uint16(0); code <= maxKeyCode; code++ {
				send(fd, evKey, code, 0)
			}
			sendSyn(fd)

		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("stdin error: %v", err)
	}

	// EOF: destroy device.
	unix.IoctlSetInt(fd, uiDevDestroy, 0)
	unix.Close(fd)
	log.Printf("osk-input: device destroyed (EOF)")
}
