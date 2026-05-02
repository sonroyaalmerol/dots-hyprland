package fedora

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Manager implements package operations for Fedora.
type Manager struct{}

// New creates a new Fedora package manager.
func New() *Manager {
	return &Manager{}
}

func (m *Manager) Distro() string { return "fedora" }

// UpdateSystem performs a full system upgrade via sudo dnf upgrade.
func (m *Manager) UpdateSystem(ctx context.Context) error {
	fmt.Println("  Updating system packages...")
	cmd := exec.CommandContext(ctx, "sudo", "dnf", "upgrade", "-y")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// InstallPackages installs package groups via dnf.
func (m *Manager) InstallPackages(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	fmt.Printf("  Installing %d packages via dnf...\n", len(packages))

	args := make([]string, 0, 3+len(packages))
	args = append(args, "install", "-y")
	args = append(args, packages...)

	sudoArgs := make([]string, 0, 2+len(args))
	sudoArgs = append(sudoArgs, "dnf")
	sudoArgs = append(sudoArgs, args...)
	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// InstallGroup installs packages from a FedoraDeps group.
func (m *Manager) InstallGroup(ctx context.Context, groupName string, packages []string, opts []string) error {
	if len(packages) == 0 {
		return nil
	}

	fmt.Printf("  Installing group [%s] (%d packages)...\n", groupName, len(packages))

	dnfArgs := []string{"install", "-y"}
	dnfArgs = append(dnfArgs, opts...)
	dnfArgs = append(dnfArgs, packages...)

	sudoArgs := make([]string, 0, 1+len(dnfArgs))
	sudoArgs = append(sudoArgs, "dnf")
	sudoArgs = append(sudoArgs, dnfArgs...)
	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// EnableCOPR enables a COPR repository.
func (m *Manager) EnableCOPR(ctx context.Context, repos []string) error {
	for _, repo := range repos {
		fmt.Printf("  Enabling COPR repo: %s\n", repo)
		cmd := exec.CommandContext(ctx, "sudo", "dnf", "copr", "enable", "-y", repo)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] COPR %s: %v\n", repo, err)
		}
	}
	return nil
}

// InstallBuildDeps installs development tools group.
func (m *Manager) InstallBuildDeps(ctx context.Context) error {
	fmt.Println("  Installing development tools...")
	cmd := exec.CommandContext(ctx, "sudo", "dnf", "install", "-y", "@development-tools", "fedora-packager")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CheckPackages checks which packages are not available in repos.
func (m *Manager) CheckPackages(ctx context.Context, packages []string) ([]string, error) {
	var missing []string
	for _, p := range packages {
		cmd := exec.CommandContext(ctx, "dnf", "list", "available", p)
		if err := cmd.Run(); err != nil {
			missing = append(missing, p)
		}
	}
	return missing, nil
}

// InstallPythonVenv creates a Python virtualenv and installs requirements.
func (m *Manager) InstallPythonVenv(ctx context.Context, venvPath, requirementsFile string) error {
	if err := os.MkdirAll(venvPath[:strings.LastIndex(venvPath, "/")], 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(venvPath + "/bin/activate"); os.IsNotExist(err) {
		fmt.Println("  Creating Python venv...")
		cmd := exec.CommandContext(ctx, "uv", "venv", "--prompt", ".venv", venvPath, "-p", "3.12")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("create venv: %w", err)
		}
	}

	fmt.Println("  Installing Python packages...")
	cmd := exec.CommandContext(ctx, "uv", "pip", "install", "-r", requirementsFile)
	cmd.Env = append(os.Environ(),
		"VIRTUAL_ENV="+venvPath,
		"UV_NO_MODIFY_PATH=1",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
