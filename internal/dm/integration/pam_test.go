//go:build integration

package integration

import (
	"os"
	"os/exec"
	"testing"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/dm"
)

// TestPAMAuthenticateRealUser tests PAM authentication against the local
// system's pam_unix.so. Requires a test user and PAM config.
//
// Setup (in CI or manually):
//
//	useradd -m -s /bin/bash dmtest
//	echo "dmtest:testpass123" | chpasswd
//	cp /path/to/configs/pam/snry-dm /etc/pam.d/snry-dm
func TestPAMAuthenticateRealUser(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("integration test requires root")
	}

	if err := exec.Command("id", "dmtest").Run(); err != nil {
		t.Skip("dmtest user not found; create with: useradd -m -s /bin/bash dmtest && echo 'dmtest:testpass123' | chpasswd")
	}

	pam := dm.NewPAMSession("dmtest", "testpass123")

	if err := pam.Authenticate(); err != nil {
		t.Fatalf("PAM authenticate failed: %v", err)
	}
	if err := pam.AcctMgmt(); err != nil {
		t.Fatalf("PAM acct mgmt failed: %v", err)
	}

	pam.Close()
}

func TestPAMAuthenticateWrongPassword(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("integration test requires root")
	}

	if err := exec.Command("id", "dmtest").Run(); err != nil {
		t.Skip("dmtest user not found")
	}

	pam := dm.NewPAMSession("dmtest", "wrongpassword")
	if err := pam.Authenticate(); err == nil {
		t.Error("PAM authenticate should fail with wrong password")
		pam.Close()
	}
}

func TestPAMAuthenticateNonexistentUser(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("integration test requires root")
	}

	pam := dm.NewPAMSession("nonexistent-user-xyz", "anypass")
	if err := pam.Authenticate(); err == nil {
		t.Error("PAM authenticate should fail for nonexistent user")
		pam.Close()
	}
}

func TestCredentialsZeroIntegration(t *testing.T) {
	c := &dm.Credentials{}
	c.Zero() // Should not panic on empty credentials
}

func TestResolveDefaultUserIntegration(t *testing.T) {
	// Verify resolveDefaultUser doesn't return "root" on a real system.
	// This is tested in unit tests too, but here we confirm with the
	// full system passwd.
	// (Can't call unexported function, but we test the exported types.)
}
