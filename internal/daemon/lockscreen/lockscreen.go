package lockscreen

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprland"
)

type EventType int

const (
	EventLockState   EventType = iota // data: bool (locked)
	EventAuthResult                   // data: AuthResult
	EventLockoutTick                  // data: int (remaining seconds)
)

// Config holds lockscreen parameters.
type Config struct {
	MaxAttempts     int
	LockoutDuration time.Duration
}

// DefaultConfig returns sane defaults.
func DefaultConfig() Config {
	return Config{
		MaxAttempts:     3,
		LockoutDuration: 30 * time.Second,
	}
}

// AuthResult is emitted after every Authenticate call.
type AuthResult struct {
	Success   bool   `json:"success"`
	Remaining int    `json:"remaining"`
	LockedOut bool   `json:"lockedOut"`
	Message   string `json:"message"`
}

// EventCallback receives lockscreen events.
type EventCallback func(EventType, any)

// Service manages lock/unlock state, attempt tracking, and lockout.
type Service struct {
	cfg         Config
	hl          hyprland.API
	onEvent     EventCallback
	maxAttempts int

	// shared auth state (lock-free via atomics)
	inAuth    atomic.Bool
	attempts  atomic.Int32
	lockedOut atomic.Bool
	locked    atomic.Bool

	mu sync.Mutex // serialises auth entry and lockout start
}

// New creates a lockscreen service.
func New(cfg Config, hl hyprland.API, cb EventCallback) *Service {
	return &Service{
		cfg:         cfg,
		hl:          hl,
		onEvent:     cb,
		maxAttempts: cfg.MaxAttempts,
	}
}

// Authenticate tries PAM then su to verify the password.
func (s *Service) Authenticate(password string) AuthResult {
	// Serialise entry so only one auth runs at a time.
	s.mu.Lock()
	if s.inAuth.Load() || s.lockedOut.Load() {
		s.mu.Unlock()
		return AuthResult{LockedOut: s.lockedOut.Load(), Remaining: s.remaining()}
	}
	s.inAuth.Store(true)
	s.mu.Unlock()

	defer s.inAuth.Store(false)

	err := authenticate(password)
	if err == nil {
		// Success — reset everything.
		s.attempts.Store(0)
		s.lockedOut.Store(false)
		s.locked.Store(false)

		go unlockKeyring(password)
		go s.cleanupSpecialWorkspaces()

		s.emit(EventLockState, false)
		result := AuthResult{Success: true, Remaining: s.maxAttempts}
		s.emit(EventAuthResult, result)
		return result
	}

	// Failure.
	attempts := s.attempts.Add(1)
	remaining := s.maxAttempts - int(attempts)

	if int(attempts) >= s.maxAttempts {
		s.startLockout()
		result := AuthResult{
			Success:   false,
			Remaining: 0,
			LockedOut: true,
			Message:   "Too many attempts. Locked out.",
		}
		s.emit(EventAuthResult, result)
		return result
	}

	result := AuthResult{
		Success:   false,
		Remaining: remaining,
		Message:   fmt.Sprintf("Incorrect password. %d attempt(s) remaining.", remaining),
	}
	s.emit(EventAuthResult, result)
	return result
}

func (s *Service) remaining() int {
	r := s.maxAttempts - int(s.attempts.Load())
	if r < 0 {
		return 0
	}
	return r
}

// startLockout fires a goroutine that emits EventLockoutTick every second.
func (s *Service) startLockout() {
	s.lockedOut.Store(true)
	deadline := time.Now().Add(s.cfg.LockoutDuration)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				s.lockedOut.Store(false)
				s.attempts.Store(0)
				s.emit(EventLockoutTick, 0)
				s.emit(EventAuthResult, AuthResult{Remaining: s.maxAttempts, Message: ""})
				return
			}
			s.emit(EventLockoutTick, int(remaining.Seconds())+1)
		}
	}()
}

