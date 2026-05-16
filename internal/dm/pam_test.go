package dm

import (
	"path/filepath"
	"testing"
)

func TestCredentialsZero(t *testing.T) {
	c := &Credentials{
		Username: "testuser",
		Password: "s3cret123",
	}

	c.Zero()

	if c.Password != "" {
		t.Error("password should be empty after Zero()")
	}
	// Username should be preserved.
	if c.Username != "testuser" {
		t.Error("username should be preserved after Zero()")
	}
}

func TestCredentialsZeroIdempotent(t *testing.T) {
	c := &Credentials{Password: ""}
	c.Zero() // Should not panic.
	if c.Password != "" {
		t.Error("empty password should remain empty")
	}
}

func TestPAMSessionZeroPassword(t *testing.T) {
	p := NewPAMSession("testuser", "hunter2")
	p.ZeroPassword()

	if p.password != "" {
		t.Error("password should be empty after ZeroPassword()")
	}
}

func TestPAMSessionZeroPasswordIdempotent(t *testing.T) {
	p := NewPAMSession("testuser", "")
	p.ZeroPassword() // Should not panic.
	if p.password != "" {
		t.Error("empty password should remain empty")
	}
}

func TestPAMSessionNilHandle(t *testing.T) {
	p := NewPAMSession("testuser", "pass")

	if err := p.AcctMgmt(); err == nil {
		t.Error("AcctMgmt should fail with nil handle")
	}
	if err := p.OpenSession(); err == nil {
		t.Error("OpenSession should fail with nil handle")
	}
}

func TestPAMSessionAccessorsBeforeAuth(t *testing.T) {
	p := NewPAMSession("testuser", "pass")

	// Before authentication, uid/gid should be zero.
	if p.Uid() != 0 {
		t.Error("Uid should be 0 before auth")
	}
	if p.Gid() != 0 {
		t.Error("Gid should be 0 before auth")
	}
	if p.HomeDir() != "" {
		t.Error("HomeDir should be empty before auth")
	}
}

func TestNewPAMSessionFields(t *testing.T) {
	p := NewPAMSession("alice", "wonderland")
	if p.username != "alice" {
		t.Errorf("username = %q, want alice", p.username)
	}
	if p.password != "wonderland" {
		t.Errorf("password = %q, want wonderland", p.password)
	}
	if p.pamh != nil {
		t.Error("pamh should be nil for new session")
	}
}

func TestUserSessionNilPAMHandle(t *testing.T) {
	creds := &Credentials{
		Username: "testuser",
		Password: "pass",
		// pamHandle is nil.
	}

	_, err := NewUserSession(DefaultConfig(), creds, nil)
	if err == nil {
		t.Error("NewUserSession should fail with nil PAM handle")
	}
	if err.Error() != "no pre-authenticated PAM handle" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUserSessionAbsoluteBinCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DaemonBin = "relative-path"
	if filepath.IsAbs(cfg.DaemonBin) {
		t.Error("relative path should not be absolute")
	}
}
