package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/platform"
	syncengine "github.com/sonroyaalmerol/snry-shell-qs/internal/syncengine"
)

// FilesSteps returns steps for syncing config files to XDG directories.
func FilesSteps(cfg Config) []Step {
	return []Step{
		{
			Name: "create-xdg-dirs",
			Fn: func(ctx context.Context) error {
				return createXDGDirs(cfg)
			},
		},
		{
			Name: "ensure-sync-manifest",
			Fn: func(ctx context.Context) error {
				return syncengine.EnsureManifest(cfg.DotsConfDir() + "/sync-manifest.json")
			},
		},
		{
			Name: "backup-existing-configs",
			Fn: func(ctx context.Context) error {
				return backupConfigs(cfg)
			},
			Optional: true,
		},
		{
			Name: "sync-quickshell-config",
			Fn: func(ctx context.Context) error {
				if cfg.SkipQuickshell {
					return nil
				}
				return syncQuickshell(cfg)
			},
		},
		{
			Name: "sync-hyprland-config",
			Fn: func(ctx context.Context) error {
				if cfg.SkipHyprland {
					return nil
				}
				return syncHyprland(cfg)
			},
		},
		{
			Name: "sync-bash-config",
			Fn: func(ctx context.Context) error {
				if cfg.SkipBash {
					return nil
				}
				return syncBash(cfg)
			},
		},
		{
			Name: "sync-fontconfig",
			Fn: func(ctx context.Context) error {
				if cfg.SkipFontconfig {
					return nil
				}
				return syncFontconfig(cfg)
			},
		},
		{
			Name: "sync-misc-configs",
			Fn: func(ctx context.Context) error {
				if cfg.SkipMiscConf {
					return nil
				}
				return syncMiscConfigs(cfg)
			},
		},
		{
			Name: "install-snry-daemon-binary",
			Fn: func(ctx context.Context) error {
				return installDaemonBinary(cfg)
			},
		},
		{
			Name: "install-hyprgrass-plugin",
			Fn: func(ctx context.Context) error {
				if cfg.SkipHyprland {
					return nil
				}
				return installHyprgrass(cfg)
			},
			Optional: true,
		},
		{
			Name: "install-google-sans-flex",
			Fn: func(ctx context.Context) error {
				return installGoogleSansFlex(cfg)
			},
			Optional: true,
		},
		{
			Name: "install-snry-shell-icon",
			Fn: func(ctx context.Context) error {
				return installIcon(cfg)
			},
		},
		{
			Name: "install-python-venv",
			Fn: func(ctx context.Context) error {
				return installPythonVenv(cfg)
			},
		},
		{
			Name: "mark-firstrun",
			Fn: func(ctx context.Context) error {
				return markFirstrun(cfg)
			},
		},
		{
			Name: "reload-hyprland",
			Fn: func(ctx context.Context) error {
				_ = exec.Command("hyprctl", "reload").Run()
				return nil
			},
			Optional: true,
		},
		{
			Name: "deploy-systemd-user-unit",
			Fn: func(ctx context.Context) error {
				return deploySystemdUnit(cfg)
			},
		},
		{
			Name: "start-quickshell",
			Fn: func(ctx context.Context) error {
				return startQuickshell()
			},
			Optional: true,
		},
	}
}

func createXDGDirs(cfg Config) error {
	dirs := []string{
		cfg.XDG.BinHome,
		cfg.XDG.CacheHome,
		cfg.XDG.ConfigHome,
		cfg.XDG.DataHome,
	}
	for _, d := range dirs {
		if err := EnsureDir(d, 0o755); err != nil {
			return err
		}
	}
	return EnsureDir(cfg.DotsConfDir(), 0o755)
}

func backupConfigs(cfg Config) error {
	if cfg.SkipBackup {
		return nil
	}
	backupDir := cfg.BackupDir()
	_ = os.MkdirAll(backupDir+"/.config", 0o755)

	for _, dir := range []string{"quickshell", "bash", "fontconfig", "hypr"} {
		src := cfg.XDG.ConfigHome + "/" + dir
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := backupDir + "/.config/" + dir
		_ = exec.Command("rsync", "-av", src+"/", dst+"/").Run()
	}
	return nil
}

func smartSyncSteps(cfg Config, srcDir string, dstDir string, relPrefix string) []syncengine.SyncStep {
	var steps []syncengine.SyncStep
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, srcDir+"/")
		steps = append(steps, syncengine.SyncStep{
			UpstreamPath: path,
			DeployPath:   dstDir + "/" + rel,
			RelPath:      relPrefix + "/" + rel,
		})
		return nil
	})
	return steps
}

