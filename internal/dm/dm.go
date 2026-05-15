package dm

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// DM is the display manager orchestrator. It manages the greeter → session
// lifecycle: start a graphical greeter, authenticate the user via PAM,
// create a proper logind session, and start the user's desktop.
type DM struct {
	cfg     Config
	socket  *Socket
	pam     *PAMSession
	greeter *Greeter
	session *UserSession
	vt      *VT
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
	// HyprlandBin is the path to Hyprland or start-hyprland.
	HyprlandBin string
	// QSBin is the path to the Quickshell binary.
	QSBin string
	// DaemonBin is the path to snry-daemon.
	DaemonBin string
	// GreeterVT is the VT number for the greeter (0 = auto).
	GreeterVT int
}

func DefaultConfig() Config {
	return Config{
		SocketPath:  "/run/snry-dm.sock",
		GreeterUser: "snry-dm",
		HyprlandBin: "start-hyprland",
		QSBin:       "qs",
		DaemonBin:   "snry-daemon",
		GreeterVT:   1,
	}
}

// Run is the main DM loop. It repeatedly starts the greeter, waits for
// successful authentication, starts the user session, and loops when the
// session ends.
func (dm *DM) Run(ctx context.Context) error {
	// Resolve frontend paths.
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

// runGreeterCycle runs one complete greeter → session cycle.
func (dm *DM) runGreeterCycle(ctx context.Context) error {
	// Open and configure the VT.
	vt, err := OpenVT(dm.cfg.GreeterVT)
	if err != nil {
		return fmt.Errorf("open VT: %w", err)
	}
	dm.vt = vt
	defer vt.Close()

	vt.SetGraphicsMode()
	vt.Activate()
	log.Printf("[dm] activated VT %d", vt.num)

	// Start the greeter (Hyprland + Quickshell on VT).
	greeter, err := NewGreeter(dm.cfg, vt)
	if err != nil {
		return fmt.Errorf("start greeter: %w", err)
	}
	dm.greeter = greeter
	defer greeter.Kill()

	// Start IPC socket for greeter auth.
	sock := NewSocket(dm.cfg.SocketPath)
	dm.socket = sock

	sockCtx, sockCancel := context.WithCancel(ctx)
	defer sockCancel()

	sockDone := make(chan error, 1)
	go func() {
		sockDone <- sock.Run(sockCtx)
	}()

	// Wait for successful authentication.
	creds, err := dm.waitForAuth(ctx, sock)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	log.Printf("[dm] user '%s' authenticated, starting session", creds.Username)

	// Kill the greeter.
	sockCancel()
	greeter.Kill()
	os.Remove(dm.cfg.SocketPath)

	// Open PAM session for the user.
	session, err := NewUserSession(dm.cfg, creds, vt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	dm.session = session

	// Run user session until it exits.
	sessionErr := session.Run(ctx)

	log.Printf("[dm] user session ended: %v", sessionErr)

	// Close PAM session.
	session.Close()

	return nil
}

// waitForAuth blocks until the greeter sends valid credentials via the IPC socket.
func (dm *DM) waitForAuth(ctx context.Context, sock *Socket) (*Credentials, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case creds := <-sock.AuthCh():
			if creds == nil {
				continue
			}

			log.Printf("[dm] authenticating user '%s'", creds.Username)

			// Authenticate via PAM.
			pam := NewPAMSession(creds.Username, creds.Password)
			err := pam.Authenticate()
			if err != nil {
				log.Printf("[dm] PAM auth failed for '%s': %v", creds.Username, err)
				sock.SendAuthResult(false, err.Error())
				continue
			}

			// Account management check.
			if err := pam.AcctMgmt(); err != nil {
				log.Printf("[dm] PAM acct check failed for '%s': %v", creds.Username, err)
				sock.SendAuthResult(false, err.Error())
				continue
			}

			sock.SendAuthResult(true, "")
			dm.pam = pam
			return creds, nil
		}
	}
}

// resolvePaths resolves the frontend QML paths from system or development locations.
func (dm *DM) resolvePaths() {
	// Try system install path first, then development path.
	systemDir := "/usr/share/snry-shell/frontend/ii"

	if dm.cfg.GreeterQSConfigDir == "" {
		if _, err := os.Stat(systemDir + "/greeter.qml"); err == nil {
			dm.cfg.GreeterQSConfigDir = systemDir
		}
	}
	if dm.cfg.ShellQSConfigDir == "" {
		if _, err := os.Stat(systemDir + "/shell.qml"); err == nil {
			dm.cfg.ShellQSConfigDir = systemDir
		}
	}

	// Development fallback: check relative to working directory.
	if dm.cfg.GreeterQSConfigDir == "" {
		if _, err := os.Stat("frontend/ii/greeter.qml"); err == nil {
			dm.cfg.GreeterQSConfigDir = "frontend/ii"
		}
	}
	if dm.cfg.ShellQSConfigDir == "" {
		if _, err := os.Stat("frontend/ii/shell.qml"); err == nil {
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

// RunFromMain is a convenience function for the main package.
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
