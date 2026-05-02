package manager

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/platform"
)

// Uninstall removes installed files and reverts system changes.
func Uninstall(ctx context.Context, cfg Config) error {
	fmt.Println("Uninstalling snry-shell...")

	// Remove tracked files
	if err := removeTrackedFiles(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "  [warn] remove tracked files: %v\n", err)
	}

	// Revert system changes
	if err := revertSystemChanges(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "  [warn] revert system: %v\n", err)
	}

	fmt.Println("Uninstall complete.")
	return nil
}

func removeTrackedFiles(cfg Config) error {
	listfile := cfg.InstalledListfile()
	f, err := os.Open(listfile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  No installed listfile found, skipping file removal.")
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "  [warn] remove %s: %v\n", path, err)
		}
	}
	return scanner.Err()
}

func revertSystemChanges(ctx context.Context, cfg Config) error {
	user := platform.RealUser()

	// Remove from groups
	for _, group := range []string{"video", "input"} {
		_, _ = platform.RunSudo(ctx, "gpasswd", "-d", user, group)
	}

	if platform.Detect() == platform.FamilyArch {
		_, _ = platform.RunSudo(ctx, "gpasswd", "-d", user, "i2c")
		_, _ = platform.RunSudo(ctx, "rm", "-f", "/etc/modules-load.d/i2c-dev.conf")
	}

	return nil
}
