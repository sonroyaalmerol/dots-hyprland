package manager

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: PersistMonitorConfig must actually write to the config file.
func TestPersistMonitorConfigWritesToFile(t *testing.T) {
	dir := t.TempDir()
	generalConf := filepath.Join(dir, "hypr", "hyprland", "general.conf")
	if err := os.MkdirAll(filepath.Dir(generalConf), 0o755); err != nil {
		t.Fatal(err)
	}
	initialContent := "# Original monitor config\nmonitor = eDP-1, 2560x1600, 0x0, 1.00\n"
	if err := os.WriteFile(generalConf, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	oldConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", dir)
	defer os.Setenv("XDG_CONFIG_HOME", oldConfigHome)

	if err := PersistMonitorConfig([]monitor{
		{Name: "eDP-1", Width: 1920, Height: 1080, Scale: 1.0, X: 0, Y: 0},
	}); err != nil {
		t.Fatalf("PersistMonitorConfig error: %v", err)
	}

	data, err := os.ReadFile(generalConf)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) == initialContent {
		t.Error("file unchanged — PersistMonitorConfig did not write")
	}
	if len(data) == 0 {
		t.Error("file is empty after PersistMonitorConfig")
	}
}
