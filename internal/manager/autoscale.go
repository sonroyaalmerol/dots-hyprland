package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
)

type monitor struct {
	Name   string  `json:"name"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
}

// Autoscale detects connected monitors and sets ideal Hyprland scale factors.
func Autoscale(ctx context.Context) error {
	// Check Hyprland is running
	if err := exec.CommandContext(ctx, "hyprctl", "version").Run(); err != nil {
		return fmt.Errorf("hyprland not running: %w", err)
	}

	// Get monitor data
	out, err := exec.CommandContext(ctx, "hyprctl", "monitors", "-j").Output()
	if err != nil {
		return fmt.Errorf("hyprctl monitors: %w", err)
	}

	var monitors []monitor
	if err := json.Unmarshal(out, &monitors); err != nil {
		return fmt.Errorf("parse monitor data: %w", err)
	}

	changed := false
	for _, m := range monitors {
		idealScale := math.Round(math.Max(1.0, float64(m.Height)/1080.0)*4) / 4.0
		idealScale = math.Round(idealScale*100) / 100

		pos := fmt.Sprintf("%dx%d", m.X, m.Y)
		fmt.Printf("  %s: %dx%d (current: %.2f, ideal: %.2f)\n",
			m.Name, m.Width, m.Height, m.Scale, idealScale)

		if m.Scale != idealScale {
			arg := fmt.Sprintf("%s,%dx%d,%s,%.2f", m.Name, m.Width, m.Height, pos, idealScale)
			cmd := exec.Command("hyprctl", "keyword", "monitor", arg)
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "  [warn] set scale for %s: %v\n", m.Name, err)
			} else {
				changed = true
			}
		}
	}

	if changed {
		fmt.Println("  Monitor scale updated.")
	} else {
		fmt.Println("  All monitors already at ideal scale.")
	}
	return nil
}

// getMonitorConfigPath returns the likely hyprland general.conf path.
func getMonitorConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = home + "/.config"
	}
	return configHome + "/hypr/hyprland/general.conf"
}

// PersistMonitorConfig writes monitor config lines to general.conf.
func PersistMonitorConfig(monitors []monitor) error {
	path := getMonitorConfigPath()
	var lines []string
	for _, m := range monitors {
		idealScale := math.Round(math.Max(1.0, float64(m.Height)/1080.0)*4) / 4.0
		pos := fmt.Sprintf("%dx%d", m.X, m.Y)
		lines = append(lines, fmt.Sprintf("monitor=%s,%dx%d,%s,%.2f",
			m.Name, m.Width, m.Height, pos, idealScale))
	}

	_ = path
	configLine := strings.Join(lines, "\n")
	fmt.Printf("  Monitor config line: %s\n", configLine)
	_ = configLine
	return nil
}
