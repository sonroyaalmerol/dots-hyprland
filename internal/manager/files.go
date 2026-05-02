package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/platform"
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

func syncQuickshell(cfg Config) error {
	src := cfg.ConfigsDir() + "/quickshell"
	dst := cfg.XDG.ConfigHome + "/quickshell"
	if _, err := os.Stat(src); err != nil {
		// Try old path
		src = cfg.RepoRoot + "/configs/quickshell"
	}
	return SyncDirectory(context.Background(), SyncOptions{
		Src:    src,
		Dst:    dst,
		Delete: true,
	})
}

func syncHyprland(cfg Config) error {
	// Sync hyprland config dir
	src := cfg.ConfigsDir() + "/hypr/hyprland"
	dst := cfg.XDG.ConfigHome + "/hypr/hyprland"
	if _, err := os.Stat(src); err != nil {
		src = cfg.RepoRoot + "/configs/hypr/hyprland"
	}
	if err := SyncDirectory(context.Background(), SyncOptions{Src: src, Dst: dst, Delete: true}); err != nil {
		return err
	}

	// Individual config files (only overwrite if firstrun or force)
	confFiles := []string{"hyprlock.conf", "monitors.conf", "workspaces.conf", "hyprland.conf", "hypridle.conf"}
	for _, f := range confFiles {
		srcFile := cfg.ConfigsDir() + "/hyprland-entries/" + f
		if _, err := os.Stat(srcFile); err != nil {
			srcFile = cfg.RepoRoot + "/configs/hypr/" + f
		}
		dstFile := cfg.XDG.ConfigHome + "/hypr/" + f

		if !cfg.Force && !cfg.IsFirstrun() {
			if _, err := os.Stat(dstFile); err == nil {
				continue // skip if already exists
			}
		}

		if err := CopyFile(context.Background(), srcFile, dstFile, 0o644); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("copy %s: %w", f, err)
		}
	}

	// Custom dir (don't delete existing)
	customSrc := cfg.ConfigsDir() + "/hypr/custom"
	customDst := cfg.XDG.ConfigHome + "/hypr/custom"
	if _, err := os.Stat(customSrc); err != nil {
		customSrc = cfg.RepoRoot + "/configs/hypr/custom"
	}
	return SyncDirectory(context.Background(), SyncOptions{Src: customSrc, Dst: customDst, Delete: false})
}

func syncBash(cfg Config) error {
	src := cfg.ConfigsDir() + "/bash"
	dst := cfg.XDG.ConfigHome + "/bash"
	if _, err := os.Stat(src); err != nil {
		src = cfg.RepoRoot + "/configs/bash"
	}
	if err := SyncDirectory(context.Background(), SyncOptions{Src: src, Dst: dst, Delete: true}); err != nil {
		return err
	}

	// Install dotfiles
	files := map[string]string{
		"bashrc":       cfg.Home + "/.bashrc",
		"bash_profile": cfg.Home + "/.bash_profile",
		"zprofile":     cfg.Home + "/.zprofile",
		"inputrc":      cfg.Home + "/.inputrc",
	}
	for name, dstPath := range files {
		srcPath := src + "/" + name
		if _, err := os.Stat(srcPath); err != nil {
			continue
		}
		if err := CopyFile(context.Background(), srcPath, dstPath, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] copy %s: %v\n", name, err)
		}
	}
	return nil
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
	return SyncDirectory(context.Background(), SyncOptions{
		Src:    src,
		Dst:    cfg.XDG.ConfigHome + "/fontconfig",
		Delete: true,
	})
}

func syncMiscConfigs(cfg Config) error {
	// Fuzzel, wlogout, etc.
	miscDirs := []string{"fuzzel", "wlogout"}
	for _, dir := range miscDirs {
		src := cfg.ConfigsDir() + "/" + dir
		if _, err := os.Stat(src); err != nil {
			src = cfg.RepoRoot + "/configs/" + dir
		}
		dst := cfg.XDG.ConfigHome + "/" + dir
		if err := SyncDirectory(context.Background(), SyncOptions{Src: src, Dst: dst, Delete: true}); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] sync %s: %v\n", dir, err)
		}
	}

	// Konsole profile
	konsoleSrc := cfg.RepoRoot + "/configs/local/share/konsole"
	if _, err := os.Stat(konsoleSrc); err == nil {
		_ = SyncDirectory(context.Background(), SyncOptions{
			Src: konsoleSrc, Dst: cfg.XDG.DataHome + "/konsole", Delete: true,
		})
	}
	return nil
}

func installDaemonBinary(cfg Config) error {
	// Check for locally built binary first
	localBin := cfg.XDG.BinHome + "/snry-daemon"

	// Try to build from source
	srcDir := cfg.RepoRoot
	if _, err := os.Stat(srcDir + "/cmd/snry-daemon/main.go"); err == nil {
		fmt.Println("  Building snry-daemon from source...")
		cmd := exec.Command("go", "build", "-o", localBin, "./cmd/snry-daemon")
		cmd.Dir = srcDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Fall back to package-installed binary
	pkgBin := "/usr/share/snry-shell/scripts/snry-daemon/snry-daemon"
	if _, err := os.Stat(pkgBin); err == nil {
		return CopyFile(context.Background(), pkgBin, localBin, 0o755)
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
