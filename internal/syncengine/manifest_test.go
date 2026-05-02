package syncengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manifest")
	}
	if m.Version != "1" {
		t.Errorf("expected version '1', got %q", m.Version)
	}
	if len(m.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(m.Entries))
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	m := &Manifest{
		Version: "1",
		Entries: map[string]*FileEntry{
			"hypr/hyprland.conf": {
				RelPath:        "hypr/hyprland.conf",
				Strategy:       "merge-hyprland",
				OriginalSHA256: "abc123",
				CurrentSHA256:  "abc123",
				UpstreamSHA256: "def456",
				DeployPath:     "/home/user/.config/hypr/hyprland.conf",
				UpstreamPath:   "/usr/share/snry/hypr/hyprland.conf",
				LastSynced:     "2026-01-01T00:00:00Z",
			},
			"bash/bashrc": {
				RelPath:        "bash/bashrc",
				Strategy:       "merge-section",
				OriginalSHA256: "789abc",
			},
		},
	}

	if err := SaveManifest(m, path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.Version != "1" {
		t.Errorf("expected version '1', got %q", loaded.Version)
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded.Entries))
	}

	entry := loaded.Entries["hypr/hyprland.conf"]
	if entry == nil {
		t.Fatal("missing entry for hypr/hyprland.conf")
	}
	if entry.Strategy != "merge-hyprland" {
		t.Errorf("expected strategy 'merge-hyprland', got %q", entry.Strategy)
	}
	if entry.OriginalSHA256 != "abc123" {
		t.Errorf("expected original sha 'abc123', got %q", entry.OriginalSHA256)
	}
	if entry.UpstreamSHA256 != "def456" {
		t.Errorf("expected upstream sha 'def456', got %q", entry.UpstreamSHA256)
	}

	entry2 := loaded.Entries["bash/bashrc"]
	if entry2 == nil {
		t.Fatal("missing entry for bash/bashrc")
	}
	if entry2.Strategy != "merge-section" {
		t.Errorf("expected strategy 'merge-section', got %q", entry2.Strategy)
	}
}

func TestEnsureManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	// First call creates the file
	if err := EnsureManifest(path); err != nil {
		t.Fatalf("first ensure failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("manifest file not created")
	}

	// Second call is a no-op
	if err := EnsureManifest(path); err != nil {
		t.Fatalf("second ensure failed: %v", err)
	}

	// Verify it's valid
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if m.Version != "1" {
		t.Errorf("expected version '1', got %q", m.Version)
	}
}

func TestCorruptManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	// Write garbage
	if err := os.WriteFile(path, []byte("not json at all {{{"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(m.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(m.Entries))
	}

	// Verify .bak was created
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Error("expected .bak file to be created")
	}
}

func TestManifestGetSetRemove(t *testing.T) {
	m := &Manifest{Version: "1", Entries: make(map[string]*FileEntry)}

	// GetEntry returns nil for missing
	if entry := m.GetEntry("nonexistent"); entry != nil {
		t.Error("expected nil for missing entry")
	}

	// SetEntry upserts
	entry := &FileEntry{RelPath: "hypr/hyprland.conf", Strategy: "merge-hyprland"}
	m.SetEntry(entry)
	if got := m.GetEntry("hypr/hyprland.conf"); got == nil {
		t.Fatal("entry not found after set")
	}
	if got := m.GetEntry("hypr/hyprland.conf"); got.Strategy != "merge-hyprland" {
		t.Errorf("expected strategy 'merge-hyprland', got %q", got.Strategy)
	}

	// Upsert
	entry2 := &FileEntry{RelPath: "hypr/hyprland.conf", Strategy: "overwrite"}
	m.SetEntry(entry2)
	if got := m.GetEntry("hypr/hyprland.conf"); got.Strategy != "overwrite" {
		t.Errorf("expected strategy 'overwrite' after upsert, got %q", got.Strategy)
	}

	// RemoveEntry deletes
	m.RemoveEntry("hypr/hyprland.conf")
	if entry := m.GetEntry("hypr/hyprland.conf"); entry != nil {
		t.Error("expected nil after remove")
	}
}

func TestSHA256(t *testing.T) {
	// SHA256 of "hello"
	got := sha256Of([]byte("hello"))
	// SHA256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != expected {
		t.Errorf("sha256 mismatch:\n got  %s\n want %s", got, expected)
	}

	// Empty input
	got2 := sha256Of([]byte(""))
	expected2 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got2 != expected2 {
		t.Errorf("sha256 of empty mismatch:\n got  %s\n want %s", got2, expected2)
	}
}
