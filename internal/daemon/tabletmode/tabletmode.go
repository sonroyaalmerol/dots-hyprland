// Package tabletmode detects tablet mode via evdev SW_TABLET_MODE switches,
// logind DBus TabletMode property, and physical keyboard inactivity heuristic.
//
// Priority: evdev SW_TABLET_MODE > logind TabletMode > keyboard heuristic.
package tabletmode

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-evdev"
)

const (
	logindDest          = "org.freedesktop.login1"
	logindPath          = "/org/freedesktop/login1"
	logindManager       = "org.freedesktop.login1.Manager"
	logindSessionIface  = "org.freedesktop.login1.Session"
	logindProp          = "TabletMode"
	kbInactivityTimeout = 30 * time.Second
)

// virtualKeyboardNames matches device names that are NOT real physical keyboards.
var virtualKeyboardNames = []string{
	"snry-osk-virtual",
	"ydotoold",
	"virtual",
	"power-button",
	"sleep-button",
	"lid-switch",
}

// Monitor watches for tablet mode changes and calls cb with the current state.
type Monitor struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	callback func(tablet bool)
	tablet   atomic.Bool

	logindMode string // "enabled", "disabled", "indeterminate"
	kbActive   bool
	kbTimer    *time.Timer
	hasTouch   bool
	session    string

	evdevTablet    bool
	evdevAvailable bool
}

// New creates a Monitor and detects initial touch device availability.
func New(conn *dbus.Conn, cb func(tablet bool)) *Monitor {
	return &Monitor{
		conn:     conn,
		callback: cb,
		hasTouch: detectTouchDevice(),
	}
}

// Run starts all monitors. Blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	go m.monitorEvdevSwitches(ctx)
	go m.monitorLogind(ctx)
	go m.monitorKeyboard(ctx)

	// Publish initial state after monitors have time to detect.
	time.AfterFunc(500*time.Millisecond, func() { m.publish() })

	<-ctx.Done()
}

func (m *Monitor) publish() {
	m.mu.Lock()
	evdevAvail := m.evdevAvailable
	evdevTab := m.evdevTablet
	logind := m.logindMode
	kb := m.kbActive
	touch := m.hasTouch
	m.mu.Unlock()

	tablet := false
	if evdevAvail {
		tablet = evdevTab
	} else {
		switch logind {
		case "enabled":
			tablet = true
		case "disabled":
			tablet = false
		default:
			tablet = !kb && touch
		}
	}

	log.Printf("[tabletmode] evdev=%v(logind=%s) kb=%v touch=%v -> tablet=%v",
		evdevTab, logind, kb, touch, tablet)
	m.tablet.Store(tablet)
	m.callback(tablet)
}

// ── evdev switch monitor ─────────────────────────────────────────────────────

func (m *Monitor) monitorEvdevSwitches(ctx context.Context) {
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		log.Printf("[tabletmode] evdev enumeration: %v", err)
		return
	}

	for _, p := range paths {
		dev, err := evdev.OpenWithFlags(p.Path, os.O_RDONLY)
		if err != nil {
			continue
		}

		hasSW := slices.Contains(dev.CapableTypes(), evdev.EV_SW)
		if !hasSW {
			dev.Close()
			continue
		}

		hasTabletMode := slices.Contains(dev.CapableEvents(evdev.EV_SW), evdev.SW_TABLET_MODE)
		if !hasTabletMode {
			dev.Close()
			continue
		}

		log.Printf("[tabletmode] found SW_TABLET_MODE device: %s (%s)", p.Name, p.Path)

		state, err := dev.State(evdev.EV_SW)
		if err != nil {
			log.Printf("[tabletmode] evdev state read: %v", err)
		} else {
			m.mu.Lock()
			m.evdevAvailable = true
			m.evdevTablet = state[evdev.SW_TABLET_MODE]
			m.mu.Unlock()
			m.publish()
		}

		dev.NonBlock()
		go m.readSwitchEvents(ctx, dev)
		return
	}

	log.Printf("[tabletmode] no SW_TABLET_MODE device found, using logind/heuristic fallback")
}

func (m *Monitor) readSwitchEvents(ctx context.Context, dev *evdev.InputDevice) {
	defer dev.Close()

	for {
		event, err := dev.ReadOne()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		if event.Type == evdev.EV_SW && event.Code == evdev.SW_TABLET_MODE {
			m.mu.Lock()
			m.evdevAvailable = true
			m.evdevTablet = event.Value != 0
			m.mu.Unlock()
			m.publish()
		}
	}
}

// ── logind monitor ──────────────────────────────────────────────────────────

