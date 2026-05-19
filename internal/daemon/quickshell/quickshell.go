package quickshell

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/xdg"
)

type Config struct {
	Binary       string        // path to qs binary (default: "qs")
	ConfigPath   string        // -p flag value: absolute path to shell root dir
	StartDelay   time.Duration // delay before first start (default: 0)
	RestartDelay time.Duration // delay between restarts (default: 1s)
	MaxRestarts  int           // max consecutive restarts before giving up (default: 10, 0 = unlimited)
}

// ResolveConfigPath returns the quickshell config path, preferring the
// system-installed frontend under /usr/share, then the embedded frontend cache.
func ResolveConfigPath() string {
	// 1. System package install
	systemPath := "/usr/share/snry-shell/frontend/ii"
	if _, err := os.Stat(systemPath + "/shell.qml"); err == nil {
		return systemPath
	}

	// 2. Embedded frontend cache
	cacheDir := xdg.Resolve().CacheHome
	return cacheDir + "/snry-shell/embedded-frontend/ii"
}

func DefaultConfig() Config {
	return Config{
		Binary:       "qs",
		ConfigPath:   ResolveConfigPath(),
		StartDelay:   2 * time.Second,
		RestartDelay: 1 * time.Second,
		MaxRestarts:  10,
	}
}

type Service struct {
	cfg    Config
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func New(cfg Config) *Service {
	return &Service{cfg: cfg}
}

// Run starts the qs process, monitors it, and auto-restarts on crash.
// Blocks until ctx is cancelled or max restarts exceeded.
func (s *Service) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	defer cancel()

	if s.cfg.StartDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.cfg.StartDelay):
		}
	}

	restarts := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		startTime := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// If process ran for more than 10s, reset the counter
		if time.Since(startTime) > 10*time.Second {
			restarts = 0
		}
		restarts++

		if s.cfg.MaxRestarts > 0 && restarts > s.cfg.MaxRestarts {
			return fmt.Errorf("quickshell exceeded max restarts (%d)", s.cfg.MaxRestarts)
		}

		log.Printf("quickshell: exited (%v), restart %d/%d in %v",
			err, restarts, s.cfg.MaxRestarts, s.cfg.RestartDelay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.cfg.RestartDelay):
		}
	}
}

func (s *Service) runOnce(ctx context.Context) error {
	s.mu.Lock()
	s.cmd = exec.CommandContext(ctx, s.cfg.Binary, "-p", s.cfg.ConfigPath)
	s.cmd.Env = os.Environ()
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	s.mu.Unlock()

	// Kill any pre-existing qs instances to prevent duplicate bars.
	if existing := exec.Command("pkill", "-f", fmt.Sprintf("%s -p %s", s.cfg.Binary, s.cfg.ConfigPath)); existing.Run() == nil {
		log.Printf("quickshell: killed pre-existing %s -p %s", s.cfg.Binary, s.cfg.ConfigPath)
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("quickshell: starting %s -p %s", s.cfg.Binary, s.cfg.ConfigPath)
	err := s.cmd.Run()
	log.Printf("quickshell: process exited: %v", err)
	return err
}

// Stop gracefully terminates the managed qs process (used during daemon shutdown).
func (s *Service) Stop() {
	killQS(s)
}

// Restart kills the running qs process and lets the Run loop restart it.
func (s *Service) Restart() {
	killQS(s)
}

// killQS terminates the qs process only — NOT its entire process group.
// User apps launched via Quickshell.execDetached (terminals, browsers, etc.)
// are children of qs but should survive a daemon restart.  By killing only
// the qs PID we let the compositor (Hyprland) re-parent those windows while
// the daemon respawns a fresh qs instance.
func killQS(s *Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}

	pid := s.cmd.Process.Pid

	// Send SIGTERM to qs only (not the process group) so that detached
	// user apps like Ghostty and browsers are not affected.
	_ = syscall.Kill(pid, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("quickshell: terminated gracefully")
	case <-time.After(2 * time.Second):
		log.Printf("quickshell: SIGTERM timeout, sending SIGKILL")
		_ = syscall.Kill(pid, syscall.SIGKILL)
		<-done
	}
}
