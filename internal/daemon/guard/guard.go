package guard

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sonroyaalmerol/snry-shell-qs/frontend"
)

type Config struct {
	WatchDir string // the deployed quickshell dir, e.g. ~/.config/quickshell/ii/
}

type Service struct {
	cfg Config
}

func New(cfg Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Run(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("guard: create watcher: %w", err)
	}
	defer watcher.Close()

	// Add the watch directory and all subdirectories.
	if err := addWatchRecursive(watcher, s.cfg.WatchDir); err != nil {
		return fmt.Errorf("guard: add watch: %w", err)
	}

	log.Printf("guard: watching %s for unauthorized changes", s.cfg.WatchDir)

	// Debounce: coalesce rapid events (e.g. editor write-then-rename).
	var timer *time.Timer
	affected := make(map[string]struct{})

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Skip the watch dir itself, only care about files.
			if event.Name == s.cfg.WatchDir {
				// If a new subdirectory was created, watch it.
				if event.Has(fsnotify.Create) {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						watcher.Add(event.Name)
					}
				}
				continue
			}
			affected[event.Name] = struct{}{}
			if timer != nil {
				timer.Reset(100 * time.Millisecond)
			} else {
				timer = time.AfterFunc(100*time.Millisecond, func() {
					for path := range affected {
						s.restoreFile(path)
					}
					affected = make(map[string]struct{})
					timer = nil
				})
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("guard: watcher error: %v", err)
		}
	}
}

// restoreFile restores a single file from the embedded FS.
func (s *Service) restoreFile(deployedPath string) {
	// Get the relative path under the watch dir.
	rel, err := filepath.Rel(s.cfg.WatchDir, deployedPath)
	if err != nil {
		return
	}

	// In the embedded FS, paths are prefixed with "ii/".
	embedPath := "ii/" + filepath.ToSlash(rel)

	// Read from embedded FS.
	data, err := fs.ReadFile(frontend.FS, embedPath)
	if err != nil {
		// File doesn't exist in embedded FS — it was a user-created file.
		// Delete it since it shouldn't be there.
		if err := os.Remove(deployedPath); err != nil && !os.IsNotExist(err) {
			log.Printf("guard: failed to remove unauthorized file %s: %v", deployedPath, err)
		} else {
			log.Printf("guard: removed unauthorized file: %s", rel)
		}
		return
	}

	// Write back to disk.
	if err := os.WriteFile(deployedPath, data, 0o644); err != nil {
		log.Printf("guard: failed to restore %s: %v", rel, err)
		return
	}
	log.Printf("guard: restored modified file: %s", rel)
}

// addWatchRecursive adds a watch on dir and all existing subdirectories.
func addWatchRecursive(watcher *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := watcher.Add(path); err != nil {
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
		return nil
	})
}
