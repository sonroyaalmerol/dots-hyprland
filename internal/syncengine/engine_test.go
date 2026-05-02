package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupEngineTest(t *testing.T) (upstreamDir string, deployDir string, manifestPath string) {
	t.Helper()
	upstreamDir = t.TempDir()
	deployDir = t.TempDir()
	manifestPath = filepath.Join(t.TempDir(), "manifest.json")
	return
}

func writeUpstream(t *testing.T, dir, relPath, content string) string {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func writeDeploy(t *testing.T, dir, relPath, content string) string {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func readDeploy(t *testing.T, dir, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatalf("read deployed file: %v", err)
	}
	return string(data)
}

func TestEngineNewFile(t *testing.T) {
	upDir, deployDir, manifestPath := setupEngineTest(t)
	upPath := writeUpstream(t, upDir, "hypr/hyprland.conf", "gaps_in = 4\n")
	deployPath := filepath.Join(deployDir, "hypr", "hyprland.conf")

	engine := New(Config{
		ManifestPath: manifestPath,
	})

	results := engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Decision != DecisionNew {
		t.Errorf("expected DecisionNew, got %v", results[0].Decision)
	}

	content := readDeploy(t, deployDir, "hypr/hyprland.conf")
	if content != "gaps_in = 4\n" {
		t.Errorf("expected 'gaps_in = 4\\n', got %q", content)
	}

	// Verify manifest
	m, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	entry := m.GetEntry("hypr/hyprland.conf")
	if entry == nil {
		t.Fatal("manifest entry not found")
	}
	if entry.Strategy != "overwrite" {
		t.Errorf("expected strategy 'overwrite', got %q", entry.Strategy)
	}
}

func TestEngineNoChange(t *testing.T) {
	upDir, deployDir, manifestPath := setupEngineTest(t)

	content := "gaps_in = 4\n"
	upPath := writeUpstream(t, upDir, "hypr/hyprland.conf", content)
	deployPath := writeDeploy(t, deployDir, "hypr/hyprland.conf", content)

	// First deploy to establish manifest
	engine := New(Config{ManifestPath: manifestPath})
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	// Second deploy with same content
	results := engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	if results[0].Decision != DecisionNoop {
		t.Errorf("expected DecisionNoop, got %v", results[0].Decision)
	}
}

func TestEngineUpstreamChanged(t *testing.T) {
	upDir, deployDir, manifestPath := setupEngineTest(t)

	oldUpstream := "gaps_in = 4\n"
	newUpstream := "gaps_in = 8\n"

	upPath := writeUpstream(t, upDir, "hypr/hyprland.conf", newUpstream)
	deployPath := writeDeploy(t, deployDir, "hypr/hyprland.conf", oldUpstream)

	// First deploy with old upstream
	oldUpPath := writeUpstream(t, upDir, "hypr/hyprland.conf.old", oldUpstream)
	engine := New(Config{ManifestPath: manifestPath})
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: oldUpPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	// Now deploy new upstream (user hasn't changed the file)
	results := engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	if results[0].Decision != DecisionUpdate {
		t.Errorf("expected DecisionUpdate, got %v", results[0].Decision)
	}

	content := readDeploy(t, deployDir, "hypr/hyprland.conf")
	if content != newUpstream {
		t.Errorf("expected %q, got %q", newUpstream, content)
	}
}

func TestEngineUserChanged(t *testing.T) {
	upDir, deployDir, manifestPath := setupEngineTest(t)

	origContent := "gaps_in = 4\n"
	upPath := writeUpstream(t, upDir, "hypr/hyprland.conf", origContent)
	deployPath := filepath.Join(deployDir, "hypr", "hyprland.conf")

	// First deploy
	engine := New(Config{ManifestPath: manifestPath})
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	// User modifies the deployed file
	if err := os.WriteFile(deployPath, []byte("gaps_in = 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Deploy again with same upstream
	results := engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	if results[0].Decision != DecisionKeep {
		t.Errorf("expected DecisionKeep, got %v", results[0].Decision)
	}

	content := readDeploy(t, deployDir, "hypr/hyprland.conf")
	if content != "gaps_in = 99\n" {
		t.Errorf("expected user's content to be kept, got %q", content)
	}
}

func TestEngineConflict(t *testing.T) {
	upDir, deployDir, manifestPath := setupEngineTest(t)

	origContent := "gaps_in = 4\n"
	upPath := writeUpstream(t, upDir, "hypr/hyprland.conf", "gaps_in = 8\n")
	deployPath := filepath.Join(deployDir, "hypr", "hyprland.conf")

	// First deploy with original content
	oldUpPath := writeUpstream(t, upDir, "hypr/hyprland.conf.orig", origContent)
	engine := New(Config{ManifestPath: manifestPath})
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: oldUpPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	// User modifies the file
	if err := os.WriteFile(deployPath, []byte("gaps_in = 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Deploy new upstream — conflict: both changed
	results := engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/hyprland.conf",
		Strategy:     StrategyOverwrite,
	}})

	if results[0].Decision != DecisionConflict {
		t.Errorf("expected DecisionConflict, got %v", results[0].Decision)
	}

	// With overwrite strategy, user's data is preserved on conflict
	content := readDeploy(t, deployDir, "hypr/hyprland.conf")
	if content != "gaps_in = 99\n" {
		t.Errorf("expected user's content preserved on conflict, got %q", content)
	}
}

func TestEngineSkipIfExists(t *testing.T) {
	upDir, deployDir, manifestPath := setupEngineTest(t)

	upPath := writeUpstream(t, upDir, "hypr/monitors.conf", "monitor = eDP-1, 1920x1080\n")
	deployPath := filepath.Join(deployDir, "hypr", "monitors.conf")

	engine := New(Config{ManifestPath: manifestPath})

	// First deploy — file doesn't exist, should be created
	results := engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/monitors.conf",
		Strategy:     StrategySkipIfExists,
	}})

	if results[0].Decision != DecisionNew {
		t.Errorf("expected DecisionNew on first deploy, got %v", results[0].Decision)
	}
	content := readDeploy(t, deployDir, "hypr/monitors.conf")
	if content != "monitor = eDP-1, 1920x1080\n" {
		t.Errorf("expected initial content, got %q", content)
	}

	// Second deploy with changed upstream — should be skipped
	newUpPath := writeUpstream(t, upDir, "hypr/monitors2.conf", "monitor = eDP-1, 2560x1440\n")
	results = engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: newUpPath,
		DeployPath:   deployPath,
		RelPath:      "hypr/monitors.conf",
		Strategy:     StrategySkipIfExists,
	}})

	// With skip-if-exists, if file exists it keeps current
	if results[0].Decision == DecisionNew {
		t.Error("expected skip-if-exists to not be DecisionNew on second run")
	}
	content = readDeploy(t, deployDir, "hypr/monitors.conf")
	if content != "monitor = eDP-1, 1920x1080\n" {
		t.Errorf("expected original content to be kept, got %q", content)
	}
}
