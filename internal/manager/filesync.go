package manager

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyncOptions configures a directory sync operation.
type SyncOptions struct {
	Src      string   // source directory
	Dst      string   // destination directory
	Delete   bool     // delete extraneous files in dst
	Excludes []string // patterns to exclude
}

// SyncDirectory synchronizes src into dst using rsync if available,
// falling back to a native Go implementation.
func SyncDirectory(ctx context.Context, opts SyncOptions) error {
	if err := os.MkdirAll(opts.Dst, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", opts.Dst, err)
	}

	// Prefer rsync for efficiency and --delete support.
	if _, err := os.Stat("/usr/bin/rsync"); err == nil {
		return rsyncDir(ctx, opts)
	}
	return nativeSyncDir(opts)
}

func rsyncDir(ctx context.Context, opts SyncOptions) error {
	args := make([]string, 0, 8+len(opts.Excludes))
	args = append(args, "-a")
	if opts.Delete {
		args = append(args, "--delete")
	}
	for _, excl := range opts.Excludes {
		args = append(args, "--exclude", excl)
	}
	args = append(args, opts.Src+"/", opts.Dst+"/")

	return runCtx(ctx, "rsync", args...)
}

// nativeSyncDir is a fallback when rsync is not installed.
// It does not support --delete or excludes.
func nativeSyncDir(opts SyncOptions) error {
	return filepath.Walk(opts.Src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(opts.Src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		dst := filepath.Join(opts.Dst, rel)

		if info.IsDir() {
			return os.MkdirAll(dst, info.Mode())
		}

		return copyFile(path, dst, info.Mode())
	})
}

// CopyFile copies a single file from src to dst, preserving mode.
func CopyFile(ctx context.Context, src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	return copyFile(src, dst, mode)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir dst dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".copy-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("copy: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dst)
}

// EnsureDir creates a directory with the given mode if it doesn't exist.
func EnsureDir(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

// WriteFile writes data to a file with the given mode, creating parent dirs.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

// LineInFile ensures a line exists in a file, appending it if necessary.
func LineInFile(path, line string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, existing := range strings.Split(string(data), "\n") {
		if existing == line {
			return nil
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, line)
	return err
}

func runCtx(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
