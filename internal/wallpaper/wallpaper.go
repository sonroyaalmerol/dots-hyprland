package wallpaper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Config holds wallpaper service configuration.
type Config struct {
	// XDG directories
	ConfigHome string
	StateHome  string
	CacheHome  string
	// Repo root for finding template files
	RepoRoot string
	// Path to the snry-shell config file
	ShellConfigFile string
	// Path to matugen config
	MatugenConfig string
	// Virtual env for Python (empty if not needed)
	VirtualEnv string
}

// DefaultConfig returns a config using standard XDG paths.
func DefaultConfig() Config {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(os.Getenv("HOME"), ".config")
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		cacheHome = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return Config{
		ConfigHome:      configHome,
		StateHome:       stateHome,
		CacheHome:       cacheHome,
		ShellConfigFile: filepath.Join(configHome, "snry-shell", "config.json"),
		MatugenConfig:   filepath.Join(configHome, "matugen", "config.toml"),
		VirtualEnv:      os.Getenv("SNRY_SHELL_VIRTUAL_ENV"),
	}
}

// GenDir returns the quickshell generated files directory.
func (c Config) GenDir() string {
	return filepath.Join(c.StateHome, "quickshell", "user", "generated")
}

// TerminalConfig holds config for terminal color generation.
type TerminalConfig struct {
	EnableTerminal     bool
	ForceDarkMode      bool
	Harmony            float64
	HarmonizeThreshold float64
	TermFgBoost        float64
}

// DefaultTerminalConfig returns default terminal generation config.
func DefaultTerminalConfig() TerminalConfig {
	return TerminalConfig{
		EnableTerminal:     true,
		ForceDarkMode:      false,
		Harmony:            0.8,
		HarmonizeThreshold: 100,
		TermFgBoost:        0.35,
	}
}

// ShellConfig represents the snry-shell config.json structure (partial).
type ShellConfig struct {
	Appearance struct {
		Palette struct {
			Type        string `json:"type"`
			AccentColor string `json:"accentColor"`
		} `json:"palette"`
		WallpaperTheming struct {
			EnableAppsAndShell      bool `json:"enableAppsAndShell"`
			EnableTerminal          bool `json:"enableTerminal"`
			EnableQtApps            bool `json:"enableQtApps"`
			TerminalGenerationProps struct {
				ForceDarkMode      bool    `json:"forceDarkMode"`
				Harmony            float64 `json:"harmony"`
				HarmonizeThreshold float64 `json:"harmonizeThreshold"`
				TermFgBoost        float64 `json:"termFgBoost"`
			} `json:"terminalGenerationProps"`
		} `json:"wallpaperTheming"`
	} `json:"appearance"`
	Background struct {
		WallpaperPath string `json:"wallpaperPath"`
		ThumbnailPath string `json:"thumbnailPath"`
	} `json:"background"`
}