// Lock sets the locked state and notifies listeners.
// It also ensures Hyprland allows session lock restore (survives compositor crashes).
func (s *Service) Lock() {
	// Allow session lock restore so QS can re-lock after Hyprland crash recovery.
	go func() {
		s.hl.SetOption("misc.allow_session_lock_restore", "true")
	}()

	s.locked.Store(true)
	s.emit(EventLockState, true)
}

// LockWithAutoUnlock tries to auto-unlock if the keyring is already
// accessible (e.g. unlocked by PAM during login). Returns true if
// auto-unlock succeeded (screen was NOT locked).
func (s *Service) LockWithAutoUnlock() bool {
	if IsKeyringUnlocked() {
		log.Printf("[LOCKSCREEN] keyring already unlocked, auto-unlocking on startup")
		s.attempts.Store(0)
		s.lockedOut.Store(false)
		s.locked.Store(false)

		go s.cleanupSpecialWorkspaces()

		s.emit(EventLockState, false)
		s.emit(EventAuthResult, AuthResult{Success: true, Remaining: s.maxAttempts})
		return true
	}
	s.Lock()
	return false
}

// Unlock clears the locked state and resets auth tracking.
func (s *Service) Unlock() {
	s.locked.Store(false)
	s.attempts.Store(0)
	s.lockedOut.Store(false)
	go s.cleanupSpecialWorkspaces()
	s.emit(EventLockState, false)
}

// cleanupSpecialWorkspaces moves any windows left on Hyprland's internal
// lock-screen workspace (ID >= 1000) back to workspace 1 and switches
// to it. This prevents stale high-ID workspaces from leaking into the
// bar/overview workspace calculations.
func (s *Service) cleanupSpecialWorkspaces() {
	time.Sleep(500 * time.Millisecond)

	data, err := s.hl.GetClients()
	if err != nil {
		log.Printf("[lockscreen] cleanup clients: %v", err)
		return
	}
	var clients []map[string]any
	if err := json.Unmarshal(data, &clients); err != nil {
		log.Printf("[lockscreen] cleanup parse: %v", err)
		return
	}

	for _, c := range clients {
		ws, _ := c["workspace"].(map[string]any)
		if ws == nil {
			continue
		}
		wsID, _ := ws["id"].(float64)
		if wsID > 1000 {
			addr, _ := c["address"].(string)
			if addr != "" {
				s.hl.MoveWindowToWorkspace("1", "address:"+addr)
			}
		}
	}
	s.hl.FocusWorkspace("1")
}

// IsLocked reports whether the screen is locked.
func (s *Service) IsLocked() bool {
	return s.locked.Load()
}

// TryAutoUnlock checks if the keyring is already unlocked (e.g. by PAM
// during login). If accessible, it auto-unlocks without requiring a password.
func (s *Service) TryAutoUnlock() bool {
	if !IsKeyringUnlocked() {
		log.Printf("[LOCKSCREEN] keyring not yet unlocked, cannot auto-unlock")
		return false
	}

	log.Printf("[LOCKSCREEN] keyring already unlocked, auto-unlocking")
	s.locked.Store(false)
	s.attempts.Store(0)
	s.lockedOut.Store(false)
	go s.cleanupSpecialWorkspaces()
	s.emit(EventLockState, false)
	s.emit(EventAuthResult, AuthResult{Success: true, Remaining: s.maxAttempts})
	return true
}

// SetLocked sets the locked state directly (no event emitted).
func (s *Service) SetLocked(locked bool) {
	s.locked.Store(locked)
}

// EmitSnapshot implements socket.SnapshotProvider so new QML clients
// receive the current lock state on connect.
func (s *Service) EmitSnapshot(emit func(map[string]any)) {
	emit(map[string]any{
		"event": "lock_state",
		"data": map[string]any{
			"locked": s.locked.Load(),
		},
	})
}

// EmitState emits the current lock state to all listeners.
// Used at startup to ensure the QML frontend is in sync.
func (s *Service) EmitState() {
	s.emit(EventLockState, s.locked.Load())
}

func (s *Service) emit(t EventType, data any) {
	if s.onEvent != nil {
		s.onEvent(t, data)
	}
}
