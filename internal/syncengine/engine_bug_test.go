package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Regression: readOrigFromManifest returns the .orig file content
// (written at first deploy time) for 3-way merge.
func TestReadOrigFromManifestReturnsData(t *testing.T) {
	dir := t.TempDir()
	deployPath := filepath.Join(dir, "test.conf")

	original := "key = original\n"

	// Write original content as the deploy file
	if err := os.WriteFile(deployPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write the .orig file with the same original content (as done at first deploy)
	origPath := deployPath + ".orig"
	if err := os.WriteFile(origPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	result := readOrigFromManifest(SyncStep{DeployPath: deployPath}, sha256Of([]byte(original)))

	if len(result) == 0 {
		t.Error("expected non-empty original data, got empty slice")
	}
	if string(result) != original {
		t.Errorf("expected %q, got %q", original, string(result))
	}
}

// Regression: handleOverwrite preserves user data on DecisionConflict.
func TestOverwriteStrategyPreservesUserOnConflict(t *testing.T) {
	upDir := t.TempDir()
	deployDir := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")

	orig := "gaps_in = 4\n"
	upstream := "gaps_in = 8\n"
	user := "gaps_in = 99\n"

	oldUp := writeUpstream(t, upDir, "old.conf", orig)
	deployPath := filepath.Join(deployDir, "test.conf")
	engine := New(Config{ManifestPath: manifestPath})
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: oldUp,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	if err := os.WriteFile(deployPath, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}

	newUp := writeUpstream(t, upDir, "new.conf", upstream)
	results := engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: newUp,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	if results[0].Decision != DecisionConflict {
		t.Fatalf("expected DecisionConflict, got %v", results[0].Decision)
	}

	content := readDeploy(t, deployDir, "test.conf")
	if content != user {
		t.Errorf("user data should be preserved on conflict, got %q", content)
	}
}

// Regression: Manifest OriginalSHA256 is preserved across syncs
// and doesn't drift to the current upstream.
func TestManifestPreservesOriginalBaseline(t *testing.T) {
	upDir := t.TempDir()
	deployDir := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	deployPath := filepath.Join(deployDir, "test.conf")

	engine := New(Config{ManifestPath: manifestPath})

	// Deploy v1
	v1Up := writeUpstream(t, upDir, "v1.conf", "key = v1\n")
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: v1Up,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	m1, _ := LoadManifest(manifestPath)
	origSHA1 := m1.GetEntry("test.conf").OriginalSHA256

	// User modifies to v2
	if err := os.WriteFile(deployPath, []byte("key = v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sync with same upstream v1 → DecisionKeep
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: v1Up,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	m2, _ := LoadManifest(manifestPath)
	origSHA2 := m2.GetEntry("test.conf").OriginalSHA256
	if origSHA2 != origSHA1 {
		t.Errorf("OriginalSHA drifted after DecisionKeep: %s → %s", origSHA1[:12], origSHA2[:12])
	}

	// Change upstream to v3, user has v2 → DecisionConflict
	v3Up := writeUpstream(t, upDir, "v3.conf", "key = v3\n")
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: v3Up,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	m3, _ := LoadManifest(manifestPath)
	origSHA3 := m3.GetEntry("test.conf").OriginalSHA256
	if origSHA3 != origSHA1 {
		t.Errorf("OriginalSHA drifted after conflict: %s → %s (should stay at original baseline)", origSHA1[:12], origSHA3[:12])
	}
}

// Regression: .orig backup is written at first deploy time.
func TestOrigBackupWrittenOnFirstDeploy(t *testing.T) {
	upDir := t.TempDir()
	deployDir := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	deployPath := filepath.Join(deployDir, "test.conf")

	engine := New(Config{ManifestPath: manifestPath})
	upPath := writeUpstream(t, upDir, "test.conf", "key = value\n")
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	origPath := deployPath + ".orig"
	data, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf(".orig file not created: %v", err)
	}
	if string(data) != "key = value\n" {
		t.Errorf(".orig content mismatch: got %q", string(data))
	}
}

// Regression: Manifest CurrentSHA256 reflects actual file on disk for DecisionKeep.
func TestManifestCurrentSHAReflectsActualFileOnKeep(t *testing.T) {
	upDir := t.TempDir()
	deployDir := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	deployPath := filepath.Join(deployDir, "test.conf")

	engine := New(Config{ManifestPath: manifestPath})

	upPath := writeUpstream(t, upDir, "test.conf", "key = v1\n")
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	// User modifies
	userContent := "key = user_modified\n"
	if err := os.WriteFile(deployPath, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}
	userSHA := sha256Of([]byte(userContent))

	// Sync with same upstream → DecisionKeep
	engine.Run(context.Background(), []SyncStep{{
		UpstreamPath: upPath,
		DeployPath:   deployPath,
		RelPath:      "test.conf",
		Strategy:     StrategyOverwrite,
	}})

	m, _ := LoadManifest(manifestPath)
	entry := m.GetEntry("test.conf")
	if entry.CurrentSHA256 != userSHA {
		t.Errorf("CurrentSHA256 should match user file, got %s (want %s)", entry.CurrentSHA256[:12], userSHA[:12])
	}
}
