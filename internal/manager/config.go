package manager

import (
	"os"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/xdg"
)

// Config holds all configuration for the manager operations.
// It replaces the Ansible group_vars/all.yml with a Go-native struct.
type Config struct {
	// Skip flags control which phases run during setup.
	SkipSysUpdate  bool
	SkipQuickshell bool
	SkipHyprland   bool
	SkipBash       bool
	SkipFontconfig bool
	SkipMiscConf   bool
	SkipBackup     bool
	Force          bool

	// FontsetDirName overrides fontconfig with a named fontset from configs/fontsets/.
	// Empty string means use the default fontconfig.
	FontsetDirName string

	// RepoRoot is the path to the snry-shell-qs repository/installation root.
	RepoRoot string

	// XDG holds resolved XDG base directory paths.
	XDG xdg.Paths

	// Home is the real user's home directory.
	Home string
}

// DotsConfDir returns the snry-shell config directory.
func (c Config) DotsConfDir() string {
	return c.XDG.ConfigHome + "/snry-shell"
}

// InstalledListfile returns the path to the installed files tracking list.
func (c Config) InstalledListfile() string {
	return c.DotsConfDir() + "/installed_listfile"
}

// FirstrunFile returns the path to the firstrun marker file.
func (c Config) FirstrunFile() string {
	return c.DotsConfDir() + "/installed_true"
}

// VenvPath returns the Python virtual environment path.
func (c Config) VenvPath() string {
	return c.XDG.StateHome + "/quickshell/.venv"
}

// BackupDir returns the backup directory for clashing configs.
func (c Config) BackupDir() string {
	return c.Home + "/ii-original-dots-backup"
}

// ConfigsDir returns the base configs directory within the repo.
func (c Config) ConfigsDir() string {
	return c.RepoRoot + "/configs"
}

// DataDir returns the base data directory within the repo.
func (c Config) DataDir() string {
	return c.RepoRoot + "/data"
}

// IsFirstrun returns true if the firstrun marker file exists.
func (c Config) IsFirstrun() bool {
	return !fileExists(c.FirstrunFile())
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
