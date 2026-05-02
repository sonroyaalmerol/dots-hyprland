package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/platform"
)

// SetupSteps returns system setup steps that require elevated privileges.
func SetupSteps(cfg Config) []Step {
	return []Step{
		{
			Name: "create-system-groups",
			Fn: func(ctx context.Context) error {
				return setupGroups(ctx)
			},
		},
		{
			Name: "enable-services",
			Fn: func(ctx context.Context) error {
				return setupServices(ctx)
			},
		},
		{
			Name: "configure-tty-autologin",
			Fn: func(ctx context.Context) error {
				return setupTTYAutologin(ctx)
			},
		},
		{
			Name: "configure-pam",
			Fn: func(ctx context.Context) error {
				return setupPAM(ctx)
			},
		},
		{
			Name: "configure-logind",
			Fn: func(ctx context.Context) error {
				return setupLogind(ctx, cfg)
			},
		},
		{
			Name: "disable-display-managers",
			Fn: func(ctx context.Context) error {
				return disableDisplayManagers(ctx)
			},
		},
		{
			Name: "set-gsettings",
			Fn: func(ctx context.Context) error {
				return setupGSettings(ctx)
			},
		},
		{
			Name: "set-zsh-default",
			Fn: func(ctx context.Context) error {
				return setZshDefault(ctx)
			},
		},
		{
			Name: "create-hushlogin",
			Fn: func(ctx context.Context) error {
				return touchFile(filepath.Join(cfg.Home, ".hushlogin"))
			},
			Optional: true,
		},
	}
}

func setupGroups(ctx context.Context) error {
	user := platform.RealUser()

	groups := []string{"video", "input"}
	if platform.Detect() == platform.FamilyArch {
		groups = append(groups, "i2c")
		// Create i2c group if it doesn't exist
		_, _ = platform.RunSudo(ctx, "groupadd", "-f", "i2c")
	}

	for _, group := range groups {
		fmt.Printf("  Adding user %s to group %s\n", user, group)
		_, _ = platform.RunSudo(ctx, "usermod", "-aG", group, user)
	}

	if platform.Detect() == platform.FamilyArch {
		// Load i2c-dev module at boot
		_ = WriteFile("/tmp/i2c-dev.conf", []byte("i2c-dev\n"), 0o644)
		_, _ = platform.RunSudo(ctx, "cp", "/tmp/i2c-dev.conf", "/etc/modules-load.d/i2c-dev.conf")
		os.Remove("/tmp/i2c-dev.conf")
	}

	if platform.Detect() == platform.FamilyFedora {
		// Load uinput module at boot
		_ = WriteFile("/tmp/uinput.conf", []byte("uinput\n"), 0o644)
		_, _ = platform.RunSudo(ctx, "cp", "/tmp/uinput.conf", "/etc/modules-load.d/uinput.conf")
		os.Remove("/tmp/uinput.conf")

		// Udev rules for uinput
		udevRule := `SUBSYSTEM=="misc", KERNEL=="uinput", MODE="0660", GROUP="input"` + "\n"
		_ = WriteFile("/tmp/99-uinput.rules", []byte(udevRule), 0o644)
		_, _ = platform.RunSudo(ctx, "cp", "/tmp/99-uinput.rules", "/etc/udev/rules.d/99-uinput.rules")
		os.Remove("/tmp/99-uinput.rules")
	}

	return nil
}

func setupServices(ctx context.Context) error {
	fmt.Println("  Enabling bluetooth service...")
	_, _ = platform.RunSudo(ctx, "systemctl", "enable", "--now", "bluetooth")
	return nil
}

func setupTTYAutologin(ctx context.Context) error {
	user := platform.RealUser()
	fmt.Printf("  Configuring TTY autologin for %s\n", user)

	// Create override directory
	_, _ = platform.RunSudo(ctx, "mkdir", "-p", "/etc/systemd/system/getty@tty1.service.d")

	// Write override
	content := `[Service]
ExecStart=
ExecStart=-/sbin/agetty --noissue --nohostname --noclear --autologin ` + user + ` %I $TERM
`
	tmpFile := "/tmp/getty-tty1-override.conf"
	_ = os.WriteFile(tmpFile, []byte(content), 0o644)
	_, err := platform.RunSudo(ctx, "cp", tmpFile, "/etc/systemd/system/getty@tty1.service.d/override.conf")
	os.Remove(tmpFile)
	if err != nil {
		return fmt.Errorf("deploy getty override: %w", err)
	}

	_, _ = platform.RunSudo(ctx, "systemctl", "daemon-reload")
	return nil
}

func setupPAM(ctx context.Context) error {
	fmt.Println("  Configuring PAM for gnome-keyring...")

	// Add pam_gnome_keyring to /etc/pam.d/login
	loginFile := "/etc/pam.d/login"

	// Auth line
	authLine := "auth       optional   pam_gnome_keyring.so"
	sessionLine := "session    optional   pam_gnome_keyring.so auto_start"

	// Use sed to add lines after specific patterns
	_, _ = platform.RunSudo(ctx, "bash", "-c",
		fmt.Sprintf(`grep -q '%s' %s || sed -i '/^auth.*include.*system-local-login/a %s' %s`,
			authLine, loginFile, authLine, loginFile))

	_, _ = platform.RunSudo(ctx, "bash", "-c",
		fmt.Sprintf(`grep -q '%s' %s || sed -i '/^session.*include.*system-local-login/a %s' %s`,
			sessionLine, loginFile, sessionLine, loginFile))

	return nil
}

func setupLogind(ctx context.Context, cfg Config) error {
	fmt.Println("  Deploying logind configuration...")

	// Check for custom logind.conf in configs
	customConf := cfg.ConfigsDir() + "/hyprland/custom/logind.conf"
	if _, err := os.Stat(customConf); err != nil {
		// Try the old path
		customConf = cfg.RepoRoot + "/configs/hypr/custom/logind.conf"
	}
	if _, err := os.Stat(customConf); err != nil {
		fmt.Println("  No custom logind.conf found, skipping.")
		return nil
	}

	_, _ = platform.RunSudo(ctx, "mkdir", "-p", "/etc/systemd/logind.conf.d")
	_, err := platform.RunSudo(ctx, "cp", customConf, "/etc/systemd/logind.conf.d/override.conf")
	return err
}

func disableDisplayManagers(ctx context.Context) error {
	fmt.Println("  Disabling display managers...")
	for _, dm := range []string{"gdm", "sddm", "lightdm", "display-manager"} {
		_, _ = platform.RunSudo(ctx, "systemctl", "disable", "--now", dm)
	}
	return nil
}

func setupGSettings(ctx context.Context) error {
	fmt.Println("  Setting desktop preferences...")

	exec.Command("gsettings", "set", "org.gnome.desktop.interface",
		"font-name", "Google Sans Flex Medium 11 @opsz=11,wght=500").Run()
	exec.Command("gsettings", "set", "org.gnome.desktop.interface",
		"color-scheme", "prefer-dark").Run()
	exec.Command("kwriteconfig6", "--file", "kdeglobals", "--group", "KDE",
		"--key", "widgetStyle", "Darkly").Run()

	return nil
}

func setZshDefault(ctx context.Context) error {
	user := platform.RealUser()
	fmt.Printf("  Setting zsh as default shell for %s\n", user)
	_, err := platform.RunSudo(ctx, "chsh", "-s", "/usr/bin/zsh", user)
	return err
}

func touchFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
