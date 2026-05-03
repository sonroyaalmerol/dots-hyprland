package sysinfo

import (
	"bufio"
	"context"
	"maps"
	"os"
	"strings"
	"sync"
)

type Service struct {
	mu       sync.RWMutex
	data     map[string]any
	callback func(map[string]any)
}

func New(cb func(map[string]any)) *Service {
	return &Service{callback: cb}
}

func (s *Service) Run(ctx context.Context) error {
	s.load()
	s.emit()
	<-ctx.Done()
	return ctx.Err()
}

func (s *Service) load() {
	data := make(map[string]any)

	// Parse /etc/os-release
	file, err := os.Open("/etc/os-release")
	if err == nil {
		defer file.Close()
		props := make(map[string]string)
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.IndexByte(line, '='); idx > 0 {
				key := line[:idx]
				val := strings.Trim(strings.Trim(line[idx+1:], `"`), "'")
				props[key] = val
			}
		}

		if pn, ok := props["PRETTY_NAME"]; ok {
			data["distroName"] = pn
		} else if n, ok := props["NAME"]; ok {
			data["distroName"] = strings.TrimSpace(strings.ReplaceAll(n, "Linux", ""))
		} else {
			data["distroName"] = "Unknown"
		}

		data["distroId"] = props["ID"]
		if data["distroId"] == "" {
			data["distroId"] = "unknown"
		}

		data["distroIcon"] = distroIcon(props["ID"], props["PRETTY_NAME"])

		data["homeUrl"] = props["HOME_URL"]
		data["documentationUrl"] = props["DOCUMENTATION_URL"]
		data["supportUrl"] = props["SUPPORT_URL"]
		data["bugReportUrl"] = props["BUG_REPORT_URL"]
		data["privacyPolicyUrl"] = props["PRIVACY_POLICY_URL"]
		data["logo"] = props["LOGO"]
		if data["logo"] == "" {
			data["logo"] = data["distroIcon"]
		}
	}

	data["username"] = os.Getenv("USER")
	if data["username"] == "" {
		data["username"] = "user"
	}

	data["desktopEnvironment"] = os.Getenv("XDG_CURRENT_DESKTOP")

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		data["windowingSystem"] = "Wayland"
	} else {
		data["windowingSystem"] = "X11"
	}

	s.mu.Lock()
	s.data = data
	s.mu.Unlock()
}

func distroIcon(id, pretty string) string {
	if strings.Contains(strings.ToLower(pretty), "nyarch") {
		return "nyarch-symbolic"
	}
	switch id {
	case "arch", "artix":
		return "arch-symbolic"
	case "endeavouros":
		return "endeavouros-symbolic"
	case "cachyos":
		return "cachyos-symbolic"
	case "nixos":
		return "nixos-symbolic"
	case "fedora":
		return "fedora-symbolic"
	case "linuxmint", "ubuntu", "zorin", "pop!_os":
		return "ubuntu-symbolic"
	case "debian", "raspbian", "kali":
		return "debian-symbolic"
	case "funtoo", "gentoo":
		return "gentoo-symbolic"
	default:
		return "linux-symbolic"
	}
}

func (s *Service) emit() {
	s.mu.RLock()
	data := make(map[string]any, len(s.data))
	maps.Copy(data, s.data)
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "system_info",
		"data":  data,
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.mu.RLock()
	data := make(map[string]any, len(s.data))
	maps.Copy(data, s.data)
	s.mu.RUnlock()

	callback(map[string]any{
		"event": "system_info",
		"data":  data,
	})
}
