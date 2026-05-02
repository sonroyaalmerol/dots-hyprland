package manager

import (
	"context"
	"fmt"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/manager/arch"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/platform"
)

// CheckDeps checks for missing packages and prints the results.
func CheckDeps(ctx context.Context, cfg Config) error {
	distro := platform.Detect()

	switch distro {
	case platform.FamilyArch:
		return checkDepsArch(ctx, cfg)
	case platform.FamilyFedora:
		return checkDepsFedora(ctx, cfg)
	default:
		return fmt.Errorf("unsupported distribution for checkdeps")
	}
}

func checkDepsArch(ctx context.Context, cfg Config) error {
	packages, err := ArchPackages(cfg.DataDir())
	if err != nil {
		return fmt.Errorf("read packages: %w", err)
	}

	mgr := arch.New()
	missing, err := mgr.CheckPackages(ctx, packages)
	if err != nil {
		return fmt.Errorf("check packages: %w", err)
	}

	if len(missing) == 0 {
		fmt.Println("  All packages are available.")
	} else {
		fmt.Printf("  Missing packages (%d):\n", len(missing))
		for _, p := range missing {
			fmt.Printf("    - %s\n", p)
		}
	}
	return nil
}

func checkDepsFedora(ctx context.Context, cfg Config) error {
	deps, err := LoadFedoraDeps(cfg.DataDir())
	if err != nil {
		return fmt.Errorf("load fedora deps: %w", err)
	}

	var allPackages []string
	for _, group := range deps.Groups {
		allPackages = append(allPackages, group.Packages...)
	}

	missing, err := (&fedoraMgr{}).CheckPackages(ctx, allPackages)
	if err != nil {
		return err
	}

	if len(missing) == 0 {
		fmt.Println("  All packages are available.")
	} else {
		fmt.Printf("  Missing packages (%d):\n", len(missing))
		for _, p := range missing {
			fmt.Printf("    - %s\n", p)
		}
	}
	return nil
}

// fedoraMgr is a minimal adapter for checkdeps.
type fedoraMgr struct{}

func (f *fedoraMgr) CheckPackages(ctx context.Context, packages []string) ([]string, error) {
	return nil, fmt.Errorf("fedora checkdeps not yet implemented")
}
