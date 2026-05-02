package manager

import (
	"os"
	"testing"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/xdg"
)

// Regression: smartSyncSteps must return an error for missing source directories.
func TestSmartSyncStepsErrorOnMissingDir(t *testing.T) {
	cfg := Config{
		RepoRoot: "/tmp/nonexistent-repo",
		Home:     "/tmp/testhome",
		XDG: xdg.Paths{
			ConfigHome: "/tmp/testhome/.config",
			CacheHome:  "/tmp/testhome/.cache",
			DataHome:   "/tmp/testhome/.local/share",
			StateHome:  "/tmp/testhome/.local/state",
			BinHome:    "/tmp/testhome/.local/bin",
		},
	}

	_, err := smartSyncSteps(cfg, "/nonexistent/path/that/does/not/exist", "/tmp/dst", "prefix")
	if err == nil {
		t.Error("expected error for missing source directory, got nil")
	}
}

// Regression: smartSyncSteps succeeds for a valid source directory.
func TestSmartSyncStepsSuccessOnValidDir(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot: dir,
		Home:     t.TempDir(),
		XDG: xdg.Paths{
			ConfigHome: t.TempDir(),
		},
	}

	// Create a test file in src
	subDir := dir + "/src"
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subDir+"/test.conf", []byte("key = value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	steps, err := smartSyncSteps(cfg, subDir, "/tmp/dst", "prefix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}
