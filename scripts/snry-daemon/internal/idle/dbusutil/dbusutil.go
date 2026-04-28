package dbusutil

import "github.com/godbus/dbus/v5"

type DBusConn interface {
	Object(dest string, path dbus.ObjectPath) dbus.BusObject
	Signal(ch chan<- *dbus.Signal)
	RemoveSignal(ch chan<- *dbus.Signal)
	BusObject() dbus.BusObject
	AddMatchSignal(opts ...dbus.MatchOption) error
}

type RealConn struct {
	Conn *dbus.Conn
}

func (r *RealConn) Object(dest string, path dbus.ObjectPath) dbus.BusObject {
	if r.Conn == nil {
		return nil
	}
	return r.Conn.Object(dest, path)
}

func (r *RealConn) Signal(ch chan<- *dbus.Signal) {
	if r.Conn == nil {
		return
	}
	r.Conn.Signal(ch)
}

func (r *RealConn) RemoveSignal(ch chan<- *dbus.Signal) {
	if r.Conn == nil {
		return
	}
	r.Conn.RemoveSignal(ch)
}

func (r *RealConn) BusObject() dbus.BusObject {
	if r.Conn == nil {
		return nil
	}
	return r.Conn.BusObject()
}

func (r *RealConn) AddMatchSignal(opts ...dbus.MatchOption) error {
	if r.Conn == nil {
		return nil
	}
	return r.Conn.AddMatchSignal(opts...)
}

func NewRealConn(conn *dbus.Conn) *RealConn {
	return &RealConn{Conn: conn}
}
