// Package xdg resolves XDG Base Directory paths from environment variables.
package xdg

import (
	"os"
	"path/filepath"
)

// Paths holds all resolved XDG directories.
type Paths struct {
	BinHome    string
	CacheHome  string
	ConfigHome string
	DataHome   string
	StateHome  string
	RuntimeDir string
}

// Resolve reads XDG environment variables and fills in defaults per spec.
func Resolve() Paths {
	home, _ := os.UserHomeDir()
	return Paths{
		BinHome:    envOr("XDG_BIN_HOME", filepath.Join(home, ".local/bin")),
		CacheHome:  envOr("XDG_CACHE_HOME", filepath.Join(home, ".cache")),
		ConfigHome: envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config")),
		DataHome:   envOr("XDG_DATA_HOME", filepath.Join(home, ".local/share")),
		StateHome:  envOr("XDG_STATE_HOME", filepath.Join(home, ".local/state")),
		RuntimeDir: os.Getenv("XDG_RUNTIME_DIR"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
