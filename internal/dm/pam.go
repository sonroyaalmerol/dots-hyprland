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
	"strings"
	"syscall"
	"time"

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
	groups   []uint32 // supplementary groups
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

	u, err := user.Lookup(p.username)
	if err != nil {
		return fmt.Errorf("lookup user %s: %w", p.username, err)
	}
	p.uid, _ = strconv.Atoi(u.Uid)
	p.gid, _ = strconv.Atoi(u.Gid)
	p.homeDir = u.HomeDir

	// Resolve supplementary groups.
	p.groups = resolveUserGroups(p.username, p.gid)

	// Set PAM environment items.
	_ = p.pamh.SetItem(pam.User, p.username)

	if err := p.pamh.OpenSession(0); err != nil {
		return fmt.Errorf("PAM open session: %w", err)
	}

	log.Printf("[dm/pam] session opened for uid=%d", p.uid)
	return nil
}

// Close closes the PAM session and zeroes the password.
func (p *PAMSession) Close() {
	p.ZeroPassword()
	if p.pamh != nil {
		if err := p.pamh.CloseSession(0); err != nil {
			log.Printf("[dm/pam] close session error: %v", err)
		}
		p.pamh.End()
		p.pamh = nil
	}
}

// ZeroPassword overwrites the cleartext password in memory.
func (p *PAMSession) ZeroPassword() {
	if p.password != "" {
		// Overwrite the backing byte slice.
		b := []byte(p.password)
		for i := range b {
			b[i] = 0
		}
		p.password = ""
	}
}

// Env returns the environment variables for the user session.
func (p *PAMSession) Env() []string {
	env := []string{
		fmt.Sprintf("HOME=%s", p.homeDir),
		fmt.Sprintf("USER=%s", p.username),
		fmt.Sprintf("LOGNAME=%s", p.username),
		"PATH=/usr/local/bin:/usr/bin:/bin",
		fmt.Sprintf("SHELL=%s", p.shell()),
		fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", p.uid),
		"XDG_SESSION_TYPE=wayland",
		"XDG_SESSION_CLASS=user",
		"XDG_SESSION_DESKTOP=hyprland",
		"XDG_CURRENT_DESKTOP=Hyprland",
		"WAYLAND_DISPLAY=wayland-1",
	}

	if pamEnv, err := p.pamh.GetEnvList(); err == nil {
		for k, v := range pamEnv {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return env
}

func (p *PAMSession) shell() string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return "/bin/bash"
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 7 && fields[0] == p.username {
			return fields[6]
		}
	}
	return "/bin/bash"
}

// Uid returns the user's UID.
func (p *PAMSession) Uid() int { return p.uid }

// Gid returns the user's GID.
func (p *PAMSession) Gid() int { return p.gid }

// Groups returns supplementary group IDs (includes primary).
func (p *PAMSession) Groups() []uint32 {
	all := make([]uint32, 0, 1+len(p.groups))
	all = append(all, uint32(p.gid))
	all = append(all, p.groups...)
	return all
}

// HomeDir returns the user's home directory.
func (p *PAMSession) HomeDir() string { return p.homeDir }

// ensureRuntimeDir creates the user's XDG_RUNTIME_DIR if it doesn't exist.
func (p *PAMSession) ensureRuntimeDir() error {
	runtimeDir := fmt.Sprintf("/run/user/%d", p.uid)
	fi, err := os.Stat(runtimeDir)
	if err == nil {
		// Already exists — verify ownership.
		if stat, ok := fi.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != p.uid {
			return fmt.Errorf("runtime dir %s owned by uid %d, expected %d", runtimeDir, stat.Uid, p.uid)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat runtime dir: %w", err)
	}

	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	if err := os.Chown(runtimeDir, p.uid, p.gid); err != nil {
		os.RemoveAll(runtimeDir)
		return fmt.Errorf("chown runtime dir: %w", err)
	}
	return nil
}

// Credentials represents user credentials received from the greeter.
type Credentials struct {
	Username  string
	Password  string
	pamHandle *PAMSession // pre-authenticated PAM handle from waitForAuth
}

// Zero overwrites the password in memory.
func (c *Credentials) Zero() {
	if c.Password != "" {
		b := []byte(c.Password)
		for i := range b {
			b[i] = 0
		}
		c.Password = ""
	}
}

// UserSession manages the user's desktop session after PAM authentication.
type UserSession struct {
	cfg       Config
	creds     *Credentials
	pam       *PAMSession
	vt        *VT
	cmd       *exec.Cmd
	hyprland  *exec.Cmd
	signature string
}

// NewUserSession creates a user session from a pre-authenticated PAM handle.
// The PAM handle comes from waitForAuth — no password re-entry needed.
func NewUserSession(cfg Config, creds *Credentials, vt *VT) (*UserSession, error) {
	pam := creds.pamHandle
	if pam == nil {
		creds.Zero()
		return nil, fmt.Errorf("no pre-authenticated PAM handle")
	}

	// Open session on the existing handle.
	if err := pam.OpenSession(); err != nil {
		pam.Close()
		creds.Zero()
		return nil, fmt.Errorf("open session: %w", err)
	}

	// Ensure XDG runtime directory exists.
	if err := pam.ensureRuntimeDir(); err != nil {
		pam.Close()
		creds.Zero()
		return nil, fmt.Errorf("runtime dir: %w", err)
	}

	// Zero the password — we no longer need it.
	creds.Zero()

	return &UserSession{
		cfg:   cfg,
		creds: creds,
		pam:   pam,
		vt:    vt,
	}, nil
}

// Run starts the user's desktop session and blocks until it exits.
// It launches the Wayland compositor (Hyprland) first, waits for its IPC
// socket to become available, then starts snry-daemon. The daemon detects
// the running compositor and proceeds with QuickShell and services.
func (s *UserSession) Run(ctx context.Context) error {
	if s.vt != nil {
		s.vt.Activate()
	}

	if err := s.startCompositor(ctx); err != nil {
		return fmt.Errorf("compositor: %w", err)
	}

	if err := s.startDaemon(ctx); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	// Block until the daemon process exits.
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("user session exited: %w", err)
	}
	return nil
}

