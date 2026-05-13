package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type monitor struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	RefreshRate    float64  `json:"refreshRate"`
	Scale          float64  `json:"scale"`
	X              int      `json:"x"`
	Y              int      `json:"y"`
	Transform      int      `json:"transform"`
	VRR            bool     `json:"vrr"`
	AvailableModes []string `json:"availableModes"`
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

// getMonitorConfigPath returns the likely hyprland config path.
func getMonitorConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = home + "/.config"
	}
	luaPath := configHome + "/hypr/hyprland/general.lua"
	if _, err := os.Stat(luaPath); err == nil {
		return luaPath
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

	// Read existing content
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	// Replace existing monitor= lines or append
	configLine := strings.Join(lines, "\n")
	existing := string(data)
	if strings.Contains(existing, "monitor=") {
		// Replace first monitor= line and remove others
		linesSlice := strings.Split(existing, "\n")
		var newLines []string
		monitorWritten := false
		for _, line := range linesSlice {
			if strings.HasPrefix(strings.TrimSpace(line), "monitor=") {
				if !monitorWritten {
					newLines = append(newLines, strings.Split(configLine, "\n")...)
					monitorWritten = true
				}
				continue
			}
			newLines = append(newLines, line)
		}
		if !monitorWritten {
			newLines = append(newLines, strings.Split(configLine, "\n")...)
		}
		existing = strings.Join(newLines, "\n")
	} else {
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		existing += configLine + "\n"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	fmt.Printf("  Writing monitor config to %s\n", path)
	return os.WriteFile(path, []byte(existing), 0o644)
}

// modeInfo holds parsed resolution and refresh rate from an available mode string.
type modeInfo struct {
	width       int
	height      int
	refreshRate float64
}

// parseMode parses a mode string like "2560x1440@180.00Hz" into components.
func parseMode(s string) (modeInfo, bool) {
	s = strings.TrimSuffix(s, "Hz")
	parts := strings.SplitN(s, "@", 2)
	if len(parts) != 2 {
		return modeInfo{}, false
	}
	resParts := strings.SplitN(parts[0], "x", 2)
	if len(resParts) != 2 {
		return modeInfo{}, false
	}
	w, errW := strconv.Atoi(resParts[0])
	h, errH := strconv.Atoi(resParts[1])
	rr, errR := strconv.ParseFloat(parts[1], 64)
	if errW != nil || errH != nil || errR != nil {
		return modeInfo{}, false
	}
	return modeInfo{width: w, height: h, refreshRate: rr}, true
}

// bestMode picks the highest resolution, then highest refresh rate among
// modes with that resolution.
func bestMode(modes []string) modeInfo {
	var best modeInfo
	for _, m := range modes {
		info, ok := parseMode(m)
		if !ok {
			continue
		}
		pixelCount := info.width * info.height
		bestPixels := best.width * best.height
		if pixelCount > bestPixels || (pixelCount == bestPixels && info.refreshRate > best.refreshRate) {
			best = info
		}
	}
	return best
}

// GenerateMonitorsLua detects connected monitors via hyprctl and writes a
// monitors.lua file with optimal resolution and refresh rate for each display.
// If the file already exists, it is regenerated only if the monitor topology
// has changed.
func GenerateMonitorsLua(cfg Config) error {
	// Check Hyprland is running
	if err := exec.Command("hyprctl", "version").Run(); err != nil {
		return nil // not running, skip silently
	}

	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return fmt.Errorf("hyprctl monitors: %w", err)
	}

	var monitors []monitor
	if err := json.Unmarshal(out, &monitors); err != nil {
		return fmt.Errorf("parse monitor data: %w", err)
	}

	if len(monitors) == 0 {
		return nil
	}

	deployPath := cfg.XDG.ConfigHome + "/hypr/monitors.lua"

	// Build the lua content
	var b strings.Builder
	b.WriteString("-- Auto-generated monitor configuration\n")
	b.WriteString("-- DO NOT EDIT: regenerated on each sync. Override in custom/general.lua\n\n")

	for _, m := range monitors {
		idealScale := math.Round(math.Max(1.0, float64(m.Height)/1080.0)*4) / 4.0
		idealScale = math.Round(idealScale*100) / 100

		mode := fmt.Sprintf("%dx%d@%.2f", m.Width, m.Height, m.RefreshRate)

		// Use available modes to find the best resolution + refresh rate
		if len(m.AvailableModes) > 0 {
			best := bestMode(m.AvailableModes)
			if best.width > 0 {
				mode = fmt.Sprintf("%dx%d@%.0f", best.width, best.height, best.refreshRate)
				idealScale = math.Round(math.Max(1.0, float64(best.height)/1080.0)*4) / 4.0
				idealScale = math.Round(idealScale*100) / 100
			}
		}

		pos := fmt.Sprintf("%dx%d", m.X, m.Y)

		b.WriteString(fmt.Sprintf("hl.monitor({ output = %q, mode = %q, position = %q, scale = %.2f",
			m.Name, mode, pos, idealScale))

		if m.Transform != 0 {
			transforms := []string{"normal", "90", "180", "270", "flipped", "flipped-90", "flipped-180", "flipped-270"}
			if int(m.Transform) < len(transforms) {
				b.WriteString(fmt.Sprintf(", transform = %q", transforms[m.Transform]))
			}
		}

		b.WriteString(" })\n")
	}

	if err := os.MkdirAll(filepath.Dir(deployPath), 0o755); err != nil {
		return err
	}

	existing, err := os.ReadFile(deployPath)
	if err == nil && string(existing) == b.String() {
		return nil // no change
	}

	fmt.Printf("  Writing monitor config to %s\n", deployPath)
	return os.WriteFile(deployPath, []byte(b.String()), 0o644)
}
