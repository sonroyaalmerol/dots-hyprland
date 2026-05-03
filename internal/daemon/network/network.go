package network

import (
	"bufio"
	"context"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	PollInterval time.Duration
}

func DefaultConfig() Config {
	return Config{PollInterval: 10 * time.Second}
}

type WifiNetwork struct {
	Active   bool   `json:"active"`
	Strength int    `json:"strength"`
	Freq     int    `json:"frequency"`
	SSID     string `json:"ssid"`
	BSSID    string `json:"bssid"`
	Security string `json:"security"`
}

type State struct {
	WifiEnabled  bool          `json:"wifiEnabled"`
	WifiStatus   string        `json:"wifiStatus"`
	Ethernet     bool          `json:"ethernet"`
	Wifi         bool          `json:"wifi"`
	NetworkName  string        `json:"networkName"`
	Strength     int           `json:"networkStrength"`
	WifiNetworks []WifiNetwork `json:"wifiNetworks"`
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex
	state    State
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		callback: cb,
	}
}

func (s *Service) Run(ctx context.Context) error {
	monitorCtx, monitorCancel := context.WithCancel(ctx)
	go s.runMonitor(monitorCtx)

	s.pollAll(ctx)

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			monitorCancel()
			return ctx.Err()
		case <-ticker.C:
			s.pollAll(ctx)
		}
	}
}

func (s *Service) runMonitor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cmd := exec.CommandContext(ctx, "nmcli", "monitor")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("[network] monitor pipe: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if err := cmd.Start(); err != nil {
			log.Printf("[network] monitor start: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				cmd.Process.Kill()
				return
			default:
			}

			line := scanner.Text()
			if strings.Contains(line, "connected") || strings.Contains(line, "disconnected") ||
				strings.Contains(line, "wifi") || strings.Contains(line, "state changed") {
				s.pollAll(ctx)
			}
		}
		cmd.Wait()
	}
}

func (s *Service) pollAll(ctx context.Context) {
	s.pollConnectionType(ctx)
	s.pollWifiEnabled(ctx)
	s.pollNetworkName(ctx)
	s.pollNetworkStrength(ctx)
	s.pollWifiNetworks(ctx)
	s.emit()
}

func (s *Service) pollConnectionType(ctx context.Context) {
	out, err := exec.CommandContext(ctx, "sh", "-c",
		"nmcli -t -f TYPE,STATE d status && nmcli -t -f CONNECTIVITY g").Output()
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	connectivity := lines[len(lines)-1]

	var ethernet, wifi bool
	var wifiStatus string = "disconnected"

	for _, line := range lines[:len(lines)-1] {
		if strings.Contains(line, "ethernet") && strings.Contains(line, "connected") {
			ethernet = true
		} else if strings.Contains(line, "wifi") {
			if strings.Contains(line, "connected") {
				wifi = true
				wifiStatus = "connected"
				if connectivity == "limited" {
					wifi = false
					wifiStatus = "limited"
				}
			} else if strings.Contains(line, "disconnected") {
				wifiStatus = "disconnected"
			} else if strings.Contains(line, "connecting") {
				wifiStatus = "connecting"
			} else if strings.Contains(line, "unavailable") {
				wifiStatus = "disabled"
			}
		}
	}

	s.mu.Lock()
	s.state.Ethernet = ethernet
	s.state.Wifi = wifi
	s.state.WifiStatus = wifiStatus
	s.mu.Unlock()
}

func (s *Service) pollWifiEnabled(ctx context.Context) {
	out, err := exec.CommandContext(ctx, "nmcli", "radio", "wifi").Output()
	if err != nil {
		return
	}
	enabled := strings.TrimSpace(string(out)) == "enabled"
	s.mu.Lock()
	s.state.WifiEnabled = enabled
	s.mu.Unlock()
}

func (s *Service) pollNetworkName(ctx context.Context) {
	out, err := exec.CommandContext(ctx, "sh", "-c",
		"nmcli -t -f NAME c show --active | head -1").Output()
	if err != nil {
		return
	}
	s.mu.Lock()
	s.state.NetworkName = strings.TrimSpace(string(out))
	s.mu.Unlock()
}

