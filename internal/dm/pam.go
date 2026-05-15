package dm

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/msteinert/pam/v2"
)

// PAMSession manages a PAM authentication and session lifecycle.
type PAMSession struct {
	username string
	password string
	pamh     *pam.Transaction
	uid      int
	gid      int
	homeDir  string
}

// NewPAMSession creates a new PAM session for the given credentials.
func NewPAMSession(username, password string) *PAMSession {
	return &PAMSession{username: username, password: password}
}

// Authenticate verifies the credentials via PAM.
func (p *PAMSession) Authenticate() error {
	pamh, err := pam.StartFunc("snry-dm", p.username, func(s pam.Style, msg string) (string, error) {
		switch s {
		case pam.PromptEchoOff:
			return p.password, nil
		case pam.PromptEchoOn:
			return p.username, nil
		case pam.ErrorMsg:
			log.Printf("[dm/pam] error: %s", msg)
			return "", nil
		case pam.TextInfo:
			log.Printf("[dm/pam] info: %s", msg)
			return "", nil
		default:
			return "", fmt.Errorf("unsupported PAM style: %v", s)
		}
	})
	if err != nil {
		return fmt.Errorf("PAM start: %w", err)
	}

	if err := pamh.Authenticate(0); err != nil {
		pamh.End()
		return fmt.Errorf("PAM authenticate: %w", err)
	}

	p.pamh = pamh
	return nil
}

// AcctMgmt performs PAM account management checks.
func (p *PAMSession) AcctMgmt() error {
	if p.pamh == nil {
		return fmt.Errorf("PAM session not initialized")
	}
	if err := p.pamh.AcctMgmt(0); err != nil {
		return fmt.Errorf("PAM acct mgmt: %w", err)
	}
	return nil
}

// OpenSession opens a PAM session and populates the user info.
func (p *PAMSession) OpenSession() error {
	if p.pamh == nil {
		return fmt.Errorf("PAM session not initialized")
	}

	// Resolve user info.
	u, err := user.Lookup(p.username)
	if err != nil {
		return fmt.Errorf("lookup user %s: %w", p.username, err)
	}
	p.uid, _ = strconv.Atoi(u.Uid)
	p.gid, _ = strconv.Atoi(u.Gid)
	p.homeDir = u.HomeDir

	// Set PAM environment items.
	pamh := p.pamh
	_ = pamh.SetItem(pam.User, p.username)

	if err := pamh.OpenSession(0); err != nil {
		return fmt.Errorf("PAM open session: %w", err)
	}

	log.Printf("[dm/pam] session opened for user %s (uid=%d)", p.username, p.uid)
	return nil
}

// CloseSession closes the PAM session.
func (p *PAMSession) CloseSession() {
	if p.pamh != nil {
		if err := p.pamh.CloseSession(0); err != nil {
			log.Printf("[dm/pam] close session error: %v", err)
		}
		p.pamh.End()
		p.pamh = nil
	}
}

