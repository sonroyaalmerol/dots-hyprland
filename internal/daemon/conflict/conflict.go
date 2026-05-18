package conflict

import (
	"log"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/proc"
)

// tray processes that indicate a system tray is running.
var trayProcesses = map[string]bool{
	"kded6": true,
}

// notification processes that indicate a notification daemon is running.
var notificationProcesses = map[string]bool{
	"mako":  true,
	"dunst": true,
}

type Service struct{}

func New() *Service {
	return &Service{}
}

// Check scans /proc for known conflicting tray and notification daemons.
// Returns a map with "trays" and "notifications" keys containing lists of found process names.
func (s *Service) Check() map[string][]string {
	foundTrays := make([]string, 0)
	foundNotifications := make([]string, 0)

	proc.ForEachComm(func(procName string, _ int) bool {
		if trayProcesses[procName] {
			foundTrays = append(foundTrays, procName)
		}
		if notificationProcesses[procName] {
			foundNotifications = append(foundNotifications, procName)
		}
		return true
	})

	log.Printf("[conflict] check complete: trays=%v notifications=%v", foundTrays, foundNotifications)
	return map[string][]string{
		"trays":         foundTrays,
		"notifications": foundNotifications,
	}
}
