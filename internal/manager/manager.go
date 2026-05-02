package manager

import (
	"context"
	"fmt"
	"os"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/manager/arch"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/manager/fedora"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/platform"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/xdg"
)

// Setup runs the full installation (deps + files + setups).
func Setup(ctx context.Context, cfg Config) error {
	fmt.Println("=== Snry Shell Setup ===")
	fmt.Printf("Distro: %s\n", platform.Detect())
	fmt.Println()

	// Phase 1: Dependencies
	if err := Deps(ctx, cfg); err != nil {
		return fmt.Errorf("deps: %w", err)
	}

	// Phase 2: System setup
	if err := Setups(ctx, cfg); err != nil {
		return fmt.Errorf("setups: %w", err)
	}

	// Phase 3: Config files
	if err := Files(ctx, cfg); err != nil {
		return fmt.Errorf("files: %w", err)
	}

	fmt.Println()
	fmt.Println("=== Setup complete! ===")
	return nil
}

// Deps installs packages for the detected distribution.
func Deps(ctx context.Context, cfg Config) error {
	fmt.Println("--- Installing dependencies ---")

	distro := platform.Detect()
	switch distro {
	case platform.FamilyArch:
		return depsArch(ctx, cfg)
	case platform.FamilyFedora:
		return depsFedora(ctx, cfg)
	default:
		return fmt.Errorf("unsupported distribution: %s", distro)
	}
}

func depsArch(ctx context.Context, cfg Config) error {
	mgr := arch.New()

	if !cfg.SkipSysUpdate {
		if err := mgr.UpdateSystem(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] system update: %v\n", err)
		}
	}

	packages, err := ArchPackages(cfg.DataDir())
	if err != nil {
		return err
	}

	// Build MicroTeX if needed
	if err := mgr.BuildMicroTeX(ctx, cfg.DataDir()+"/arch/microtex-git"); err != nil {
		fmt.Fprintf(os.Stderr, "  [warn] microtex: %v\n", err)
	}

	return mgr.InstallPackages(ctx, packages)
}

func depsFedora(ctx context.Context, cfg Config) error {
	mgr := fedora.New()

	if !cfg.SkipSysUpdate {
		if err := mgr.UpdateSystem(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] system update: %v\n", err)
		}
	}

	deps, err := LoadFedoraDeps(cfg.DataDir())
	if err != nil {
		return err
	}

	// Enable COPR repos
	if err := mgr.EnableCOPR(ctx, deps.COPR.Repos); err != nil {
		fmt.Fprintf(os.Stderr, "  [warn] copr repos: %v\n", err)
	}

	// Install each group
	for name, group := range deps.Groups {
		if err := mgr.InstallGroup(ctx, name, group.Packages, group.InstallOpts); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] group %s: %v\n", name, err)
		}
	}

	return nil
}

// Setups runs system setup steps (groups, systemd, PAM, etc.).
func Setups(ctx context.Context, cfg Config) error {
	fmt.Println("--- Running system setup ---")
	steps := SetupSteps(cfg)
	results := RunSteps(ctx, steps, consoleProgress)
	PrintResults(os.Stdout, results)
	return firstError(results)
}

// Files syncs config files to XDG directories.
func Files(ctx context.Context, cfg Config) error {
	fmt.Println("--- Syncing config files ---")
	steps := FilesSteps(cfg)
	results := RunSteps(ctx, steps, consoleProgress)
	PrintResults(os.Stdout, results)
	return firstError(results)
}

// DefaultConfig returns a Config populated from environment and repo root.
func DefaultConfig(repoRoot string) Config {
	return Config{
		XDG:      xdg.Resolve(),
		Home:     platform.HomeDir(),
		RepoRoot: repoRoot,
	}
}

func consoleProgress(step string, current, total int) {
	fmt.Printf("  [%d/%d] %s\n", current, total, step)
}

func firstError(results []StepResult) error {
	for _, r := range results {
		if r.Err != nil {
			return r.Err
		}
	}
	return nil
}
