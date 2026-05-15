package wallpaper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHexConversion(t *testing.T) {
	tests := []struct {
		hex  string
		argb uint32
	}{
		{"#FF0000", 0xFFFF0000},
		{"#00FF00", 0xFF00FF00},
		{"#0000FF", 0xFF0000FF},
		{"#191114", 0xFF191114},
		{"#000000", 0xFF000000},
		{"#FFFFFF", 0xFFFFFFFF},
	}
	for _, tc := range tests {
		got := HexToARGB(tc.hex)
		if got != tc.argb {
			t.Errorf("HexToARGB(%s) = %08X, want %08X", tc.hex, got, tc.argb)
		}
		back := ARGBToHex(tc.argb)
		if back != tc.hex {
			t.Errorf("ARGBToHex(%08X) = %s, want %s", tc.argb, back, tc.hex)
		}
	}
}

func TestHCTRoundtrip(t *testing.T) {
	// Test that converting a color to HCT and back produces the same color.
	// With the proper CAM16+HctSolver implementation, roundtrips should be exact.
	colors := []string{"#FF0000", "#00FF00", "#0000FF", "#191114", "#FFB0CC"}
	for _, hex := range colors {
		argb := HexToARGB(hex)
		hct := HCTFromARGB(argb)
		back := hct.ToARGB()
		r1 := int((argb >> 16) & 0xFF)
		g1 := int((argb >> 8) & 0xFF)
		b1 := int(argb & 0xFF)
		r2 := int((back >> 16) & 0xFF)
		g2 := int((back >> 8) & 0xFF)
		b2 := int(back & 0xFF)
		if r1 != r2 || g1 != g2 || b1 != b2 {
			t.Errorf("HCT roundtrip for %s: got (%d,%d,%d), want (%d,%d,%d), diff=(%d,%d,%d)",
				hex, r2, g2, b2, r1, g1, b1, r1-r2, g1-g2, b1-b2)
		}
	}
}

func TestHarmonize(t *testing.T) {
	// Test that harmonization doesn't crash and produces valid colors
	designColor := HexToARGB("#B52755") // term1 (red)
	sourceColor := HexToARGB("#FFB0CC") // primary (pink)
	result := Harmonize(designColor, sourceColor, 100, 0.8)
	hex := ARGBToHex(result)
	if hex == "#000000" || hex == "" {
		t.Errorf("Harmonize produced invalid color: %s", hex)
	}
}

func TestBoostChromaTone(t *testing.T) {
	argb := HexToARGB("#191114") // surface (dark)
	boosted := BoostChromaTone(argb, 1.2, 0.95)
	hex := ARGBToHex(boosted)
	if hex == "#000000" || hex == "" {
		t.Errorf("BoostChromaTone produced invalid color: %s", hex)
	}
}

func TestLoadColorMapFromJSON(t *testing.T) {
	tmpDir := t.TempDir()
	colors := map[string]string{
		"background": "#191114",
		"primary":    "#FFB0CC",
		"surface":    "#191114",
	}
	data, _ := json.MarshalIndent(colors, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "colors.json"), data, 0644)

	loaded, err := LoadColorMapFromJSON(filepath.Join(tmpDir, "colors.json"))
	if err != nil {
		t.Fatalf("LoadColorMapFromJSON: %v", err)
	}
	if loaded["background"] != "#191114" {
		t.Errorf("expected background=#191114, got %s", loaded["background"])
	}
	if loaded["primary"] != "#FFB0CC" {
		t.Errorf("expected primary=#FFB0CC, got %s", loaded["primary"])
	}
}

func TestLoadTerminalScheme(t *testing.T) {
	tmpDir := t.TempDir()
	scheme := map[string]any{
		"dark": map[string]string{
			"term0": "#282828",
			"term1": "#CC241D",
		},
		"light": map[string]string{
			"term0": "#FDF9F3",
			"term1": "#FF6188",
		},
	}
	data, _ := json.MarshalIndent(scheme, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "scheme-base.json"), data, 0644)

	loaded, err := LoadTerminalScheme(filepath.Join(tmpDir, "scheme-base.json"))
	if err != nil {
		t.Fatalf("LoadTerminalScheme: %v", err)
	}
	if loaded.Dark["term0"] != "#282828" {
		t.Errorf("expected dark term0=#282828, got %s", loaded.Dark["term0"])
	}
	if loaded.Light["term0"] != "#FDF9F3" {
		t.Errorf("expected light term0=#FDF9F3, got %s", loaded.Light["term0"])
	}
}

