package dm

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Greeter manages the greeter processes: a Hyprland compositor and Quickshell
// running with greeter.qml. It runs as a dedicated system user for isolation.
type Greeter struct {
	cfg        Config
	vt         *VT
	hyprland   *exec.Cmd
	quickshell *exec.Cmd
	signature  string
}

// NewGreeter starts the greeter on the given VT.
// uid/gid are the greeter user's IDs (resolved by the DM).
func NewGreeter(cfg Config, uid, gid uint32, vt *VT) (*Greeter, error) {
	g := &Greeter{
		cfg:       cfg,
		vt:        vt,
		signature: "snry-greeter",
	}

	// Verify the binary paths are absolute.
	if !filepath.IsAbs(cfg.HyprlandBin) {
		return nil, fmt.Errorf("HyprlandBin must be an absolute path, got: %s", cfg.HyprlandBin)
	}
	if !filepath.IsAbs(cfg.QSBin) {
		return nil, fmt.Errorf("QSBin must be an absolute path, got: %s", cfg.QSBin)
	}

	// Ensure the greeter user has an XDG runtime directory.
	greeterRuntimeDir := fmt.Sprintf("/run/user/%d", uid)
	if err := os.MkdirAll(greeterRuntimeDir, 0700); err != nil {
		return nil, fmt.Errorf("create greeter runtime dir: %w", err)
	}
	if err := os.Chown(greeterRuntimeDir, int(uid), int(gid)); err != nil {
		return nil, fmt.Errorf("chown greeter runtime dir: %w", err)
	}

	// Ensure Hyprland instance directory exists.
	hyprDir := filepath.Join(greeterRuntimeDir, "hypr", g.signature)
	if err := os.MkdirAll(hyprDir, 0700); err != nil {
		return nil, fmt.Errorf("create hypr dir: %w", err)
	}
	if err := os.Chown(hyprDir, int(uid), int(gid)); err != nil {
		return nil, fmt.Errorf("chown hypr dir: %w", err)
	}

	// Ensure the greeter home directory exists.
	greeterHome := fmt.Sprintf("/var/lib/%s", cfg.GreeterUser)
	if err := os.MkdirAll(greeterHome, 0750); err != nil {
		return nil, fmt.Errorf("create greeter home: %w", err)
	}
	if err := os.Chown(greeterHome, int(uid), int(gid)); err != nil {
		return nil, fmt.Errorf("chown greeter home: %w", err)
	}

	if err := g.startHyprland(uid, gid, greeterRuntimeDir); err != nil {
		return nil, fmt.Errorf("start greeter Hyprland: %w", err)
	}

	if err := g.waitForHyprland(greeterRuntimeDir); err != nil {
		g.Kill()
		return nil, fmt.Errorf("wait for hyprland: %w", err)
	}

	if err := g.startQuickshell(uid, gid, greeterRuntimeDir); err != nil {
		g.Kill()
		return nil, fmt.Errorf("start greeter Quickshell: %w", err)
	}

	return g, nil
}

func (g *Greeter) startHyprland(uid, gid uint32, runtimeDir string) error {
	if _, err := os.Stat(g.cfg.HyprlandBin); err != nil {
		return fmt.Errorf("Hyprland binary not found at %s: %w", g.cfg.HyprlandBin, err)
	}

	g.hyprland = exec.Command(g.cfg.HyprlandBin)
	g.hyprland.Env = []string{
		fmt.Sprintf("HOME=/var/lib/%s", g.cfg.GreeterUser),
		fmt.Sprintf("USER=%s", g.cfg.GreeterUser),
		fmt.Sprintf("XDG_RUNTIME_DIR=%s", runtimeDir),
		fmt.Sprintf("XDG_CONFIG_HOME=/var/lib/%s/.config", g.cfg.GreeterUser),
		fmt.Sprintf("HYPRLAND_INSTANCE_SIGNATURE=%s", g.signature),
		"WAYLAND_DISPLAY=wayland-greeter",
		"PATH=/usr/local/bin:/usr/bin:/bin",
		fmt.Sprintf("XDG_VTNR=%d", g.vt.Num()),
		"HYPRLAND_CONFIG=/usr/share/snry-shell/configs/hyprland/hyprland-greeter/hyprland.conf",
	}
	// Don't inherit root's stdout/stderr — use /dev/null or dedicated log.
	g.hyprland.Stdout = nil
	g.hyprland.Stderr = nil
	g.hyprland.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
	}

	log.Printf("[dm/greeter] starting Hyprland (uid=%d, vt=%d)", uid, g.vt.Num())
	return g.hyprland.Start()
}

