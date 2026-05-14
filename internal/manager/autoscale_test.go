package manager

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatRefresh(t *testing.T) {
	tests := []struct {
		w, h int
		rr   float64
		want string
	}{
		{1920, 1080, 60, "1920x1080@60"},
		{1920, 1080, 60.0, "1920x1080@60"},
		{2560, 1440, 165.0, "2560x1440@165"},
		{2560, 1440, 59.95, "2560x1440@59.95"},
		{3840, 2160, 144.001, "3840x2160@144.001"},
	}
	for _, tt := range tests {
		got := formatRefresh(tt.w, tt.h, tt.rr)
		if got != tt.want {
			t.Errorf("formatRefresh(%d, %d, %v) = %q, want %q", tt.w, tt.h, tt.rr, got, tt.want)
		}
	}
}

func TestFormatScale(t *testing.T) {
	tests := []struct {
		scale float64
		want  string
	}{
		{1.0, "1"},
		{1.25, "1.25"},
		{1.5, "1.5"},
		{2.0, "2"},
		{2.75, "2.75"},
		{1.00, "1"},
	}
	for _, tt := range tests {
		got := formatScale(tt.scale)
		if got != tt.want {
			t.Errorf("formatScale(%v) = %q, want %q", tt.scale, got, tt.want)
		}
	}
}

func TestGenerateMonitorsLuaTransform(t *testing.T) {
	// Verify transform is output as integer, not string
	monitors := []monitor{
		{Name: "eDP-1", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1.0, X: 0, Y: 0, Transform: 1},
	}
	var b strings.Builder
	b.WriteString("-- test\n\n")
	for _, m := range monitors {
		mode := formatRefresh(m.Width, m.Height, m.RefreshRate)
		pos := fmt.Sprintf("%dx%d", m.X, m.Y)
		scaleStr := formatScale(1.0)
		fmt.Fprintf(&b, "hl.monitor({ output = %q, mode = %q, position = %q, scale = %s", m.Name, mode, pos, scaleStr)
		if m.Transform != 0 {
			fmt.Fprintf(&b, ", transform = %d", m.Transform)
		}
		b.WriteString(" })\n")
	}
	output := b.String()

	if !strings.Contains(output, "transform = 1") {
		t.Errorf("expected transform as integer, got: %s", output)
	}
	if strings.Contains(output, `transform = "1"`) || strings.Contains(output, `transform = "90"`) {
		t.Errorf("transform should be integer not string, got: %s", output)
	}
}

func TestGenerateMonitorsLuaVRR(t *testing.T) {
	monitors := []monitor{
		{Name: "DP-1", Width: 2560, Height: 1440, RefreshRate: 165, Scale: 1.0, X: 0, Y: 0, VRR: true},
	}
	var b strings.Builder
	b.WriteString("-- test\n\n")
	for _, m := range monitors {
		mode := formatRefresh(m.Width, m.Height, m.RefreshRate)
		pos := fmt.Sprintf("%dx%d", m.X, m.Y)
		scaleStr := formatScale(1.0)
		fmt.Fprintf(&b, "hl.monitor({ output = %q, mode = %q, position = %q, scale = %s", m.Name, mode, pos, scaleStr)
		if m.VRR {
			b.WriteString(", vrr = 1")
		}
		b.WriteString(" })\n")
	}
	output := b.String()

	if !strings.Contains(output, "vrr = 1") {
		t.Errorf("expected vrr = 1, got: %s", output)
	}
}

func TestGenerateMonitorsLuaNoVRR(t *testing.T) {
	// VRR=false should not output vrr field at all
	monitors := []monitor{
		{Name: "DP-1", Width: 2560, Height: 1440, RefreshRate: 165, Scale: 1.0, X: 0, Y: 0, VRR: false},
	}
	var b strings.Builder
	b.WriteString("-- test\n\n")
	for _, m := range monitors {
		mode := formatRefresh(m.Width, m.Height, m.RefreshRate)
		pos := fmt.Sprintf("%dx%d", m.X, m.Y)
		scaleStr := formatScale(1.0)
		fmt.Fprintf(&b, "hl.monitor({ output = %q, mode = %q, position = %q, scale = %s", m.Name, mode, pos, scaleStr)
		b.WriteString(" })\n")
	}
	output := b.String()

	if strings.Contains(output, "vrr") {
		t.Errorf("vrr should not appear when VRR=false, got: %s", output)
	}
}
