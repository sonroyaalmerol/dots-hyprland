package syncengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Manifest struct {
	Version string                `json:"version"`
	Entries map[string]*FileEntry `json:"entries"`
}

type FileEntry struct {
	RelPath        string `json:"relPath"`
	Strategy       string `json:"strategy"`
	OriginalSHA256 string `json:"originalSha256"`
	CurrentSHA256  string `json:"currentSha256"`
	UpstreamSHA256 string `json:"upstreamSha256"`
	DeployPath     string `json:"deployPath"`
	UpstreamPath   string `json:"upstreamPath"`
	LastSynced     string `json:"lastSynced"`
	Conflict       bool   `json:"conflict"`
}

func sha256Of(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Version: "1", Entries: make(map[string]*FileEntry)}, nil
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		bakPath := path + ".bak"
		os.Rename(path, bakPath)
		return &Manifest{Version: "1", Entries: make(map[string]*FileEntry)}, nil
	}

	if m.Entries == nil {
		m.Entries = make(map[string]*FileEntry)
	}
	if m.Version == "" {
		m.Version = "1"
	}
	return &m, nil
}

func SaveManifest(m *Manifest, path string) error {
	if m.Version == "" {
		m.Version = "1"
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sync-manifest-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp manifest: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename manifest: %w", err)
	}
	return nil
}

func EnsureManifest(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	m := &Manifest{Version: "1", Entries: make(map[string]*FileEntry)}
	return SaveManifest(m, path)
}

func (m *Manifest) GetEntry(relPath string) *FileEntry {
	if m.Entries == nil {
		return nil
	}
	return m.Entries[relPath]
}

func (m *Manifest) SetEntry(entry *FileEntry) {
	if m.Entries == nil {
		m.Entries = make(map[string]*FileEntry)
	}
	m.Entries[entry.RelPath] = entry
}

func (m *Manifest) RemoveEntry(relPath string) {
	if m.Entries != nil {
		delete(m.Entries, relPath)
	}
}
