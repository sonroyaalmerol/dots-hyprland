package dm

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// DM is the display manager orchestrator. It manages the greeter → session
// lifecycle: start a graphical greeter, authenticate the user via PAM,
// create a proper logind session, and start the user's desktop.
type DM struct {
	cfg        Config
	greeterUID uint32
	greeterGID uint32
	socket     *Socket
	greeter    *Greeter
	session    *UserSession
	vt         *VT
}

type Config struct {
	// SocketPath is the Unix domain socket the DM listens on for greeter IPC.
	SocketPath string
	// GreeterUser is the system user that runs the greeter compositor.
	GreeterUser string
	// GreeterQMLPath is the path to greeter.qml.
	GreeterQMLPath string
	// GreeterQSConfigDir is the parent directory of greeter.qml (for QS imports).
	GreeterQSConfigDir string
	// ShellQMLPath is the path to the full shell.qml (for user session).
	ShellQMLPath string
	// ShellQSConfigDir is the parent directory of shell.qml.
	ShellQSConfigDir string
	// HyprlandBin is the absolute path to Hyprland.
	HyprlandBin string
	// StartHyprlandBin is the absolute path to start-hyprland.
	StartHyprlandBin string
	// QSBin is the absolute path to the Quickshell binary.
	QSBin string
	// DaemonBin is the absolute path to snry-daemon.
	DaemonBin string
	// GreeterVT is the VT number for the greeter (0 = auto).
	GreeterVT int
}

// Known absolute binary paths. Using exec.LookPath is unsafe for setuid-root
// programs because a malicious PATH can substitute binaries.
var defaultBinPaths = []string{
	"/usr/bin",
	"/usr/local/bin",
}

func DefaultConfig() Config {
	return Config{
		SocketPath:       "/run/snry-dm.sock",
		GreeterUser:      "snry-dm",
		HyprlandBin:      findBinary("Hyprland"),
		StartHyprlandBin: findBinary("start-hyprland"),
		QSBin:            findBinary("qs"),
		DaemonBin:        findBinary("snry-daemon"),
		GreeterVT:        1,
	}
}

