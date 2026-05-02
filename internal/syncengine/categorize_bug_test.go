package syncengine

import "testing"

// Regression: matchPattern correctly handles ** globs.
func TestMatchPatternDoubleStarGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.conf", "hypr/hyprland.conf", true},
		{"**/*.conf", "deep/nested/file.conf", true},
		{"**/*.conf", "file.conf", true},
		{"**/*.conf", "ghostty/config", false},
		{"**/*.conf", "file.txt", false},
		{"fontconfig/**", "fontconfig/conf.d/fonts.conf", true},
		{"fontconfig/**", "fontconfig/fonts.conf", true},
		{"fontconfig/**", "other/fonts.conf", false},
		{"**", "anything/at/all", true},
		{"**", "single", true},
		{"hypr/hyprland/*.conf", "hypr/hyprland/general.conf", true},
		{"hypr/hyprland/*.conf", "hypr/other/general.conf", false},
	}

	for _, tt := range tests {
		got := matchPattern(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

// Regression: ghostty/config (no .conf extension) gets StrategyOverwrite, not StrategyMergeKV.
func TestCategorizeGhosttyConfig(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("ghostty/config")
	if s != StrategyOverwrite {
		t.Errorf("ghostty/config should get StrategyOverwrite, got %v", s)
	}
}
