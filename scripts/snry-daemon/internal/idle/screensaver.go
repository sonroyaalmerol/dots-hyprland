package idle

import (
	"sync"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

const (
	screenSaverName  = "org.freedesktop.ScreenSaver"
	screenSaverPath  = "/org/freedesktop/ScreenSaver"
	screenSaverIface = "org.freedesktop.ScreenSaver"
)

// ScreenSaver implements org.freedesktop.ScreenSaver on the session bus,
// allowing media players and other apps to inhibit idle.
type ScreenSaver struct {
	b          *bus
	inhibitors sync.Map // uint32 -> string (app name)
	id         atomic.Uint32
}

func newScreenSaver(b *bus) *ScreenSaver {
	return &ScreenSaver{b: b}
}

func (ss *ScreenSaver) Inhibit(appName string, reason string) (uint32, *dbus.Error) {
	id := ss.id.Add(1)
	ss.inhibitors.Store(id, appName)
	ss.b.publish(topicIdleInhibit, true)
	return id, nil
}

func (ss *ScreenSaver) UnInhibit(id uint32) *dbus.Error {
	ss.inhibitors.Delete(id)
	empty := true
	ss.inhibitors.Range(func(_, _ any) bool {
		empty = false
		return false
	})
	if empty {
		ss.b.publish(topicIdleInhibit, false)
	}
	return nil
}

func (ss *ScreenSaver) Lock() *dbus.Error {
	ss.b.publish(topicScreenLock, true)
	return nil
}

func (ss *ScreenSaver) SimulateUserActivity() *dbus.Error { return nil }
func (ss *ScreenSaver) GetActive() (bool, *dbus.Error)    { return false, nil }

func registerScreenSaver(conn *dbus.Conn, ss *ScreenSaver) error {
	if err := conn.Export(ss, screenSaverPath, screenSaverIface); err != nil {
		return err
	}
	_, err := conn.RequestName(screenSaverName, dbus.NameFlagReplaceExisting)
	return err
}