// findBinary resolves an absolute path from known directories.
// Returns the binary name if not found (will fail later with clear error).
func findBinary(name string) string {
	for _, dir := range defaultBinPaths {
		p := dir + "/" + name
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return name
}

// Run is the main DM loop.
func (dm *DM) Run(ctx context.Context) error {
	// Resolve greeter user IDs.
	uid, gid, err := lookupUserIDs(dm.cfg.GreeterUser)
	if err != nil {
		return fmt.Errorf("lookup greeter user: %w", err)
	}
	dm.greeterUID = uint32(uid)
	dm.greeterGID = uint32(gid)

	dm.resolvePaths()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := dm.runGreeterCycle(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("[dm] greeter cycle failed: %v, restarting in 3s", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
	}
}

func (dm *DM) runGreeterCycle(ctx context.Context) error {
	// Open and configure the VT.
	vt, err := OpenVT(dm.cfg.GreeterVT)
	if err != nil {
		return fmt.Errorf("open VT: %w", err)
	}
	dm.vt = vt
	defer func() {
		vt.SetTextMode()
		vt.Close()
	}()

	vt.SetGraphicsMode()
	vt.Activate()
	log.Printf("[dm] activated VT %d", vt.num)

	// Start the greeter (Hyprland + Quickshell on VT).
	greeter, err := NewGreeter(dm.cfg, dm.greeterUID, dm.greeterGID, vt)
	if err != nil {
		return fmt.Errorf("start greeter: %w", err)
	}
	dm.greeter = greeter
	defer greeter.Kill()

	// Start IPC socket for greeter auth (restricted to greeter user).
	sock := NewSocket(dm.cfg.SocketPath, dm.greeterUID, dm.greeterGID)
	dm.socket = sock

	sockCtx, sockCancel := context.WithCancel(ctx)
	defer sockCancel()

	go func() {
		if err := sock.Run(sockCtx); err != nil {
			log.Printf("[dm/socket] error: %v", err)
		}
	}()

	// Wait for successful authentication.
	creds, err := dm.waitForAuth(ctx, sock)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	log.Printf("[dm] user authenticated, starting session")

	// Kill the greeter before starting user session.
	sockCancel()
	greeter.Kill()
	os.Remove(dm.cfg.SocketPath)

	// Open PAM session for the user (no re-auth; password already verified).
	session, err := NewUserSession(dm.cfg, creds, vt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	dm.session = session

	// Run user session until it exits.
	sessionErr := session.Run(ctx)

	log.Printf("[dm] user session ended: %v", sessionErr)

	session.Close()
	return nil
}

// waitForAuth blocks until the greeter sends valid credentials via the IPC socket.
// On success, the password is zeroed from the Credentials after PAM verification.
func (dm *DM) waitForAuth(ctx context.Context, sock *Socket) (*Credentials, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case creds := <-sock.AuthCh():
			if creds == nil {
				continue
			}

			// Sanitize username for logging (no control chars, no newlines).
			safeName := sanitizeForLog(creds.Username)
			log.Printf("[dm] authenticating user '%s'", safeName)

			// Authenticate via PAM (single handle; reused for session).
			pam := NewPAMSession(creds.Username, creds.Password)
			if err := pam.Authenticate(); err != nil {
				log.Printf("[dm] PAM auth failed: %v", err)
				sock.SendAuthResultAll(false, "Authentication failed.")
				creds.Zero()
				continue
			}

			if err := pam.AcctMgmt(); err != nil {
				log.Printf("[dm] PAM acct check failed: %v", err)
				sock.SendAuthResultAll(false, "Authentication failed.")
				pam.Close()
				creds.Zero()
				continue
			}

			sock.SendAuthResultAll(true, "")

			// Hand the open PAM handle to the session creator — no re-auth needed.
			creds.pamHandle = pam
			return creds, nil
		}
	}
}

// resolvePaths resolves the frontend QML paths from system or development locations.
func (dm *DM) resolvePaths() {
	systemDir := "/usr/share/snry-shell/frontend/ii"

	if dm.cfg.GreeterQSConfigDir == "" {
		if verifyQMLDir(systemDir, "greeter.qml") {
			dm.cfg.GreeterQSConfigDir = systemDir
		}
	}
	if dm.cfg.ShellQSConfigDir == "" {
		if verifyQMLDir(systemDir, "shell.qml") {
			dm.cfg.ShellQSConfigDir = systemDir
		}
	}

	// Development fallback (only if running from source tree).
	if dm.cfg.GreeterQSConfigDir == "" {
		if verifyQMLDir("frontend/ii", "greeter.qml") {
			dm.cfg.GreeterQSConfigDir = "frontend/ii"
		}
	}
	if dm.cfg.ShellQSConfigDir == "" {
		if verifyQMLDir("frontend/ii", "shell.qml") {
			dm.cfg.ShellQSConfigDir = "frontend/ii"
		}
	}

	if dm.cfg.GreeterQMLPath == "" && dm.cfg.GreeterQSConfigDir != "" {
		dm.cfg.GreeterQMLPath = dm.cfg.GreeterQSConfigDir + "/greeter.qml"
	}
	if dm.cfg.ShellQMLPath == "" && dm.cfg.ShellQSConfigDir != "" {
		dm.cfg.ShellQMLPath = dm.cfg.ShellQSConfigDir + "/shell.qml"
	}
}

// verifyQMLDir checks that a QML file exists and is owned by root
// (prevents TOCTOU symlink attacks on system paths).
func verifyQMLDir(dir, file string) bool {
	p := dir + "/" + file
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	// For system paths, verify root ownership.
	if strings.HasPrefix(dir, "/usr/") {
		stat, ok := fi.Sys().(*syscall.Stat_t)
		if ok && stat.Uid != 0 {
			log.Printf("[dm] rejecting non-root-owned QML file: %s (uid=%d)", p, stat.Uid)
			return false
		}
	}
	return true
}

// sanitizeForLog strips control characters and newlines from untrusted strings.
func sanitizeForLog(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 && r != ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// RunFromMain is the CLI entrypoint.
func RunFromMain() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "snry-dm must run as root")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := DefaultConfig()
	dm := &DM{cfg: cfg}

	if err := dm.Run(ctx); err != nil && err != ctx.Err() {
		log.Fatalf("[dm] fatal: %v", err)
	}
}