func (s *Service) pollNetworkStrength(ctx context.Context) {
	out, err := exec.CommandContext(ctx, "sh", "-c",
		"nmcli -f IN-USE,SIGNAL,SSID device wifi | awk '/^\\*/{if (NR!=1) {print $2}}'").Output()
	if err != nil {
		return
	}
	strength, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	s.mu.Lock()
	s.state.Strength = strength
	s.mu.Unlock()
}

func (s *Service) pollWifiNetworks(ctx context.Context) {
	out, err := exec.CommandContext(ctx, "nmcli", "-g",
		"ACTIVE,SIGNAL,FREQ,SSID,BSSID,SECURITY", "d", "w").Output()
	if err != nil {
		return
	}

	placeholder := "STRINGWHICHHOPEFULLYWONTBEUSED"
	text := strings.TrimSpace(string(out))
	if text == "" {
		s.mu.Lock()
		s.state.WifiNetworks = nil
		s.mu.Unlock()
		return
	}

	networkMap := make(map[string]WifiNetwork)
	for n := range strings.SplitSeq(text, "\n") {
		unescaped := strings.ReplaceAll(n, "\\:", placeholder)
		fields := strings.Split(unescaped, ":")
		if len(fields) < 4 {
			continue
		}

		bssid := strings.ReplaceAll(fields[4], placeholder, ":")

		ssid := fields[3]
		if ssid == "" {
			continue
		}

		active := fields[0] == "yes"
		strength, _ := strconv.Atoi(fields[1])
		freq, _ := strconv.Atoi(fields[2])
		security := ""
		if len(fields) > 5 {
			security = fields[5]
		}

		net := WifiNetwork{
			Active:   active,
			Strength: strength,
			Freq:     freq,
			SSID:     ssid,
			BSSID:    bssid,
			Security: security,
		}

		existing, ok := networkMap[ssid]
		if !ok {
			networkMap[ssid] = net
		} else if active && !existing.Active {
			networkMap[ssid] = net
		} else if !active && !existing.Active && strength > existing.Strength {
			networkMap[ssid] = net
		}
	}

	networks := make([]WifiNetwork, 0, len(networkMap))
	for _, n := range networkMap {
		networks = append(networks, n)
	}

	s.mu.Lock()
	s.state.WifiNetworks = networks
	s.mu.Unlock()
}

func (s *Service) emit() {
	s.mu.RLock()
	stateCopy := s.state
	networks := make([]WifiNetwork, len(stateCopy.WifiNetworks))
	copy(networks, stateCopy.WifiNetworks)
	stateCopy.WifiNetworks = networks
	s.mu.RUnlock()

	s.callback(map[string]any{
		"event": "network",
		"data":  stateCopy,
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.emit()
}

// Commands

func (s *Service) EnableWifi(ctx context.Context, enabled bool) {
	cmd := "off"
	if enabled {
		cmd = "on"
	}
	exec.CommandContext(ctx, "nmcli", "radio", "wifi", cmd).Run()
	s.pollWifiEnabled(ctx)
	s.emit()
}

func (s *Service) ToggleWifi(ctx context.Context) {
	s.mu.RLock()
	enabled := s.state.WifiEnabled
	s.mu.RUnlock()
	s.EnableWifi(ctx, !enabled)
}

func (s *Service) RescanWifi(ctx context.Context) {
	exec.CommandContext(ctx, "nmcli", "dev", "wifi", "list", "--rescan", "yes").Run()
	s.pollWifiNetworks(ctx)
	s.emit()
}

func (s *Service) ConnectWifi(ctx context.Context, ssid string) error {
	return exec.CommandContext(ctx, "nmcli", "dev", "wifi", "connect", ssid).Run()
}

func (s *Service) ConnectWifiWithPassword(ctx context.Context, ssid, password string) error {
	return exec.CommandContext(ctx, "nmcli", "dev", "wifi", "connect", ssid, "password", password).Run()
}

func (s *Service) DisconnectWifi(ctx context.Context, ssid string) error {
	return exec.CommandContext(ctx, "nmcli", "connection", "down", ssid).Run()
}

func (s *Service) ChangePassword(ctx context.Context, ssid, password string) error {
	return exec.CommandContext(ctx, "sh", "-c",
		"nmcli connection modify \""+ssid+"\" wifi-sec.psk \""+password+"\"").Run()
}
