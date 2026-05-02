package manager

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// ArchPackages reads the Arch package list from data/arch/packages.conf.
// Lines starting with # are comments. Blank lines are skipped.
func ArchPackages(dataDir string) ([]string, error) {
	path := dataDir + "/arch/packages.conf"
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var packages []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		packages = append(packages, line)
	}
	return packages, scanner.Err()
}

// FedoraDeps represents the parsed feddeps.toml structure.
type FedoraDeps struct {
	COPR struct {
		Repos []string `toml:"repos"`
	} `toml:"copr"`
	Groups map[string]FedoraGroup `toml:"groups"`
}

// FedoraGroup is a package group with optional install options.
type FedoraGroup struct {
	Packages    []string `toml:"packages"`
	InstallOpts []string `toml:"install_opts"`
}

// LoadFedoraDeps reads and parses data/fedora/feddeps.toml.
func LoadFedoraDeps(dataDir string) (*FedoraDeps, error) {
	path := dataDir + "/fedora/feddeps.toml"
	var deps FedoraDeps
	if _, err := toml.DecodeFile(path, &deps); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &deps, nil
}

// PythonRequirements returns the path to requirements.txt.
func PythonRequirements(dataDir string) string {
	return dataDir + "/python/requirements.txt"
}

// PackageManager is the interface for OS-specific package operations.
type PackageManager interface {
	Distro() string
	UpdateSystem(ctx context.Context) error
	InstallPackages(ctx context.Context, packages []string) error
	InstallBuildDeps(ctx context.Context) error
	CheckPackages(ctx context.Context, packages []string) (missing []string, err error)
}