func TestGenerateTerminalColors(t *testing.T) {
	colors := ColorMap{
		"background":             "#191114",
		"primary":                "#FFB0CC",
		"surface":                "#191114",
		"surface_container_low":  "#21191c",
		"on_surface":             "#EEDFE2",
		"on_secondary_container": "#FFD9E4",
	}

	scheme := &TerminalScheme{
		Dark: map[string]string{
			"term0": "#282828", "term1": "#CC241D", "term15": "#EBDBB2",
		},
		Light: map[string]string{
			"term0": "#FDF9F3", "term1": "#FF6188", "term15": "#333034",
		},
	}

	termColors := GenerateTerminalColors(colors, scheme, true, 0.8, 100, 0.35)
	if len(termColors) == 0 {
		t.Error("GenerateTerminalColors returned empty map")
	}
	// term0 should be modified (boosted)
	if _, ok := termColors["term0"]; !ok {
		t.Error("term0 not in result")
	}
	// All colors should be valid hex
	for name, hex := range termColors {
		if len(hex) != 7 || hex[0] != '#' {
			t.Errorf("invalid hex color for %s: %s", name, hex)
		}
	}
}

func TestGenerateGhosttyConfig(t *testing.T) {
	colors := ColorMap{
		"surface":                "#191114",
		"on_surface":             "#EEDFE2",
		"on_secondary_container": "#FFD9E4",
		"secondary_container":    "#5A3F49",
		"primary":                "#FFB0CC",
		"primary_container":      "#6F334C",
		"secondary":              "#E2BDC8",
		"tertiary":               "#F0BC96",
		"tertiary_container":     "#623F21",
		"error":                  "#FFB4AB",
		"error_container":        "#93000A",
		"on_primary":             "#541D35",
		"on_primary_container":   "#FFD9E4",
		"on_secondary":           "#422932",
		"on_tertiary":            "#48290D",
		"on_tertiary_container":  "#FFDCC4",
		"on_error":               "#690005",
		"on_error_container":     "#FFDAD6",
		"outline_variant":        "#514347",
	}

	termColors := map[string]string{
		"term0": "#282828", "term1": "#CC241D", "term15": "#EBDBB2",
	}

	config := GenerateGhosttyConfig(colors, termColors, true)

	// Should start with background
	if len(config) == 0 {
		t.Error("GenerateGhosttyConfig returned empty string")
	}

	// Should NOT contain template placeholders like $surface
	if containsAny(config, "$surface", "$onSurface", "$primary") {
		t.Errorf("ghostty config contains unresolved template placeholders")
	}

	// Should have valid hex values
	if !containsLine(config, "background = #") {
		t.Errorf("ghostty config missing background line")
	}
	if !containsLine(config, "foreground = #") {
		t.Errorf("ghostty config missing foreground line")
	}
	if !containsLine(config, "palette = 0=#") {
		t.Errorf("ghostty config missing palette entry")
	}
}

func TestSnakeToCamel(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"background", "background"},
		{"on_surface", "onSurface"},
		{"surface_container_low", "surfaceContainerLow"},
		{"primary", "primary"},
		{"error_container", "errorContainer"},
	}
	for _, tc := range tests {
		got := snakeToCamel(tc.input)
		if got != tc.want {
			t.Errorf("snakeToCamel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSchemeDetection(t *testing.T) {
	tests := []struct {
		input string
		want  SchemeType
	}{
		{"scheme-tonal-spot", SchemeTonalSpot},
		{"scheme-neutral", SchemeNeutral},
		{"auto", SchemeAuto},
		{"invalid", SchemeTonalSpot},
		{"", SchemeTonalSpot},
	}
	for _, tc := range tests {
		got := ParseSchemeType(tc.input)
		if got != tc.want {
			t.Errorf("ParseSchemeType(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestSCSSGeneration(t *testing.T) {
	colors := ColorMap{
		"background": "#191114",
		"primary":    "#FFB0CC",
	}
	termColors := map[string]string{
		"term0": "#282828",
		"term1": "#CC241D",
	}

	scss := GenerateSCSSOutput(colors, termColors, true, false)

	if !containsLine(scss, "$background: #191114") {
		t.Errorf("SCSS missing $background line")
	}
	if !containsLine(scss, "$darkmode: true") {
		t.Errorf("SCSS missing $darkmode line")
	}
	if !containsLine(scss, "$term0: #282828") {
		t.Errorf("SCSS missing $term0 line")
	}
}

func containsLine(s, substr string) bool {
	return len(s) > 0 && containsAny(s, substr)
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