func (g *Greeter) waitForHyprland(runtimeDir string) error {
	sockPath := filepath.Join(runtimeDir, "hypr", g.signature, ".socket.sock")

	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			return fmt.Errorf("timeout waiting for Hyprland socket")
		case <-ticker.C:
			if _, err := os.Stat(sockPath); err == nil {
				log.Printf("[dm/greeter] Hyprland socket ready")
				return nil
			}
		}
	}
}

func (g *Greeter) startQuickshell(uid, gid uint32, runtimeDir string) error {
	qmlPath := g.cfg.GreeterQMLPath
	if qmlPath == "" {
		return fmt.Errorf("greeter QML path not configured")
	}
	if _, err := os.Stat(qmlPath); err != nil {
		return fmt.Errorf("greeter QML not found at %s: %w", qmlPath, err)
	}

	g.quickshell = exec.Command(g.cfg.QSBin, "-p", qmlPath)
	g.quickshell.Env = []string{
		fmt.Sprintf("HOME=/var/lib/%s", g.cfg.GreeterUser),
		fmt.Sprintf("USER=%s", g.cfg.GreeterUser),
		fmt.Sprintf("XDG_RUNTIME_DIR=%s", runtimeDir),
		fmt.Sprintf("XDG_CONFIG_HOME=/var/lib/%s/.config", g.cfg.GreeterUser),
		fmt.Sprintf("HYPRLAND_INSTANCE_SIGNATURE=%s", g.signature),
		"WAYLAND_DISPLAY=wayland-greeter",
		"PATH=/usr/local/bin:/usr/bin:/bin",
		fmt.Sprintf("SNRY_DAEMON_SOCK=%s", g.cfg.SocketPath),
		"SNRY_DM_MODE=1",
	}
	g.quickshell.Stdout = nil
	g.quickshell.Stderr = nil
	g.quickshell.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
	}

	log.Printf("[dm/greeter] starting Quickshell with %s", qmlPath)
	return g.quickshell.Start()
}

// Kill terminates the greeter processes with SIGTERM, escalating to
// SIGKILL after 5 seconds if they haven't exited.
func (g *Greeter) Kill() {
	killProcWithTimeout(g.quickshell, "quickshell")
	killProcWithTimeout(g.hyprland, "hyprland")
}

// lookupUserIDs looks up uid/gid from /etc/passwd for the given username.
func lookupUserIDs(username string) (int, int, error) {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return 0, 0, err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 4 && fields[0] == username {
			uid, err := strconv.Atoi(fields[2])
			if err != nil {
				return 0, 0, fmt.Errorf("invalid uid for %s: %s", username, fields[2])
			}
			gid, err := strconv.Atoi(fields[3])
			if err != nil {
				return 0, 0, fmt.Errorf("invalid gid for %s: %s", username, fields[3])
			}
			return uid, gid, nil
		}
	}
	return 0, 0, fmt.Errorf("user %s not found in /etc/passwd", username)
}

// killProcWithTimeout sends SIGTERM then escalates to SIGKILL after 5s.
func killProcWithTimeout(cmd *exec.Cmd, name string) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return
	}

	// SIGTERM first.
	syscall.Kill(-pgid, syscall.SIGTERM)
	log.Printf("[dm/greeter] sent SIGTERM to %s (pgid %d)", name, pgid)

	// Wait up to 5s, then SIGKILL.
	done := make(chan struct{})
	go func() {
		cmd.Process.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		syscall.Kill(-pgid, syscall.SIGKILL)
		log.Printf("[dm/greeter] sent SIGKILL to %s (pgid %d)", name, pgid)
	}
}
