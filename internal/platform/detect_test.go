package platform

import (
	"os"
	"testing"
)

func TestDetectReturnsValidFamily(t *testing.T) {
	// Detect() reads /etc/os-release, which exists on most Linux systems.
	// We just verify it returns one of the known constants.
	d := Detect()
	switch d {
	case FamilyArch, FamilyFedora, FamilyUnknown:
		// ok
	default:
		t.Errorf("unexpected distro family: %q", d)
	}
}

func TestHomeDir(t *testing.T) {
	orig := os.Getenv("HOME")
	defer func() {
		_ = os.Setenv("HOME", orig)
	}()

	_ = os.Setenv("HOME", "/test/home")
	if got := HomeDir(); got != "/test/home" {
		t.Errorf("got %q, want /test/home", got)
	}
}

func TestRealUser(t *testing.T) {
	// RealUser falls back to USER when SUDO_USER is unset
	origSudo := os.Getenv("SUDO_USER")
	origUser := os.Getenv("USER")
	defer func() {
		_ = os.Setenv("SUDO_USER", origSudo)
		_ = os.Setenv("USER", origUser)
	}()

	_ = os.Unsetenv("SUDO_USER")
	_ = os.Setenv("USER", "testuser")
	if got := RealUser(); got != "testuser" {
		t.Errorf("got %q, want testuser", got)
	}

	_ = os.Setenv("SUDO_USER", "realuser")
	if got := RealUser(); got != "realuser" {
		t.Errorf("got %q, want realuser", got)
	}
}
