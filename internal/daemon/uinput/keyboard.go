package uinput

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

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

type Keyboard struct {
	fd int
}

func NewKeyboard() *Keyboard {
	return &Keyboard{fd: -1}
}

func (k *Keyboard) Init() error {
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

	k.fd = fd
	log.Printf("uinput: virtual keyboard ready (fd %d)", fd)
	return nil
}

func (k *Keyboard) Close() {
	if k.fd >= 0 {
		unix.IoctlSetInt(k.fd, uiDevDestroy, 0)
		unix.Close(k.fd)
		k.fd = -1
		log.Printf("uinput: device destroyed")
	}
}

func (k *Keyboard) Press(code uint16) {
	k.send(evKey, code, 1)
	k.syn()
}

func (k *Keyboard) Release(code uint16) {
	k.send(evKey, code, 0)
	k.syn()
}

func (k *Keyboard) ReleaseAll() {
	for code := uint16(0); code <= maxKey; code++ {
		k.send(evKey, code, 0)
	}
	k.syn()
}

func (k *Keyboard) Combo(codes []uint16) {
	for _, code := range codes {
		k.send(evKey, code, 1)
		k.syn()
	}
	for i := len(codes) - 1; i >= 0; i-- {
		k.send(evKey, codes[i], 0)
		k.syn()
	}
}

func (k *Keyboard) send(evType, code uint16, value int32) {
	if k.fd < 0 {
		return
	}
	var ev inputEvent
	ev.typ = evType
	ev.code = code
	ev.val = value
	syscall.Write(k.fd, (*[24]byte)(unsafe.Pointer(&ev))[:])
}

func (k *Keyboard) syn() {
	k.send(evSyn, 0, 0)
}
