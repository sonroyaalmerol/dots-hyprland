package conflict

import (
	"log"
	"os"
	"path/filepath"
	"strings"
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

	entries, err := os.ReadDir("/proc")
	if err != nil {
		log.Printf("[conflict] read /proc: %v", err)
		return map[string][]string{
			"trays":         foundTrays,
			"notifications": foundNotifications,
		}
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Only look at numeric directories (PID dirs).
		name := entry.Name()
		if len(name) == 0 || name[0] < '0' || name[0] > '9' {
			continue
		}

		comm, err := os.ReadFile(filepath.Join("/proc", name, "comm"))
		if err != nil {
			continue
		}
		procName := strings.TrimSpace(string(comm))
		if seen[procName] {
			continue
		}
		seen[procName] = true

		if trayProcesses[procName] {
			foundTrays = append(foundTrays, procName)
		}
		if notificationProcesses[procName] {
			foundNotifications = append(foundNotifications, procName)
		}
	}

	log.Printf("[conflict] check complete: trays=%v notifications=%v", foundTrays, foundNotifications)
	return map[string][]string{
		"trays":         foundTrays,
		"notifications": foundNotifications,
	}
}
