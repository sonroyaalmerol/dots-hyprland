// Package platform detects the running Linux distribution and provides
// helpers for executing privileged commands.
package platform

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Family identifies a supported distribution family.
type Family int

const (
	FamilyUnknown Family = iota
	FamilyArch
	FamilyFedora
)

func (f Family) String() string {
	switch f {
	case FamilyArch:
		return "arch"
	case FamilyFedora:
		return "fedora"
	default:
		return "unknown"
	}
}

// Detect reads /etc/os-release and returns the distribution family.
func Detect() Family {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return FamilyUnknown
	}
	defer f.Close()

	id, idLike := "", ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := cutPrefix(line, "ID="); ok {
			id = strings.Trim(strings.ToLower(after), `"'`)
		}
		if after, ok := cutPrefix(line, "ID_LIKE="); ok {
			idLike = strings.Trim(strings.ToLower(after), `"'`)
		}
	}

	if id == "fedora" {
		return FamilyFedora
	}
	if id == "arch" || id == "cachyos" || strings.Contains(idLike, "arch") {
		return FamilyArch
	}
	return FamilyUnknown
}

func cutPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

// RealUser returns the actual login user, even under sudo.
func RealUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	return os.Getenv("USER")
}

// HomeDir returns the real user's home directory.
func HomeDir() string {
	if home := os.Getenv("SUDO_HOME"); home != "" {
		return home
	}
	if u := RealUser(); u != "" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	home, _ := os.UserHomeDir()
	return home
}

// SudoCmd creates an exec.Cmd that runs the given command via sudo.
// It attaches stdin so the user can enter their password.
func SudoCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	allArgs := make([]string, 0, 1+len(args))
	allArgs = append(allArgs, name)
	allArgs = append(allArgs, args...)
	cmd := exec.CommandContext(ctx, "sudo", allArgs...)
	cmd.Stdin = os.Stdin
	return cmd
}

// RunSudo executes a command via sudo, capturing output.
func RunSudo(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := SudoCmd(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sudo %s %s: %w: %s", name, strings.Join(args, " "), err, out)
	}
	return out, nil
}
