package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprland"
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
func Autoscale(ctx context.Context, hl hyprland.API) error {
	// Check Hyprland is running
	if hl == nil || !hl.IsRunning() {
		return fmt.Errorf("hyprland not available")
	}

	out, err := hl.GetMonitors()
	if err != nil {
		return fmt.Errorf("hyprland monitors: %w", err)
	}

	var monitors []monitor
	if err := json.Unmarshal(out, &monitors); err != nil {
		return fmt.Errorf("parse monitor data: %w", err)
	}

	changed := false
	for _, m := range monitors {
		idealScale := math.Round(math.Max(1.0, float64(m.Height)/1080.0)*4) / 4.0
		idealScale = math.Round(idealScale*100) / 100

		fmt.Printf("  %s: %dx%d (current: %.2f, ideal: %.2f)\n",
			m.Name, m.Width, m.Height, m.Scale, idealScale)

		if m.Scale != idealScale {
			err := hl.SetMonitor(m.Name, fmt.Sprintf("%dx%d@", m.Width, m.Height), fmt.Sprintf("%dx%d", m.X, m.Y), idealScale)
			if err != nil {
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

// formatRefresh formats a resolution+refresh as a Hyprland mode string.
// Integer refresh rates omit the decimal (e.g. "@60" not "@60.00").
func formatRefresh(w, h int, rr float64) string {
	s := strconv.FormatFloat(rr, 'f', -1, 64)
	return fmt.Sprintf("%dx%d@%s", w, h, s)
}

// formatScale formats a scale factor, omitting unnecessary trailing zeros.
// E.g. 1.0 → "1", 1.25 → "1.25", 2.0 → "2".
func formatScale(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64)
}

// GenerateMonitorsLua detects connected monitors via hyprctl and writes a
// monitors.lua file with optimal resolution and refresh rate for each display.
// If the file already exists, it is regenerated only if the monitor topology
// has changed.
func GenerateMonitorsLua(cfg Config, hl hyprland.API) error {
	// Check Hyprland is running
	if hl == nil || !hl.IsRunning() {
		return nil // not running, skip silently
	}

	out, err := hl.GetMonitors()
	if err != nil {
		return fmt.Errorf("hyprland monitors: %w", err)
	}

	var monitors []monitor
	if err := json.Unmarshal(out, &monitors); err != nil {
		return fmt.Errorf("parse monitor data: %w", err)
	}

	if len(monitors) == 0 {
		return nil
	}

	deployPath := cfg.XDG.ConfigHome + "/hypr/hyprland/monitors.lua"

	// Build the lua content
	var b strings.Builder
	b.WriteString("-- Auto-generated monitor configuration\n")
	b.WriteString("-- DO NOT EDIT: regenerated on each sync. Override in custom/general.lua\n\n")

	for _, m := range monitors {
		idealScale := math.Round(math.Max(1.0, float64(m.Height)/1080.0)*4) / 4.0
		idealScale = math.Round(idealScale*100) / 100

		mode := formatRefresh(m.Width, m.Height, m.RefreshRate)

		// Use available modes to find the best resolution + refresh rate
		if len(m.AvailableModes) > 0 {
			best := bestMode(m.AvailableModes)
			if best.width > 0 {
				mode = formatRefresh(best.width, best.height, best.refreshRate)
				idealScale = math.Round(math.Max(1.0, float64(best.height)/1080.0)*4) / 4.0
				idealScale = math.Round(idealScale*100) / 100
			}
		}

		pos := fmt.Sprintf("%dx%d", m.X, m.Y)
		scaleStr := formatScale(idealScale)

		fmt.Fprintf(&b, "hl.monitor({ output = %q, mode = %q, position = %q, scale = %s",
			m.Name, mode, pos, scaleStr)

		if m.Transform != 0 {
			fmt.Fprintf(&b, ", transform = %d", m.Transform)
		}

		if m.VRR {
			b.WriteString(", vrr = 1")
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

// GenerateWorkspacesLua splits workspaces 1-10 equally across connected monitors
// and writes workspace_rule entries to workspaces.lua.
func GenerateWorkspacesLua(cfg Config, hl hyprland.API) error {
	if hl == nil || !hl.IsRunning() {
		return nil
	}

	out, err := hl.GetMonitors()
	if err != nil {
		return fmt.Errorf("hyprland monitors: %w", err)
	}

	var monitors []monitor
	if err := json.Unmarshal(out, &monitors); err != nil {
		return fmt.Errorf("parse monitor data: %w", err)
	}

	if len(monitors) == 0 {
		return nil
	}

	deployPath := cfg.XDG.ConfigHome + "/hypr/hyprland/workspaces.lua"
	totalWS := 10
	wsPerMonitor := totalWS / len(monitors)

	var b strings.Builder
	b.WriteString("-- Auto-generated workspace configuration\n")
	b.WriteString("-- DO NOT EDIT: regenerated on each sync. Override in custom/rules.lua\n\n")

	for i, m := range monitors {
		start := i*wsPerMonitor + 1
		end := start + wsPerMonitor - 1
		if i == len(monitors)-1 {
			end = totalWS // last monitor gets remainder
		}
		for ws := start; ws <= end; ws++ {
			opts := fmt.Sprintf("monitor = %q", m.Name)
			if ws == start {
				opts += ", default = true"
			}
			fmt.Fprintf(&b, "hl.workspace_rule({ workspace = %q, %s })\n", strconv.Itoa(ws), opts)
		}
	}

	if err := os.MkdirAll(filepath.Dir(deployPath), 0o755); err != nil {
		return err
	}

	existing, err := os.ReadFile(deployPath)
	if err == nil && string(existing) == b.String() {
		return nil
	}

	fmt.Printf("  Writing workspace config to %s\n", deployPath)
	return os.WriteFile(deployPath, []byte(b.String()), 0o644)
}