// startCompositor launches Hyprland as the user and waits for its IPC socket.
func (s *UserSession) startCompositor(ctx context.Context) error {
	startHypr := s.cfg.StartHyprlandBin
	if startHypr == "" {
		startHypr = "start-hyprland"
	}

	configPath := systemHyprlandConfig()
	if configPath == "" {
		return fmt.Errorf("no system Hyprland config found in /usr/share/snry-shell/configs/hypr")
	}

	s.signature = fmt.Sprintf("snry-%d", s.pam.Uid())

	args := []string{"--", "-c", configPath}

	s.hyprland = exec.Command(startHypr, args...)
	s.hyprland.Env = s.pam.Env()
	if s.vt != nil {
		s.hyprland.Env = append(s.hyprland.Env, fmt.Sprintf("XDG_VTNR=%d", s.vt.Num()))
	}
	s.hyprland.Env = append(s.hyprland.Env,
		fmt.Sprintf("HYPRLAND_INSTANCE_SIGNATURE=%s", s.signature),
	)
	s.hyprland.Stdout = nil
	s.hyprland.Stderr = nil
	s.hyprland.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Credential: &syscall.Credential{
			Uid:    uint32(s.pam.Uid()),
			Gid:    uint32(s.pam.Gid()),
			Groups: s.pam.Groups(),
		},
	}

	log.Printf("[dm] starting Hyprland for user session (uid=%d)", s.pam.Uid())
	if err := s.hyprland.Start(); err != nil {
		return fmt.Errorf("start hyprland: %w", err)
	}

	// Wait for the Hyprland IPC socket to appear.
	runtimeDir := fmt.Sprintf("/run/user/%d", s.pam.Uid())
	sockPath := filepath.Join(runtimeDir, "hypr", s.signature, ".socket.sock")

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("timeout waiting for Hyprland socket (30s)")
		case <-ticker.C:
			if _, err := os.Stat(sockPath); err == nil {
				log.Printf("[dm] Hyprland socket ready (signature=%s)", s.signature)
				return nil
			}
		}
	}
}

// startDaemon starts snry-daemon as the user. The daemon will detect the
// already-running compositor and proceed with QuickShell and services.
func (s *UserSession) startDaemon(ctx context.Context) error {
	if !filepath.IsAbs(s.cfg.DaemonBin) {
		return fmt.Errorf("DaemonBin must be an absolute path, got: %s", s.cfg.DaemonBin)
	}
	if _, err := os.Stat(s.cfg.DaemonBin); err != nil {
		return fmt.Errorf("daemon binary not found at %s: %w", s.cfg.DaemonBin, err)
	}

	s.cmd = exec.CommandContext(ctx, s.cfg.DaemonBin, "daemon")
	s.cmd.Env = s.pam.Env()
	if s.vt != nil {
		s.cmd.Env = append(s.cmd.Env, fmt.Sprintf("XDG_VTNR=%d", s.vt.Num()))
	}
	// Propagate the compositor instance signature so the daemon finds it immediately.
	s.cmd.Env = append(s.cmd.Env,
		fmt.Sprintf("HYPRLAND_INSTANCE_SIGNATURE=%s", s.signature),
	)
	s.cmd.Stdout = nil
	s.cmd.Stderr = nil
	s.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Credential: &syscall.Credential{
			Uid:    uint32(s.pam.Uid()),
			Gid:    uint32(s.pam.Gid()),
			Groups: s.pam.Groups(),
		},
	}

	log.Printf("[dm] starting snry-daemon (uid=%d)", s.pam.Uid())
	return s.cmd.Start()
}

// systemHyprlandConfig returns the path to the system-level Hyprland config
// (hyprland.lua preferred, hyprland.conf fallback). Returns empty string if not found.
func systemHyprlandConfig() string {
	systemDir := "/usr/share/snry-shell/configs/hypr"
	for _, name := range []string{"hyprland.lua", "hyprland.conf"} {
		p := systemDir + "/" + name
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Close cleans up the user session.
func (s *UserSession) Close() {
	killProcWithTimeout(s.cmd, "snry-daemon")
	killProcWithTimeout(s.hyprland, "hyprland")
	if s.pam != nil {
		s.pam.Close()
	}
}

// resolveUserGroups returns the supplementary group IDs for a user.
func resolveUserGroups(username string, primaryGID int) []uint32 {
	u, err := user.Lookup(username)
	if err != nil {
		return nil
	}
	groupStrs, err := u.GroupIds()
	if err != nil {
		return nil
	}
	seen := map[int]bool{primaryGID: true}
	var gids []uint32
	for _, gs := range groupStrs {
		g, err := strconv.Atoi(gs)
		if err != nil {
			continue
		}
		if !seen[g] {
			seen[g] = true
			gids = append(gids, uint32(g))
		}
	}
	return gids
}