// LoadShellConfig reads and parses the snry-shell config.json.
func LoadShellConfig(path string) (*ShellConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg ShellConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// SaveShellConfig writes the config back to disk.
func SaveShellConfig(path string, cfg *ShellConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// SetWallpaperPath updates the wallpaper path in config.
func SetWallpaperPath(cfgPath, wallPath string) error {
	cfg, err := LoadShellConfig(cfgPath)
	if err != nil {
		return err
	}
	cfg.Background.WallpaperPath = wallPath
	return SaveShellConfig(cfgPath, cfg)
}

// SetThumbnailPath updates the thumbnail path in config.
func SetThumbnailPath(cfgPath, thumbPath string) error {
	cfg, err := LoadShellConfig(cfgPath)
	if err != nil {
		return err
	}
	cfg.Background.ThumbnailPath = thumbPath
	return SaveShellConfig(cfgPath, cfg)
}

// SetAccentColor updates the accent color in config.
func SetAccentColor(cfgPath, color string) error {
	cfg, err := LoadShellConfig(cfgPath)
	if err != nil {
		return err
	}
	cfg.Appearance.Palette.AccentColor = color
	return SaveShellConfig(cfgPath, cfg)
}

// ClearAccentColor removes the accent color from config.
func ClearAccentColor(cfgPath string) error {
	return SetAccentColor(cfgPath, "")
}

// IsDarkMode detects whether the system is in dark mode.
func IsDarkMode() bool {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		return true // default to dark
	}
	return strings.Contains(string(out), "prefer-dark")
}

// SetDarkMode sets the system color scheme via gsettings.
func SetDarkMode(isDark bool) {
	if isDark {
		exec.Command("gsettings", "set", "org.gnome.desktop.interface", "color-scheme", "prefer-dark").Run()
		exec.Command("gsettings", "set", "org.gnome.desktop.interface", "gtk-theme", "adw-gtk3-dark").Run()
	} else {
		exec.Command("gsettings", "set", "org.gnome.desktop.interface", "color-scheme", "prefer-light").Run()
		exec.Command("gsettings", "set", "org.gnome.desktop.interface", "gtk-theme", "adw-gtk3").Run()
	}
}

// RunMatugen executes matugen to generate material colors from an image or color.
// It generates colors.json and other template outputs.
func RunMatugen(cfg Config, imgPath string, color string, mode string, schemeType SchemeType) error {
	genDir := cfg.GenDir()
	if err := os.MkdirAll(genDir, 0755); err != nil {
		return fmt.Errorf("create gen dir: %w", err)
	}

	args := []string{"--source-color-index", "0"}
	args = append(args, "--mode", mode)
	args = append(args, "--type", string(schemeType))
	args = append(args, "-c", cfg.MatugenConfig)

	if color != "" {
		args = append(args, "color", "hex", color)
	} else {
		args = append(args, "image", imgPath)
	}

	cmd := exec.Command("matugen", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("matugen: %w", err)
	}

	return nil
}

// GenerateAndApplyColors reads colors.json from matugen output, generates terminal
// colors, and applies them to ghostty, nvim, vscode, etc.
func GenerateAndApplyColors(cfg Config, termCfg TerminalConfig) error {
	genDir := cfg.GenDir()

	// Read colors.json generated by matugen
	colors, err := LoadColorMapFromJSON(filepath.Join(genDir, "colors.json"))
	if err != nil {
		return fmt.Errorf("load colors: %w", err)
	}

	// Determine if dark mode
	isDark := true
	if termCfg.ForceDarkMode {
		isDark = true
	} else {
		isDark = IsDarkMode()
	}
	_ = isDark

	// Load terminal base scheme
	schemePath := FindTerminalScheme(cfg)
	scheme, err := LoadTerminalScheme(schemePath)
	if err != nil {
		return fmt.Errorf("load terminal scheme: %w", err)
	}

	// Generate harmonized terminal colors
	termColors := GenerateTerminalColors(colors, scheme, isDark,
		termCfg.Harmony, termCfg.HarmonizeThreshold, termCfg.TermFgBoost)

	// Apply terminal theme files
	if err := ApplyTerminalTheme(colors, termColors, isDark, genDir); err != nil {
		return fmt.Errorf("apply terminal theme: %w", err)
	}

	return nil
}

// FindTerminalScheme locates the terminal scheme-base.json file.
func FindTerminalScheme(cfg Config) string {
	// Try installed path first
	exe, err := os.Executable()
	if err == nil {
		shareDir := filepath.Join(filepath.Dir(exe), "..", "share", "snry-shell", "configs", "quickshell", "ii", "scripts", "colors", "terminal", "scheme-base.json")
		if _, err := os.Stat(shareDir); err == nil {
			return shareDir
		}
	}
	// Try repo path
	if cfg.RepoRoot != "" {
		repoPath := filepath.Join(cfg.RepoRoot, "frontend", "ii", "scripts", "colors", "terminal", "scheme-base.json")
		if _, err := os.Stat(repoPath); err == nil {
			return repoPath
		}
	}
	// Fallback to config dir
	return filepath.Join(cfg.ConfigHome, "quickshell", "ii", "scripts", "colors", "terminal", "scheme-base.json")
}

// FullWallpaperSwitch performs a complete wallpaper switch operation.
// This replaces the switchwall.sh script.
func FullWallpaperSwitch(cfg Config, imgPath string, mode string, schemeType SchemeType, color string) error {
	shellCfg, err := LoadShellConfig(cfg.ShellConfigFile)
	if err != nil {
		log.Printf("[wallpaper] warning: could not load shell config: %v", err)
		shellCfg = &ShellConfig{}
	}

	// Auto-detect scheme type if needed
	if schemeType == SchemeAuto && imgPath != "" {
		schemeType = DetectSchemeFromImage(imgPath, SchemeAuto)
	}

	// Determine dark/light mode
	if mode == "" {
		if IsDarkMode() {
			mode = "dark"
		} else {
			mode = "light"
		}
	}

	// Check if app/shell theming is enabled
	enableAppsAndShell := true // default
	if shellCfg != nil {
		enableAppsAndShell = shellCfg.Appearance.WallpaperTheming.EnableAppsAndShell
	}
	if !enableAppsAndShell {
		log.Printf("[wallpaper] app and shell theming disabled, skipping")
		return nil
	}

	// Set dark/light mode
	SetDarkMode(mode == "dark")

	// Create generated dirs
	genDir := cfg.GenDir()
	os.MkdirAll(filepath.Join(genDir), 0755)

	// Run matugen
	if err := RunMatugen(cfg, imgPath, color, mode, schemeType); err != nil {
		return fmt.Errorf("matugen: %w", err)
	}

	// Get terminal config
	termCfg := DefaultTerminalConfig()
	if shellCfg != nil {
		termCfg.EnableTerminal = shellCfg.Appearance.WallpaperTheming.EnableTerminal
		termCfg.ForceDarkMode = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.ForceDarkMode
		termCfg.Harmony = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.Harmony
		termCfg.HarmonizeThreshold = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.HarmonizeThreshold
		termCfg.TermFgBoost = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.TermFgBoost
	}

	// Generate and apply terminal colors
	if termCfg.EnableTerminal {
		if err := GenerateAndApplyColors(cfg, termCfg); err != nil {
			log.Printf("[wallpaper] terminal colors: %v", err)
		}
	}

	// Handle KDE material you colors (if enabled)
	if shellCfg.Appearance.WallpaperTheming.EnableQtApps {
		handleKDEMaterialYouColors(cfg, schemeType)
	}

	return nil
}

// handleKDEMaterialYouColors invokes the KDE material-you-colors wrapper.
func handleKDEMaterialYouColors(cfg Config, schemeType SchemeType) {
	kdeSchemeVariant := string(schemeType)
	if kdeSchemeVariant == "" {
		kdeSchemeVariant = "scheme-tonal-spot"
	}
	wrapperPath := filepath.Join(cfg.ConfigHome, "matugen", "templates", "kde", "kde-material-you-colors-wrapper.sh")
	if _, err := os.Stat(wrapperPath); err != nil {
		return
	}
	exec.Command(wrapperPath, "--scheme-variant", kdeSchemeVariant).Run()
}
