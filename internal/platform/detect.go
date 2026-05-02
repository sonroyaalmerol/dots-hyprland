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
	defer func() { _ = f.Close() }()

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

// IsRoot reports whether the current process runs as uid 0.
func IsRoot() bool {
	return os.Geteuid() == 0
}

// RealUser returns the actual login user, even under sudo or when running as root
// from a system package manager.
func RealUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	if u := os.Getenv("USER"); u != "" && u != "root" {
		return u
	}
	// When running as root from a package manager, look up the first normal user.
	if IsRoot() {
		return detectFirstNormalUser()
	}
	return os.Getenv("USER")
}

// detectFirstNormalUser returns the first user with uid >= 1000 from /etc/passwd.
func detectFirstNormalUser() string {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "nobody" {
			continue
		}
		uid := 0
		fmt.Sscanf(fields[2], "%d", &uid)
		if uid >= 1000 && uid < 65534 {
			return fields[0]
		}
	}
	return ""
}

// HomeDir returns the real user's home directory.
func HomeDir() string {
	if home := os.Getenv("SUDO_HOME"); home != "" {
		return home
	}
	u := RealUser()
	if u != "" && u != "root" {
		// When not running as root, prefer $HOME
		if !IsRoot() {
			if home, err := os.UserHomeDir(); err == nil {
				return home
			}
		}
		// Running as root — look up home from passwd entry
		f, err := os.Open("/etc/passwd")
		if err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				fields := strings.Split(scanner.Text(), ":")
				if len(fields) >= 6 && fields[0] == u {
					return fields[5]
				}
			}
		}
	}
	// Fallback
	home, _ := os.UserHomeDir()
	return home
}

// SudoCmd creates an exec.Cmd that runs the given command via sudo.
// When already running as root, it runs the command directly.
func SudoCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	if IsRoot() {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdin = os.Stdin
		return cmd
	}
	allArgs := make([]string, 0, 1+len(args))
	allArgs = append(allArgs, name)
	allArgs = append(allArgs, args...)
	cmd := exec.CommandContext(ctx, "sudo", allArgs...)
	cmd.Stdin = os.Stdin
	return cmd
}

// RunSudo executes a command via sudo, capturing output.
// When already running as root, it runs the command directly.
func RunSudo(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := SudoCmd(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		prefix := "sudo "
		if IsRoot() {
			prefix = ""
		}
		return nil, fmt.Errorf("%s%s %s: %w: %s", prefix, name, strings.Join(args, " "), err, out)
	}
	return out, nil
}