func runSmartSync(cfg Config, steps []syncengine.SyncStep) error {
	engine := syncengine.New(syncengine.Config{
		ManifestPath: cfg.DotsConfDir() + "/sync-manifest.json",
		Variables:    syncengine.ResolveTemplateVars(cfg.Home, cfg.XDG.ConfigHome, cfg.XDG.DataHome, cfg.XDG.StateHome, cfg.XDG.BinHome, cfg.XDG.CacheHome, cfg.XDG.RuntimeDir, cfg.VenvPath(), cfg.FontsetDirName),
		Categorizer:  syncengine.DefaultCategorizer(),
	})
	results := engine.Run(context.Background(), steps)
	var errs []string
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  [sync] %s: %v\n", r.RelPath, r.Err)
			errs = append(errs, r.RelPath)
		} else if r.Conflict != nil {
			fmt.Fprintf(os.Stderr, "  [conflict] %s: %s\n", r.RelPath, r.Conflict.Reason)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sync failed for %d files", len(errs))
	}
	return nil
}

func syncQuickshell(cfg Config) error {
	src := cfg.ConfigsDir() + "/quickshell"
	if _, err := os.Stat(src); err != nil {
		src = cfg.RepoRoot + "/configs/quickshell"
	}
	steps := smartSyncSteps(cfg, src, cfg.XDG.ConfigHome+"/quickshell", "quickshell")
	return runSmartSync(cfg, steps)
}

func syncHyprland(cfg Config) error {
	var allSteps []syncengine.SyncStep

	// Sync hyprland config dir
	hyprlandSrc := cfg.ConfigsDir() + "/hypr/hyprland"
	if _, err := os.Stat(hyprlandSrc); err != nil {
		hyprlandSrc = cfg.RepoRoot + "/configs/hypr/hyprland"
	}
	allSteps = append(allSteps, smartSyncSteps(cfg, hyprlandSrc, cfg.XDG.ConfigHome+"/hypr/hyprland", "hypr/hyprland")...)

	// Individual config files
	confFiles := []string{"hyprlock.conf", "hyprland.conf", "hypridle.conf"}
	for _, f := range confFiles {
		srcFile := cfg.ConfigsDir() + "/hyprland-entries/" + f
		if _, err := os.Stat(srcFile); err != nil {
			srcFile = cfg.RepoRoot + "/configs/hypr/" + f
		}
		if _, err := os.Stat(srcFile); err != nil {
			continue
		}
		allSteps = append(allSteps, syncengine.SyncStep{
			UpstreamPath: srcFile,
			DeployPath:   cfg.XDG.ConfigHome + "/hypr/" + f,
			RelPath:      "hypr/" + f,
		})
	}

	// monitors.conf and workspaces.conf (skip-if-exists via categorizer)
	for _, f := range []string{"monitors.conf", "workspaces.conf"} {
		srcFile := cfg.ConfigsDir() + "/hyprland-entries/" + f
		if _, err := os.Stat(srcFile); err != nil {
			srcFile = cfg.RepoRoot + "/configs/hypr/" + f
		}
		if _, err := os.Stat(srcFile); err != nil {
			continue
		}
		allSteps = append(allSteps, syncengine.SyncStep{
			UpstreamPath: srcFile,
			DeployPath:   cfg.XDG.ConfigHome + "/hypr/" + f,
			RelPath:      "hypr/" + f,
		})
	}

	// Custom dir (don't delete existing)
	customSrc := cfg.ConfigsDir() + "/hypr/custom"
	if _, err := os.Stat(customSrc); err != nil {
		customSrc = cfg.RepoRoot + "/configs/hypr/custom"
	}
	allSteps = append(allSteps, smartSyncSteps(cfg, customSrc, cfg.XDG.ConfigHome+"/hypr/custom", "hypr/custom")...)

	return runSmartSync(cfg, allSteps)
}

func syncBash(cfg Config) error {
	bashSrc := cfg.ConfigsDir() + "/bash"
	if _, err := os.Stat(bashSrc); err != nil {
		bashSrc = cfg.RepoRoot + "/configs/bash"
	}

	var allSteps []syncengine.SyncStep
	allSteps = append(allSteps, smartSyncSteps(cfg, bashSrc, cfg.XDG.ConfigHome+"/bash", "bash")...)

	// Install dotfiles to home dir
	dotfiles := map[string]string{
		"bashrc":       cfg.Home + "/.bashrc",
		"bash_profile": cfg.Home + "/.bash_profile",
		"zprofile":     cfg.Home + "/.zprofile",
		"inputrc":      cfg.Home + "/.inputrc",
	}
	for name, dstPath := range dotfiles {
		srcPath := bashSrc + "/" + name
		if _, err := os.Stat(srcPath); err != nil {
			continue
		}
		allSteps = append(allSteps, syncengine.SyncStep{
			UpstreamPath: srcPath,
			DeployPath:   dstPath,
			RelPath:      "bash/" + name,
		})
	}

	return runSmartSync(cfg, allSteps)
}

