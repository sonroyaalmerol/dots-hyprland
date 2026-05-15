package dm

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// VT manages a Linux virtual terminal.
type VT struct {
	fd  *os.File
	num int
}

// ioctl constants for VT management.
const (
	kdSetmode    = 0x4B3A
	kdGraphics   = 0x03
	kdText       = 0x00
	vtActivate   = 0x5606
	vtWaitactive = 0x5607
	vtOpenqry    = 0x5600
	vtGetstate   = 0x5603
)

// OpenVT opens the specified VT number. If num is 0, it finds an available VT.
func OpenVT(num int) (*VT, error) {
	var vtNum int
	var f *os.File
	var err error

	if num > 0 {
		// Open the specific VT.
		path := fmt.Sprintf("/dev/tty%d", num)
		f, err = os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		vtNum = num
	} else {
		// Find an available VT.
		console, err := os.Open("/dev/console")
		if err != nil {
			return nil, fmt.Errorf("open /dev/console: %w", err)
		}
		defer console.Close()

		var avail int
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, console.Fd(), vtOpenqry, uintptr(unsafe.Pointer(&avail)))
		if errno != 0 {
			return nil, fmt.Errorf("VT_OPENQRY: %v", errno)
		}

		path := fmt.Sprintf("/dev/tty%d", avail)
		f, err = os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		vtNum = avail
	}

	return &VT{fd: f, num: vtNum}, nil
}

// Num returns the VT number.
func (v *VT) Num() int { return v.num }

// Activate switches to this VT.
func (v *VT) Activate() error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, v.fd.Fd(), vtActivate, uintptr(v.num))
	if errno != 0 {
		return fmt.Errorf("VT_ACTIVATE(%d): %v", v.num, errno)
	}
	return nil
}

// WaitActive blocks until this VT becomes active.
func (v *VT) WaitActive() error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, v.fd.Fd(), vtWaitactive, uintptr(v.num))
	if errno != 0 {
		return fmt.Errorf("VT_WAITACTIVE(%d): %v", v.num, errno)
	}
	return nil
}

// SetGraphicsMode sets the VT to graphics mode (disables text console rendering).
func (v *VT) SetGraphicsMode() error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, v.fd.Fd(), kdSetmode, kdGraphics)
	if errno != 0 {
		return fmt.Errorf("KDSETMODE KD_GRAPHICS: %v", errno)
	}
	return nil
}

// SetTextMode restores the VT to text mode.
func (v *VT) SetTextMode() error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, v.fd.Fd(), kdSetmode, kdText)
	if errno != 0 {
		return fmt.Errorf("KDSETMODE KD_TEXT: %v", errno)
	}
	return nil
}

// Close releases the VT file descriptor.
func (v *VT) Close() error {
	if v.fd != nil {
		v.SetTextMode()
		return v.fd.Close()
	}
	return nil
}