// Env returns the environment variables for the user session,
// including PAM environment items and standard XDG variables.
func (p *PAMSession) Env() []string {
	env := []string{
		fmt.Sprintf("HOME=%s", p.homeDir),
		fmt.Sprintf("USER=%s", p.username),
		fmt.Sprintf("LOGNAME=%s", p.username),
		fmt.Sprintf("PATH=/usr/local/bin:/usr/bin:/bin"),
		fmt.Sprintf("SHELL=/bin/bash"),
		fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", p.uid),
		fmt.Sprintf("XDG_SESSION_TYPE=wayland"),
		fmt.Sprintf("XDG_SESSION_CLASS=user"),
		fmt.Sprintf("XDG_SESSION_DESKTOP=hyprland"),
		fmt.Sprintf("XDG_CURRENT_DESKTOP=Hyprland"),
		fmt.Sprintf("WAYLAND_DISPLAY=wayland-1"),
	}

	// Add PAM environment if available.
	if pamEnv, err := p.pamh.GetEnvList(); err == nil {
		for k, v := range pamEnv {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return env
}

// Uid returns the user's UID.
func (p *PAMSession) Uid() int { return p.uid }

// Gid returns the user's GID.
func (p *PAMSession) Gid() int { return p.gid }

// Username returns the authenticated username.
func (p *PAMSession) Username() string { return p.username }

// HomeDir returns the user's home directory.
func (p *PAMSession) HomeDir() string { return p.homeDir }

// ensureRuntimeDir creates the user's XDG_RUNTIME_DIR if it doesn't exist.
func (p *PAMSession) ensureRuntimeDir() error {
	runtimeDir := fmt.Sprintf("/run/user/%d", p.uid)
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		if err := os.MkdirAll(runtimeDir, 0700); err != nil {
			return fmt.Errorf("create runtime dir: %w", err)
		}
		if err := os.Chown(runtimeDir, p.uid, p.gid); err != nil {
			return fmt.Errorf("chown runtime dir: %w", err)
		}
	}
	return nil
}

// Credentials represents user credentials received from the greeter.
type Credentials struct {
	Username string
	Password string
}

// UserSession manages the user's desktop session after PAM authentication.
type UserSession struct {
	cfg   Config
	creds *Credentials
	pam   *PAMSession
	vt    *VT
	cmd   *exec.Cmd
}

// NewUserSession creates a user session from authenticated credentials.
func NewUserSession(cfg Config, creds *Credentials, vt *VT) (*UserSession, error) {
	pam := NewPAMSession(creds.Username, creds.Password)

	// Re-authenticate (we already did this, but we need a fresh PAM handle
	// that we can open a session with).
	if err := pam.Authenticate(); err != nil {
		return nil, fmt.Errorf("re-authenticate: %w", err)
	}
	if err := pam.AcctMgmt(); err != nil {
		return nil, fmt.Errorf("acct mgmt: %w", err)
	}
	if err := pam.OpenSession(); err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}

	// Ensure XDG runtime directory exists.
	if err := pam.ensureRuntimeDir(); err != nil {
		pam.CloseSession()
		return nil, fmt.Errorf("runtime dir: %w", err)
	}

	return &UserSession{
		cfg:   cfg,
		creds: creds,
		pam:   pam,
		vt:    vt,
	}, nil
}

// Run starts the user's desktop session and blocks until it exits.
func (s *UserSession) Run(ctx context.Context) error {
	// Activate the VT for the user session.
	if s.vt != nil {
		s.vt.Activate()
	}

	// Build the user's Hyprland + snry-daemon command.
	// We start snry-daemon which internally launches Hyprland.
	daemonBin, err := exec.LookPath(s.cfg.DaemonBin)
	if err != nil {
		return fmt.Errorf("find %s: %w", s.cfg.DaemonBin, err)
	}

	s.cmd = exec.CommandContext(ctx, daemonBin, "daemon")
	s.cmd.Env = s.pam.Env()
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr
	s.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Credential: &syscall.Credential{
			Uid: uint32(s.pam.Uid()),
			Gid: uint32(s.pam.Gid()),
		},
	}

	log.Printf("[dm] starting user session for %s (uid=%d)", s.creds.Username, s.pam.Uid())

	if err := s.cmd.Run(); err != nil {
		return fmt.Errorf("user session exited: %w", err)
	}
	return nil
}

// Close cleans up the user session.
func (s *UserSession) Close() {
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Signal(syscall.SIGTERM)
	}
	if s.pam != nil {
		s.pam.CloseSession()
	}
}

// ResolveConfigPath resolves the frontend config path.
// This is exported for use by the DM.
func ResolveConfigPath() string {
	systemDir := "/usr/share/snry-shell/frontend/ii"
	if _, err := os.Stat(filepath.Join(systemDir, "shell.qml")); err == nil {
		return systemDir
	}
	// Development fallback.
	if _, err := os.Stat("frontend/ii/shell.qml"); err == nil {
		abs, _ := filepath.Abs("frontend/ii")
		return abs
	}
	return systemDir // fallback to system path
}
