package quickshell

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Config struct {
	Binary       string        // path to qs binary (default: "qs")
	ConfigDir    string        // -c flag value (named config under ~/.config/quickshell/)
	ConfigPath   string        // -p flag value (absolute path, overrides ConfigDir if set)
	GreeterPath  string        // -p flag value for greeter config (absolute path)
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

// Phase represents the current display manager phase.
type Phase int

const (
	PhaseGreeter Phase = iota // Lock-screen-only config
	PhaseSession              // Full shell config
)

func (p Phase) String() string {
	switch p {
	case PhaseGreeter:
		return "greeter"
	case PhaseSession:
		return "session"
	default:
		return "unknown"
	}
}

type Service struct {
	cfg    Config
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	phase  atomic.Int32 // stores Phase as int32
}

func New(cfg Config) *Service {
	return &Service{cfg: cfg}
}

// GetPhase returns the current phase.
func (s *Service) GetPhase() Phase {
	return Phase(s.phase.Load())
}

// SetPhase updates the target phase. The config is picked up on next restart.
func (s *Service) SetPhase(p Phase) {
	s.phase.Store(int32(p))
}

// configPath returns the Quickshell config argument for the current phase.
// If ConfigPath or GreeterPath is set (non-empty), it uses -p (path mode).
// Otherwise it falls back to -c with ConfigDir.
func (s *Service) configArg() (flag, value string) {
	phase := s.GetPhase()
	switch phase {
	case PhaseGreeter:
		if s.cfg.GreeterPath != "" {
			return "-p", s.cfg.GreeterPath
		}
		return "-c", s.cfg.ConfigDir
	default: // PhaseSession
		if s.cfg.ConfigPath != "" {
			return "-p", s.cfg.ConfigPath
		}
		return "-c", s.cfg.ConfigDir
	}
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

		flag, value := s.configArg()
		log.Printf("quickshell: exited (%v), restart %d/%d in %v (phase=%s, %s %s)",
			err, restarts, s.cfg.MaxRestarts, s.cfg.RestartDelay, s.GetPhase(), flag, value)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.cfg.RestartDelay):
		}
	}
}

func (s *Service) runOnce(ctx context.Context) error {
	flag, value := s.configArg()

	s.mu.Lock()
	s.cmd = exec.CommandContext(ctx, s.cfg.Binary, flag, value)
	s.cmd.Env = os.Environ()
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	s.mu.Unlock()

	// Kill any pre-existing qs instances to prevent duplicate bars.
	if existing := exec.Command("pkill", "-f", fmt.Sprintf("%s %s %s", s.cfg.Binary, flag, value)); existing.Run() == nil {
		log.Printf("quickshell: killed pre-existing %s %s %s", s.cfg.Binary, flag, value)
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("quickshell: starting %s %s %s (phase=%s)", s.cfg.Binary, flag, value, s.GetPhase())
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

// TransitionToSession switches from greeter to full session config.
// It sets the phase and restarts Quickshell with the full shell config.
func (s *Service) TransitionToSession() {
	log.Printf("quickshell: transitioning from greeter to session phase")
	s.SetPhase(PhaseSession)
	s.Restart()
}

// TransitionToGreeter switches from session to greeter config.
// It sets the phase and restarts Quickshell with the lock-screen-only config.
func (s *Service) TransitionToGreeter() {
	log.Printf("quickshell: transitioning from session to greeter phase")
	s.SetPhase(PhaseGreeter)
	s.Restart()
}

// killQS terminates the running qs process group. The Run loop will detect
// the exit and restart automatically (unless the context is cancelled).
func killQS(s *Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}

	// Send SIGTERM to the process group.
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
