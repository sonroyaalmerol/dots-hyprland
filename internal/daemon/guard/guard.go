package guard

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sonroyaalmerol/snry-shell-qs/frontend"
)

type Config struct {
	WatchDir  string // directory to watch (e.g. /usr/share/snry-shell/frontend/ii/)
	SourceDir string // source directory to restore from (filesystem); empty = use embedded FS

	// Excludes lists relative paths (forward-slash, e.g. "monitors.lua") that
	// should NOT be restored from SourceDir. Instead, the Regenerate callback
	// is invoked so the caller can rebuild the file from live data.
	Excludes []string

	// Regenerate is called when an excluded file is modified or created.
	// The rel argument is the forward-slash relative path within WatchDir.
	// If nil, excluded files are simply left untouched (not restored, not removed).
	Regenerate func(rel string) error
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

// isExcluded returns true if the relative path matches an entry in Excludes.
func (s *Service) isExcluded(rel string) bool {
	normalized := filepath.ToSlash(rel)
	return slices.Contains(s.cfg.Excludes, normalized)
}

// restoreFile restores a single file from the source (embedded FS or filesystem),
// or delegates to the Regenerate callback for excluded (generated) files.
func (s *Service) restoreFile(deployedPath string) {
	rel, err := filepath.Rel(s.cfg.WatchDir, deployedPath)
	if err != nil {
		return
	}

	// Generated files: regenerate instead of restoring from source.
	if s.isExcluded(rel) {
		if s.cfg.Regenerate != nil {
			log.Printf("guard: regenerating excluded file: %s", rel)
			if err := s.cfg.Regenerate(filepath.ToSlash(rel)); err != nil {
				log.Printf("guard: failed to regenerate %s: %v", rel, err)
			}
		} else {
			log.Printf("guard: leaving excluded file untouched: %s", rel)
		}
		return
	}

	var data []byte
	if s.cfg.SourceDir != "" {
		// Filesystem source: read from SourceDir/<relative-path>
		sourcePath := filepath.Join(s.cfg.SourceDir, filepath.ToSlash(rel))
		data, err = os.ReadFile(sourcePath)
	} else {
		// Embedded FS source (quickshell): paths are prefixed with "ii/"
		embedPath := "ii/" + filepath.ToSlash(rel)
		data, err = fs.ReadFile(frontend.FS, embedPath)
	}

	if err != nil {
		if err := os.Remove(deployedPath); err != nil && !os.IsNotExist(err) {
			log.Printf("guard: failed to remove unauthorized file %s: %v", deployedPath, err)
		} else {
			log.Printf("guard: removed unauthorized file: %s", rel)
		}
		return
	}

	if err := os.WriteFile(deployedPath, data, 0o644); err != nil {
		log.Printf("guard: failed to restore %s: %v", rel, err)
		return
	}
	log.Printf("guard: restored modified file: %s", rel)
}

// EnsureGenerated writes generated files that don't exist yet on disk.
// This should be called on startup to seed monitors.lua and workspaces.lua
// before the guard starts watching.
func (s *Service) EnsureGenerated() {
	for _, rel := range s.cfg.Excludes {
		path := filepath.Join(s.cfg.WatchDir, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if s.cfg.Regenerate != nil {
				log.Printf("guard: generating missing file: %s", rel)
				if err := s.cfg.Regenerate(rel); err != nil {
					log.Printf("guard: failed to generate %s: %v", rel, err)
				}
			}
		}
	}
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
