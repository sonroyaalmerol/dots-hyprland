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
	defer os.Setenv("HOME", orig)

	os.Setenv("HOME", "/test/home")
	if got := HomeDir(); got != "/test/home" {
		t.Errorf("got %q, want /test/home", got)
	}
}

func TestRealUser(t *testing.T) {
	// RealUser falls back to USER when SUDO_USER is unset
	origSudo := os.Getenv("SUDO_USER")
	origUser := os.Getenv("USER")
	defer func() {
		os.Setenv("SUDO_USER", origSudo)
		os.Setenv("USER", origUser)
	}()

	os.Unsetenv("SUDO_USER")
	os.Setenv("USER", "testuser")
	if got := RealUser(); got != "testuser" {
		t.Errorf("got %q, want testuser", got)
	}

	os.Setenv("SUDO_USER", "realuser")
	if got := RealUser(); got != "realuser" {
		t.Errorf("got %q, want realuser", got)
	}
}