func syncFontconfig(cfg Config) error {
	src := cfg.ConfigsDir() + "/fontconfig"
	if cfg.FontsetDirName != "" {
		src = cfg.ConfigsDir() + "/fontsets/" + cfg.FontsetDirName
	}
	if _, err := os.Stat(src); err != nil {
		src = cfg.RepoRoot + "/configs/fontconfig"
		if cfg.FontsetDirName != "" {
			src = cfg.RepoRoot + "/configs/extra/fontsets/" + cfg.FontsetDirName
		}
	}
	steps := smartSyncSteps(cfg, src, cfg.XDG.ConfigHome+"/fontconfig", "fontconfig")
	return runSmartSync(cfg, steps)
}

func syncMiscConfigs(cfg Config) error {
	var allSteps []syncengine.SyncStep

	// Directories to sync
	dirs := []string{"fuzzel", "wlogout", "foot", "ghostty", "Kvantum", "matugen", "mpv", "kde-material-you-colors", "zshrc.d", "xdg-desktop-portal"}
	for _, dir := range dirs {
		src := cfg.ConfigsDir() + "/" + dir
		if _, err := os.Stat(src); err != nil {
			src = cfg.RepoRoot + "/configs/" + dir
		}
		if _, err := os.Stat(src); err == nil {
			allSteps = append(allSteps, smartSyncSteps(cfg, src, cfg.XDG.ConfigHome+"/"+dir, dir)...)
		}
	}

	// Individual files to sync
	files := map[string]string{
		"starship.toml":      "starship.toml",
		"darklyrc":           "darklyrc",
		"dolphinrc":          "dolphinrc",
		"kdeglobals":         "kdeglobals",
		"konsolerc":          "konsolerc",
		"chrome-flags.conf":  "chrome-flags.conf",
		"code-flags.conf":    "code-flags.conf",
		"thorium-flags.conf": "thorium-flags.conf",
	}
	for filename, relPath := range files {
		srcFile := cfg.ConfigsDir() + "/" + filename
		if _, err := os.Stat(srcFile); err != nil {
			srcFile = cfg.RepoRoot + "/configs/" + filename
		}
		if _, err := os.Stat(srcFile); err == nil {
			allSteps = append(allSteps, syncengine.SyncStep{
				UpstreamPath: srcFile,
				DeployPath:   cfg.XDG.ConfigHome + "/" + filename,
				RelPath:      relPath,
			})
		}
	}

	// Konsole profile
	konsoleSrc := cfg.RepoRoot + "/configs/local/share/konsole"
	if _, err := os.Stat(konsoleSrc); err == nil {
		allSteps = append(allSteps, smartSyncSteps(cfg, konsoleSrc, cfg.XDG.DataHome+"/konsole", "konsole")...)
	}

	if len(allSteps) == 0 {
		return nil
	}
	return runSmartSync(cfg, allSteps)
}

