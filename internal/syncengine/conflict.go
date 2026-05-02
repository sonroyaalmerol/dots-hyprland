package syncengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ConflictInfo struct {
	RelPath     string
	Strategy    string
	Reason      string
	OrigSHA     string
	CurrentSHA  string
	UpstreamSHA string
	OrigFile    string
	NewFile     string
}

func WriteConflictBackups(deployPath string, current, upstream []byte) error {
	origPath := deployPath + ".orig"
	newPath := deployPath + ".new"

	if err := atomicWrite(origPath, current, 0o644); err != nil {
		return fmt.Errorf("write .orig: %w", err)
	}
	if err := atomicWrite(newPath, upstream, 0o644); err != nil {
		return fmt.Errorf("write .new: %w", err)
	}
	return nil
}

func LogConflict(conflictsPath string, info ConflictInfo) error {
	dir := filepath.Dir(conflictsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create conflicts dir: %w", err)
	}

	entry := map[string]any{
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"relPath":     info.RelPath,
		"strategy":    info.Strategy,
		"reason":      info.Reason,
		"origSHA":     info.OrigSHA,
		"currentSHA":  info.CurrentSHA,
		"upstreamSHA": info.UpstreamSHA,
		"origFile":    info.OrigFile,
		"newFile":     info.NewFile,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal conflict entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(conflictsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open conflicts log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write conflict entry: %w", err)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sync-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
