// Package waylandutil provides common helpers for Wayland protocol interactions,
// including workarounds for library bugs.
package waylandutil

import (
	"encoding/binary"

	"github.com/rajveermalviya/go-wayland/wayland/client"
)

// FixedBind works around a bug in go-wayland where Registry.Bind incorrectly
// uses padded length for the string length field in the wire protocol.
//
// The Wayland wire protocol for strings requires a 32-bit length field containing
// the actual string length PLUS the null terminator (realLen + 1), followed by
// the bytes and padding. The library was incorrectly sending the padded length
// in the length field, causing protocol errors on some compositors.
func FixedBind(registry *client.Registry, name uint32, iface string, version uint32, id client.Proxy) error {
	const opcode = 0
	realLen := uint32(len(iface) + 1)
	paddedLen := (realLen + 3) & ^uint32(3)

	_reqBufLen := 8 + 4 + (4 + paddedLen) + 4 + 4
	_reqBuf := make([]byte, _reqBufLen)
	l := 0

	// Object ID (Registry)
	binary.LittleEndian.PutUint32(_reqBuf[l:l+4], registry.ID())
	l += 4
	// Opcode and total message length
	binary.LittleEndian.PutUint32(_reqBuf[l:l+4], uint32(_reqBufLen<<16|opcode&0x0000ffff))
	l += 4
	// Argument: name
	binary.LittleEndian.PutUint32(_reqBuf[l:l+4], name)
	l += 4
	// Argument: interface string
	binary.LittleEndian.PutUint32(_reqBuf[l:l+4], realLen)
	l += 4
	copy(_reqBuf[l:l+len(iface)], iface)
	l += int(paddedLen)
	// Argument: version
	binary.LittleEndian.PutUint32(_reqBuf[l:l+4], version)
	l += 4
	// Argument: new_id
	binary.LittleEndian.PutUint32(_reqBuf[l:l+4], id.ID())
	l += 4

	return registry.Context().WriteMsg(_reqBuf, nil)
}

// Roundtrip performs a sync round-trip to ensure all pending events are
// delivered and processed before returning.
func Roundtrip(display *client.Display) error {
	cb, err := display.Sync()
	if err != nil {
		return err
	}
	done := make(chan struct{})
	cb.SetDoneHandler(func(client.CallbackDoneEvent) {
		close(done)
	})
	for {
		if err := display.Context().Dispatch(); err != nil {
			return err
		}
		select {
		case <-done:
			return nil
		default:
		}
	}
}