func installDaemonBinary(cfg Config) error {
	localBin := cfg.XDG.BinHome + "/snry-daemon"

	// If already installed system-wide, just symlink
	systemBin := "/usr/bin/snry-daemon"
	if _, err := os.Stat(systemBin); err == nil {
		_ = os.Remove(localBin)
		if err := os.Symlink(systemBin, localBin); err != nil {
			fmt.Printf("  Linked %s -> %s\n", localBin, systemBin)
		}
		return nil
	}

	// Try to build from source (dev mode)
	srcDir := cfg.RepoRoot
	if _, err := os.Stat(srcDir + "/cmd/snry-daemon/main.go"); err == nil {
		if _, err := os.Stat(srcDir + "/go.mod"); err == nil {
			fmt.Println("  Building snry-daemon from source...")
			cmd := exec.Command("go", "build", "-o", localBin, "./cmd/snry-daemon")
			cmd.Dir = srcDir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
	}

	fmt.Println("  No snry-daemon source or package binary found, skipping.")
	return nil
}

func installHyprgrass(cfg Config) error {
	pluginDir := cfg.Home + "/.local/lib/hyprland/plugins"
	pluginFile := pluginDir + "/libhyprgrass.so"

	if _, err := os.Stat(pluginFile); err == nil {
		return nil // already installed
	}

	cacheDir := cfg.XDG.CacheHome + "/snry-shell/hyprgrass"
	_ = os.MkdirAll(pluginDir, 0o755)

	// Clone
	if _, err := os.Stat(cacheDir); err != nil {
		fmt.Println("  Cloning hyprgrass...")
		if err := exec.Command("git", "clone", "--depth=1",
			"https://github.com/horriblename/hyprgrass.git", cacheDir).Run(); err != nil {
			return fmt.Errorf("clone hyprgrass: %w", err)
		}
		exec.Command("git", "-C", cacheDir, "submodule", "update", "--init", "--recursive").Run()
	}

	// Build
	buildDir := cacheDir + "/build"
	if _, err := os.Stat(buildDir); err != nil {
		fmt.Println("  Building hyprgrass...")
		mesonCmd := exec.Command("meson", "setup", "build",
			"--prefix="+cfg.Home+"/.local", "-Dhyprgrass=true")
		mesonCmd.Dir = cacheDir
		mesonCmd.Env = append(os.Environ(), "HYPRLAND_HEADERS=/usr/include")
		if err := mesonCmd.Run(); err != nil {
			return fmt.Errorf("meson setup: %w", err)
		}
	}

	ninjaCmd := exec.Command("ninja", "-C", "build")
	ninjaCmd.Dir = cacheDir
	if err := ninjaCmd.Run(); err != nil {
		return fmt.Errorf("ninja build: %w", err)
	}

	return CopyFile(context.Background(),
		cacheDir+"/build/src/libhyprgrass.so", pluginFile, 0o755)
}

func installGoogleSansFlex(cfg Config) error {
	if platform.Detect() == platform.FamilyFedora {
		return nil // Fedora has it in repos
	}

	// Check if already installed
	if err := exec.Command("fc-list").Run(); err == nil {
		grep := exec.Command("sh", "-c", "fc-list | grep -qi 'Google Sans Flex'")
		if grep.Run() == nil {
			return nil
		}
	}

	cacheDir := cfg.XDG.CacheHome + "/snry-shell/google-sans-flex"
	fontsDir := cfg.XDG.DataHome + "/fonts/snry-shell-google-sans-flex"

	if _, err := os.Stat(cacheDir); err != nil {
		fmt.Println("  Cloning Google Sans Flex...")
		if err := exec.Command("git", "clone", "--depth=1",
			"https://github.com/end-4/google-sans-flex", cacheDir).Run(); err != nil {
			return err
		}
	}

	_ = os.MkdirAll(fontsDir, 0o755)
	cmd := exec.Command("rsync", "-a", cacheDir+"/", fontsDir+"/")
	if err := cmd.Run(); err != nil {
		return err
	}

	_ = exec.Command("fc-cache", "-fv").Run()
	return nil
}

func installIcon(cfg Config) error {
	src := cfg.RepoRoot + "/configs/local/share/icons/snry-shell.svg"
	dst := cfg.XDG.DataHome + "/icons/snry-shell.svg"
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	return CopyFile(context.Background(), src, dst, 0o644)
}

func installPythonVenv(cfg Config) error {
	venvPath := cfg.VenvPath()
	reqFile := PythonRequirements(cfg.DataDir())

	_ = os.MkdirAll(filepath.Dir(venvPath), 0o755)

	if _, err := os.Stat(venvPath + "/bin/activate"); os.IsNotExist(err) {
		fmt.Println("  Creating Python venv...")
		cmd := exec.Command("uv", "venv", "--prompt", ".venv", venvPath, "-p", "3.12")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("create venv: %w", err)
		}
	}

	fmt.Println("  Installing Python packages...")
	cmd := exec.Command("uv", "pip", "install", "-r", reqFile)
	cmd.Env = append(os.Environ(),
		"VIRTUAL_ENV="+venvPath,
		"UV_NO_MODIFY_PATH=1",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func markFirstrun(cfg Config) error {
	_ = touchFile(cfg.FirstrunFile())
	return LineInFile(cfg.InstalledListfile(), cfg.FirstrunFile())
}

func startQuickshell() error {
	if err := exec.Command("pidof", "qs").Run(); err != nil {
		fmt.Println("  Starting quickshell...")
		cmd := exec.Command("qs", "-c", "ii")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Start()
	}
	return nil
}

func deploySystemdUnit(cfg Config) error {
	src := cfg.ConfigsDir() + "/systemd/user/snry-daemon.service"
	if _, err := os.Stat(src); err != nil {
		fmt.Println("  No systemd unit found, skipping.")
		return nil
	}

	userDir := cfg.XDG.ConfigHome + "/systemd/user"
	if err := EnsureDir(userDir, 0o755); err != nil {
		return err
	}

	dst := userDir + "/snry-daemon.service"
	if err := CopyFile(context.Background(), src, dst, 0o644); err != nil {
		return fmt.Errorf("deploy systemd unit: %w", err)
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Println("  Deployed snry-daemon.service to systemd user directory.")
	return nil
}
