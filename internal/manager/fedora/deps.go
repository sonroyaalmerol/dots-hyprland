package fedora

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Manager implements package operations for Fedora.
type Manager struct{}

// New creates a new Fedora package manager.
func New() *Manager {
	return &Manager{}
}

// UpdateSystem performs a full system upgrade via sudo dnf upgrade.
func (m *Manager) UpdateSystem(ctx context.Context) error {
	fmt.Println("  Updating system packages...")
	cmd := exec.CommandContext(ctx, "sudo", "dnf", "upgrade", "-y")
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
