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
func NewGreeter(cfg Config, vt *VT) (*Greeter, error) {
	g := &Greeter{
		cfg:       cfg,
		vt:        vt,
		signature: "snry-greeter",
	}

	greeterUser := cfg.GreeterUser
	uid, gid, err := lookupUserIDs(greeterUser)
	if err != nil {
		return nil, fmt.Errorf("lookup greeter user %s: %w (create with: sudo useradd -r -M -s /bin/false snry-dm)", greeterUser, err)
	}

	// Ensure the greeter user has an XDG runtime directory.
	greeterRuntimeDir := fmt.Sprintf("/run/user/%d", uid)
	if err := os.MkdirAll(greeterRuntimeDir, 0700); err != nil {
		return nil, fmt.Errorf("create greeter runtime dir: %w", err)
	}
	os.Chown(greeterRuntimeDir, uid, gid)

	// Ensure Hyprland instance directory exists.
	hyprDir := filepath.Join(greeterRuntimeDir, "hypr", g.signature)
	if err := os.MkdirAll(hyprDir, 0700); err != nil {
		return nil, fmt.Errorf("create hypr dir: %w", err)
	}
	os.Chown(hyprDir, uid, gid)

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

func (g *Greeter) startHyprland(uid, gid int, runtimeDir string) error {
	bin, err := exec.LookPath("Hyprland")
	if err != nil {
		return fmt.Errorf("Hyprland binary not found: %w", err)
	}

	g.hyprland = exec.Command(bin)
	g.hyprland.Env = []string{
		fmt.Sprintf("HOME=/var/lib/%s", g.cfg.GreeterUser),
		fmt.Sprintf("USER=%s", g.cfg.GreeterUser),
		fmt.Sprintf("XDG_RUNTIME_DIR=%s", runtimeDir),
		fmt.Sprintf("XDG_CONFIG_HOME=/var/lib/%s/.config", g.cfg.GreeterUser),
		fmt.Sprintf("HYPRLAND_INSTANCE_SIGNATURE=%s", g.signature),
		"WAYLAND_DISPLAY=wayland-greeter",
		"PATH=/usr/local/bin:/usr/bin:/bin",
		fmt.Sprintf("XDG_VTNR=%d", g.vt.Num()),
	}
	g.hyprland.Stdout = os.Stdout
	g.hyprland.Stderr = os.Stderr
	g.hyprland.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
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

func (g *Greeter) startQuickshell(uid, gid int, runtimeDir string) error {
	qmlPath := g.cfg.GreeterQMLPath
	if qmlPath == "" {
		return fmt.Errorf("greeter QML path not configured")
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
	g.quickshell.Stdout = os.Stdout
	g.quickshell.Stderr = os.Stderr
	g.quickshell.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}

	log.Printf("[dm/greeter] starting Quickshell with %s", qmlPath)
	return g.quickshell.Start()
}

// Kill terminates the greeter processes.
func (g *Greeter) Kill() {
	killProc(g.quickshell, "quickshell")
	killProc(g.hyprland, "hyprland")
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
			uid, _ := strconv.Atoi(fields[2])
			gid, _ := strconv.Atoi(fields[3])
			return uid, gid, nil
		}
	}
	return 0, 0, fmt.Errorf("user %s not found in /etc/passwd", username)
}

func killProc(cmd *exec.Cmd, name string) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	}
	log.Printf("[dm/greeter] killed %s (pid %d)", name, cmd.Process.Pid)
}
