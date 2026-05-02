package arch

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Manager implements package operations for Arch-based distributions.
type Manager struct {
	realUser string
}

// New creates a new Arch package manager.
func New() *Manager {
	return &Manager{
		realUser: realUser(),
	}
}

func (m *Manager) Distro() string { return "arch" }

// UpdateSystem performs a full system upgrade via sudo pacman -Syu.
func (m *Manager) UpdateSystem(ctx context.Context) error {
	fmt.Println("  Updating system packages...")
	cmd := exec.CommandContext(ctx, "sudo", "pacman", "-Syu", "--noconfirm")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// InstallPackages installs packages via paru (AUR helper).
// It first ensures paru is installed, then installs all packages.
func (m *Manager) InstallPackages(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	if err := m.ensureParu(ctx); err != nil {
		return fmt.Errorf("paru setup: %w", err)
	}

	fmt.Printf("  Installing %d packages via paru...\n", len(packages))

	args := make([]string, 0, 4+len(packages))
	args = append(args, "-S", "--needed", "--noconfirm")
	args = append(args, packages...)

	cmd := exec.CommandContext(ctx, "paru", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// InstallBuildDeps installs base-devel (needed for paru and makepkg).
func (m *Manager) InstallBuildDeps(ctx context.Context) error {
	fmt.Println("  Installing build dependencies...")
	cmd := exec.CommandContext(ctx, "sudo", "pacman", "-S", "--needed", "--noconfirm", "base-devel")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CheckPackages returns packages not found in pacman repos or AUR.
func (m *Manager) CheckPackages(ctx context.Context, packages []string) ([]string, error) {
	// Build local package set
	localSet := make(map[string]struct{}, 1024)

	// Get pacman repo packages
	if out, err := exec.CommandContext(ctx, "pacman", "-Ssq").Output(); err == nil {
		for p := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			localSet[p] = struct{}{}
		}
	}

	// Get AUR packages
	if out, err := exec.CommandContext(ctx, "curl", "-s", "https://aur.archlinux.org/packages.gz").Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				localSet[line] = struct{}{}
			}
		}
	}

	var missing []string
	for _, p := range packages {
		if _, ok := localSet[p]; !ok {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// BuildMicroTeX builds and installs the MicroTeX AUR package from PKGBUILD.
func (m *Manager) BuildMicroTeX(ctx context.Context, pkgbuildDir string) error {
	// Check if already installed
	if err := exec.CommandContext(ctx, "pacman", "-Q", "snry-shell-microtex-git").Run(); err == nil {
		fmt.Println("  MicroTeX already installed, skipping.")
		return nil
	}

	fmt.Println("  Building MicroTeX from PKGBUILD...")

	buildDir := "/tmp/build-microtex"
	_ = os.RemoveAll(buildDir)
	defer func() { _ = os.RemoveAll(buildDir) }()

	// Copy PKGBUILD to build directory
	if err := copyDir(pkgbuildDir, buildDir); err != nil {
		return fmt.Errorf("copy PKGBUILD: %w", err)
	}

	// Build package
	cmd := exec.CommandContext(ctx, "makepkg", "-sf", "--noconfirm")
	cmd.Dir = buildDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("makepkg: %w", err)
	}

	// Install package
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return fmt.Errorf("read build dir: %w", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".pkg.tar.zst") {
			installCmd := exec.CommandContext(ctx, "sudo", "pacman", "-U", "--noconfirm", buildDir+"/"+e.Name())
			installCmd.Stdin = os.Stdin
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			return installCmd.Run()
		}
	}

	return fmt.Errorf("no .pkg.tar.zst found in %s", buildDir)
}

func (m *Manager) ensureParu(ctx context.Context) error {
	if _, err := exec.LookPath("paru"); err == nil {
		return nil
	}

	fmt.Println("  Installing paru AUR helper...")

	// Ensure base-devel is installed
	if err := m.InstallBuildDeps(ctx); err != nil {
		return err
	}

	buildDir := "/tmp/buildparu"
	_ = os.RemoveAll(buildDir)
	defer func() { _ = os.RemoveAll(buildDir) }()

	cloneCmd := exec.CommandContext(ctx, "git", "clone", "https://aur.archlinux.org/paru.git", buildDir)
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("clone paru: %w", err)
	}

	buildCmd := exec.CommandContext(ctx, "makepkg", "-si", "--noconfirm", "--needed")
	buildCmd.Dir = buildDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	return buildCmd.Run()
}

func realUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	return os.Getenv("USER")
}

func copyDir(src, dst string) error {
	cmd := exec.Command("cp", "-a", src+"/.", dst+"/")
	return cmd.Run()
}