func (m *Monitor) monitorLogind(ctx context.Context) {
	if m.conn == nil {
		log.Printf("[tabletmode] no system bus, skipping logind monitor")
		return
	}
	session, err := m.resolveSession()
	if err != nil {
		log.Printf("[tabletmode] cannot resolve session: %v", err)
		return
	}
	m.session = session

	ch := make(chan *dbus.Signal, 16)
	m.conn.Signal(ch)
	defer m.conn.RemoveSignal(ch)

	if err := m.conn.AddMatchSignal(dbus.WithMatchObjectPath(dbus.ObjectPath(session))); err != nil {
		log.Printf("[tabletmode] AddMatchSignal: %v", err)
		return
	}

	m.queryLogind()

	for {
		select {
		case <-ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if sig.Path != dbus.ObjectPath(session) {
				continue
			}
			m.queryLogind()
		}
	}
}

func (m *Monitor) resolveSession() (string, error) {
	if m.conn == nil {
		return "", fmt.Errorf("no d-bus connection")
	}

	var sessionPath dbus.ObjectPath
	mgr := m.conn.Object(logindDest, dbus.ObjectPath(logindPath))

	err := mgr.Call(logindManager+".GetSessionByPID", 0, uint32(os.Getpid())).Store(&sessionPath)
	if err == nil {
		return string(sessionPath), nil
	}

	sessionID := os.Getenv("XDG_SESSION_ID")
	if sessionID != "" {
		return string(dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/login1/session/%s", sessionID))), nil
	}

	return "", fmt.Errorf("could not determine logind session path: %v", err)
}

func (m *Monitor) queryLogind() {
	if m.conn == nil || m.session == "" {
		return
	}
	obj := m.conn.Object(logindDest, dbus.ObjectPath(m.session))
	v, err := obj.GetProperty(logindSessionIface + "." + logindProp)
	if err != nil {
		m.mu.Lock()
		m.logindMode = "indeterminate"
		m.mu.Unlock()
		return
	}
	mode, ok := v.Value().(string)
	if !ok {
		m.mu.Lock()
		m.logindMode = "indeterminate"
		m.mu.Unlock()
		return
	}
	m.mu.Lock()
	m.logindMode = mode
	m.mu.Unlock()
	m.publish()
}

// ── keyboard activity monitor ──────────────────────────────────────────────

func (m *Monitor) monitorKeyboard(ctx context.Context) {
	devices := findPhysicalKeyboardDevices()
	if len(devices) == 0 {
		log.Printf("[tabletmode] no physical keyboard devices found")
		return
	}
	log.Printf("[tabletmode] monitoring %d keyboard device(s)", len(devices))

	for _, dev := range devices {
		dev.NonBlock()
		go m.readKeyboard(ctx, dev)
	}
}

func (m *Monitor) readKeyboard(ctx context.Context, dev *evdev.InputDevice) {
	defer dev.Close()

	for {
		event, err := dev.ReadOne()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		if event.Type == evdev.EV_KEY && event.Value == 1 {
			m.onKeyboardActivity()
		}
	}
}

func (m *Monitor) onKeyboardActivity() {
	m.mu.Lock()
	m.kbActive = true
	if m.kbTimer != nil {
		m.kbTimer.Stop()
	}
	m.kbTimer = time.AfterFunc(kbInactivityTimeout, func() {
		m.mu.Lock()
		m.kbActive = false
		m.mu.Unlock()
		m.publish()
	})
	m.mu.Unlock()
	m.publish()
}

// ── evdev device helpers ────────────────────────────────────────────────────

func findPhysicalKeyboardDevices() []*evdev.InputDevice {
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		log.Printf("[tabletmode] evdev enumeration: %v", err)
		return nil
	}

	var devices []*evdev.InputDevice
	for _, p := range paths {
		if isVirtualKeyboard(p.Name) {
			continue
		}

		dev, err := evdev.OpenWithFlags(p.Path, os.O_RDONLY)
		if err != nil {
			continue
		}

		hasKey := slices.Contains(dev.CapableTypes(), evdev.EV_KEY)
		if !hasKey {
			dev.Close()
			continue
		}

		hasKeyA := slices.Contains(dev.CapableEvents(evdev.EV_KEY), 0x1e)
		if !hasKeyA {
			dev.Close()
			continue
		}

		devices = append(devices, dev)
	}
	return devices
}

func detectTouchDevice() bool {
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return false
	}

	for _, p := range paths {
		dev, err := evdev.OpenWithFlags(p.Path, os.O_RDONLY)
		if err != nil {
			continue
		}

		hasABS := slices.Contains(dev.CapableTypes(), evdev.EV_ABS)
		if !hasABS {
			dev.Close()
			continue
		}

		if slices.Contains(dev.CapableEvents(evdev.EV_ABS), evdev.ABS_MT_POSITION_X) {
			dev.Close()
			return true
		}
		dev.Close()
	}
	return false
}

func isVirtualKeyboard(name string) bool {
	lower := strings.ToLower(name)
	for _, pat := range virtualKeyboardNames {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

// EmitSnapshot implements socket.SnapshotProvider so new QML clients
// receive the current tablet mode state on connect.
func (m *Monitor) EmitSnapshot(emit func(map[string]any)) {
	emit(map[string]any{
		"event": "tablet_mode",
		"data":  map[string]any{"active": m.tablet.Load()},
	})
}
