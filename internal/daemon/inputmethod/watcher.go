package inputmethod

import (
	"context"
	"log"

	"github.com/rajveermalviya/go-wayland/wayland/client"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/inputmethod/protocol"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/waylandutil"
)

// Watcher monitors zwp_input_method_v2 activate/deactivate events on the
// Wayland display and calls the callback for each focus change.
type Watcher struct {
	display  *client.Display
	callback func(active bool)
}

const (
	imInterfaceName = "zwp_input_method_manager_v2"
	seatInterface   = "wl_seat"
	seatVersion     = 8
	imVersion       = 1
)

// New connects to the Wayland display, binds to zwp_input_method_manager_v2
// and wl_seat, creates an input method, and sets up event handlers.
// Returns nil if the protocol is not available.
func New(cb func(active bool)) (*Watcher, error) {
	display, err := client.Connect("")
	if err != nil {
		log.Printf("[inputmethod] cannot connect to Wayland display: %v", err)
		return nil, nil
	}

	registry, err := display.GetRegistry()
	if err != nil {
		log.Printf("[inputmethod] cannot get registry: %v", err)
		display.Destroy()
		return nil, nil
	}

	var (
		imManagerName uint32
		imManagerVer  uint32
		seatName      uint32
		seatVer       uint32
	)

	registry.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case imInterfaceName:
			imManagerName = e.Name
			imManagerVer = e.Version
		case seatInterface:
			if seatName == 0 {
				seatName = e.Name
				seatVer = e.Version
			}
		}
	})

	// Round-trip to receive global events.
	if err := waylandutil.Roundtrip(display); err != nil {
		log.Printf("[inputmethod] registry round-trip failed: %v", err)
		display.Destroy()
		return nil, nil
	}

	if imManagerName == 0 || seatName == 0 {
		if imManagerName == 0 {
			log.Printf("[inputmethod] %s not advertised by compositor", imInterfaceName)
		}
		if seatName == 0 {
			log.Printf("[inputmethod] %s not advertised by compositor", seatInterface)
		}
		display.Destroy()
		return nil, nil
	}

	// Bind the input method manager and seat using the FixedBind workaround.
	manager := protocol.NewInputMethodManager(display.Context())
	if err := waylandutil.FixedBind(registry, imManagerName, imInterfaceName, min(imManagerVer, imVersion), manager); err != nil {
		log.Printf("[inputmethod] bind %s failed: %v", imInterfaceName, err)
		display.Destroy()
		return nil, nil
	}

	seat := client.NewSeat(display.Context())
	if err := waylandutil.FixedBind(registry, seatName, seatInterface, min(seatVer, seatVersion), seat); err != nil {
		log.Printf("[inputmethod] bind %s failed: %v", seatInterface, err)
		display.Destroy()
		return nil, nil
	}

	// Wait for Binds to process.
	if err := waylandutil.Roundtrip(display); err != nil {
		log.Printf("[inputmethod] bind round-trip failed: %v", err)
		display.Destroy()
		return nil, nil
	}

	im, err := manager.GetInputMethod(seat)
	if err != nil {
		log.Printf("[inputmethod] GetInputMethod failed: %v", err)
		display.Destroy()
		return nil, nil
	}

	w := &Watcher{display: display, callback: cb}

	im.SetActivateHandler(func(protocol.InputMethodActivateEvent) {
		log.Printf("[inputmethod] activate")
		cb(true)
	})

	im.SetDeactivateHandler(func(protocol.InputMethodDeactivateEvent) {
		log.Printf("[inputmethod] deactivate")
		cb(false)
	})

	im.SetUnavailableHandler(func(protocol.InputMethodUnavailableEvent) {
		log.Printf("[inputmethod] unavailable")
		cb(false)
	})

	im.SetDoneHandler(func(protocol.InputMethodDoneEvent) {})

	im.SetContentTypeHandler(func(protocol.InputMethodContentTypeEvent) {})

	log.Printf("[inputmethod] connected to input-method-v2 protocol")
	return w, nil
}

// Run dispatches Wayland events in a loop. Blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	log.Printf("[inputmethod] watching for input-method events")

	d := w.display
	go func() {
		<-ctx.Done()
		d.Context().Close()
	}()

	defer d.Context().Close()

	for {
		if err := d.Context().Dispatch(); err != nil {
			log.Printf("[inputmethod] dispatch ended: %v", err)
			return
		}
	}
}
