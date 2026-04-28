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
)

type Config struct {
	Binary       string        // path to qs binary (default: "qs")
	ConfigDir    string        // -c flag value (default: "ii")
	StartDelay   time.Duration // delay before first start (default: 0)
	RestartDelay time.Duration // delay between restarts (default: 1s)
	MaxRestarts  int           // max consecutive restarts before giving up (default: 10, 0 = unlimited)
}

func DefaultConfig() Config {
	return Config{
		Binary:       "qs",
		ConfigDir:    "ii",
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
	s.cmd = exec.CommandContext(ctx, s.cfg.Binary, "-c", s.cfg.ConfigDir)
	s.cmd.Env = os.Environ()
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	s.mu.Unlock()

	// Kill any pre-existing qs instances to prevent duplicate bars.
	if existing := exec.Command("pkill", "-f", fmt.Sprintf("%s -c %s", s.cfg.Binary, s.cfg.ConfigDir)); existing.Run() == nil {
		log.Printf("quickshell: killed pre-existing %s -c %s", s.cfg.Binary, s.cfg.ConfigDir)
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("quickshell: starting %s -c %s", s.cfg.Binary, s.cfg.ConfigDir)
	err := s.cmd.Run()
	log.Printf("quickshell: process exited: %v", err)
	return err
}

// Stop gracefully terminates the managed qs process (used during daemon shutdown).
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}

	// Send SIGTERM to the process group
	pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	}

	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("quickshell: terminated gracefully")
	case <-time.After(5 * time.Second):
		log.Printf("quickshell: SIGTERM timeout, sending SIGKILL")
		if pgid > 0 {
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}
}
