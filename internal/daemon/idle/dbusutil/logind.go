package dbusutil

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

const (
	LogindDest    = "org.freedesktop.login1"
	LogindPath    = "/org/freedesktop/login1"
	LogindManager = "org.freedesktop.login1.Manager"
	LogindSession = "org.freedesktop.login1.Session"
)

func GetSessionPath(conn *dbus.Conn) (dbus.ObjectPath, error) {
	if conn == nil {
		return "", fmt.Errorf("no d-bus connection")
	}

	var sessionPath dbus.ObjectPath
	mgr := conn.Object(LogindDest, LogindPath)

	err := mgr.Call(LogindManager+".GetSessionByPID", 0, uint32(os.Getpid())).Store(&sessionPath)
	if err == nil {
		return sessionPath, nil
	}

	sessionID := os.Getenv("XDG_SESSION_ID")
	if sessionID != "" {
		return dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/login1/session/%s", sessionID)), nil
	}

	return "", fmt.Errorf("could not determine logind session path: %v", err)
}

func LogindSuspend(conn *dbus.Conn) error {
	if conn == nil {
		return fmt.Errorf("no d-bus connection")
	}
	return conn.Object(LogindDest, LogindPath).
		Call(LogindManager+".Suspend", 0, false).Err
}

func LogindReboot(conn *dbus.Conn) error {
	if conn == nil {
		return fmt.Errorf("no d-bus connection")
	}
	return conn.Object(LogindDest, LogindPath).
		Call(LogindManager+".Reboot", 0, false).Err
}

func LogindPowerOff(conn *dbus.Conn) error {
	if conn == nil {
		return fmt.Errorf("no d-bus connection")
	}
	return conn.Object(LogindDest, LogindPath).
		Call(LogindManager+".PowerOff", 0, false).Err
}

func LogindInhibit(conn *dbus.Conn, what, who, why, mode string) (dbus.UnixFD, error) {
	if conn == nil {
		return 0, fmt.Errorf("no d-bus connection")
	}
	var fd dbus.UnixFD
	err := conn.Object(LogindDest, LogindPath).
		Call(LogindManager+".Inhibit", 0, what, who, why, mode).
		Store(&fd)
	return fd, err
}
