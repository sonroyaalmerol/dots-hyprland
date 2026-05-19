package idle

import (
	"sync"
	"testing"
	"time"
)

// mockHL implements hyprland.API for testing.
type mockHL struct {
	mu        sync.Mutex
	dpmsCalls []string
}

func (m *mockHL) SetDPMS(action string) error {
	m.mu.Lock()
	m.dpmsCalls = append(m.dpmsCalls, action)
	m.mu.Unlock()
	return nil
}

func (m *mockHL) dpmsLog() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.dpmsCalls))
	copy(out, m.dpmsCalls)
	return out
}

// stub methods
func (m *mockHL) FocusWorkspace(string) error                     { return nil }
func (m *mockHL) ToggleSpecialWorkspace(string) error             { return nil }
func (m *mockHL) ActiveWorkspaceID() (int, error)                 { return 0, nil }
func (m *mockHL) FocusWindow(string) error                        { return nil }
func (m *mockHL) CloseWindow(string) error                        { return nil }
func (m *mockHL) MoveWindowToWorkspace(string, string) error      { return nil }
func (m *mockHL) MoveWindowToCoords(string, string, string) error { return nil }
func (m *mockHL) FocusMonitor(string) error                       { return nil }
func (m *mockHL) SetMonitor(string, string, string, float64) error {
	return nil
}
func (m *mockHL) SetOption(string, string) error   { return nil }
func (m *mockHL) ResetOption(string) error         { return nil }
func (m *mockHL) GetOption(string) ([]byte, error) { return nil, nil }
func (m *mockHL) SetAnimation(string, bool, float64, string, string) error {
	return nil
}
func (m *mockHL) Reload() error                       { return nil }
func (m *mockHL) ExecCommand(string) error            { return nil }
func (m *mockHL) ActivateGlobalShortcut(string) error { return nil }
func (m *mockHL) BindKey(string, string, bool) error  { return nil }
func (m *mockHL) UnbindKey(string) error              { return nil }
func (m *mockHL) GetMonitors() ([]byte, error)        { return nil, nil }
func (m *mockHL) GetClients() ([]byte, error)         { return nil, nil }
func (m *mockHL) GetDevices() ([]byte, error)         { return nil, nil }
func (m *mockHL) IsRunning() bool                     { return true }

// ── Helper: set a source flag directly and recompute ────────────────────────

// setInhibitedForTest sets screensaverInhibited and recomputes.
func (s *Service) setInhibitedForTest(inhibited bool) {
	s.mu.Lock()
	s.screensaverInhibited = inhibited
	s.mu.Unlock()
	s.recomputeInhibition()
}

// ── Config tests ────────────────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("IdleTimeout = %v, want 5m", cfg.IdleTimeout)
	}
	if cfg.LockDelay != 3*time.Second {
		t.Errorf("LockDelay = %v, want 3s", cfg.LockDelay)
	}
	if cfg.LockDisplayOffTimeout != 3*time.Second {
		t.Errorf("LockDisplayOffTimeout = %v, want 3s", cfg.LockDisplayOffTimeout)
	}
	if cfg.SuspendTimeout != 0 {
		t.Errorf("SuspendTimeout = %v, want 0", cfg.SuspendTimeout)
	}
}

// ── isLocked tests ──────────────────────────────────────────────────────────

func TestIsLockedNoProvider(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	if s.isLocked() {
		t.Error("isLocked should return false with no provider")
	}
}

func TestIsLockedWithProvider(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })
	if !s.isLocked() {
		t.Error("isLocked should return true when provider says true")
	}

	s.SetLockedProvider(func() bool { return false })
	if s.isLocked() {
		t.Error("isLocked should return false when provider says false")
	}
}

func TestSetLockedProvider(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	called := false
	s.SetLockedProvider(func() bool {
		called = true
		return true
	})
	_ = s.isLocked()
	if !called {
		t.Error("lockedProvider should be called")
	}
}

// ── Display tests ───────────────────────────────────────────────────────────

func TestSuppressDisplayOn(t *testing.T) {
	hl := &mockHL{}
	s := New(nil, nil, DefaultConfig(), hl)

	// Turn display off (allowed)
	s.setDisplay(false)
	if log := hl.dpmsLog(); len(log) != 1 || log[0] != "off" {
		t.Errorf("expected 1 off call, got %v", log)
	}

	// Suppress display on
	s.SuppressDisplayOn(true)
	s.setDisplay(true)
	// Should be suppressed — still just 1 call (the off from above)
	if log := hl.dpmsLog(); len(log) != 1 {
		t.Errorf("display on should be suppressed, got %v", log)
	}

	// Unsuppress
	s.SuppressDisplayOn(false)
	s.setDisplay(true)
	if log := hl.dpmsLog(); len(log) != 2 || log[1] != "on" {
		t.Errorf("display on should work after unsuppress, got %v", log)
	}
}

func TestSetDisplayNilHyprland(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	// Should not panic.
	s.setDisplay(true)
	s.setDisplay(false)
}

func TestDisplayOnStartsTrue(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	if !s.displayOn.Load() {
		t.Error("displayOn should be true at startup")
	}
}

func TestDisplayOnTracksSetDisplay(t *testing.T) {
	hl := &mockHL{}
	s := New(nil, nil, DefaultConfig(), hl)

	s.setDisplay(false)
	if s.displayOn.Load() {
		t.Error("displayOn should be false after setDisplay(false)")
	}

	s.setDisplay(true)
	if !s.displayOn.Load() {
		t.Error("displayOn should be true after setDisplay(true)")
	}
}

func TestSetOnDisplayChange(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	var calls []bool
	s.SetOnDisplayChange(func(on bool) {
		calls = append(calls, on)
	})

	s.setDisplay(false)
	s.setDisplay(true)

	if len(calls) != 2 || calls[0] || !calls[1] {
		t.Errorf("expected [false true], got %v", calls)
	}
}

// ── Lock change tests ───────────────────────────────────────────────────────

func TestNotifyLockChangedLocking(t *testing.T) {
	hl := &mockHL{}
	s := New(nil, nil, DefaultConfig(), hl)
	s.SetLockedProvider(func() bool { return true })

	s.NotifyLockChanged()

	log := hl.dpmsLog()
	if len(log) < 1 || log[0] != "on" {
		t.Errorf("lock should turn display ON, got %v", log)
	}
	if s.lastLocked != true {
		t.Error("lastLocked should be true after NotifyLockChanged")
	}
}

func TestNotifyLockChangedUnlocking(t *testing.T) {
	hl := &mockHL{}
	s := New(nil, nil, DefaultConfig(), hl)

	s.SetLockedProvider(func() bool { return true })
	s.NotifyLockChanged()

	s.SetLockedProvider(func() bool { return false })
	s.NotifyLockChanged()

	log := hl.dpmsLog()
	if len(log) < 2 || log[1] != "on" {
		t.Errorf("unlock should turn display ON, got %v", log)
	}
	if s.lastLocked != false {
		t.Error("lastLocked should be false after unlock")
	}
}

func TestNotifyLockChangedClearsDisplayOffForced(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SuppressDisplayOn(true)

	s.SetLockedProvider(func() bool { return false })
	s.NotifyLockChanged()

	if s.displayOffForced {
		t.Error("displayOffForced should be cleared on unlock")
	}
}

// ── Callback tests ──────────────────────────────────────────────────────────

func TestSetOnLogindUnlock(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	called := false
	s.SetOnLogindUnlock(func() { called = true })
	s.onLogindUnlock()
	if !called {
		t.Error("onLogindUnlock callback should be called")
	}
}

func TestSetOnLockCallback(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	called := false
	s.SetOnLock(func() { called = true })
	s.onLockCallback()
	if !called {
		t.Error("onLockCallback should be called")
	}
}

// ── doLock tests ────────────────────────────────────────────────────────────

func TestLockDoesNotDoubleLock(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })

	callCount := 0
	s.SetOnLock(func() { callCount++ })

	s.doLock()
	if callCount != 0 {
		t.Errorf("doLock should not lock when already locked, got %d calls", callCount)
	}
}

func TestLockSkippedWhenInhibited(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.setInhibitedForTest(true)

	callCount := 0
	s.SetOnLock(func() { callCount++ })

	s.doLock()
	if callCount != 0 {
		t.Errorf("doLock should not lock when inhibited, got %d calls", callCount)
	}
}

// ── tick tests ──────────────────────────────────────────────────────────────

func TestTickNoSuspendWhenUnlocked(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return false })
	s.cfg.SuspendTimeout = time.Millisecond
	s.idleStarted = time.Now().Add(-2 * time.Millisecond)

	s.tick()
	// tick checks isLocked first — not locked, returns early.
	_ = s.idleStarted.IsZero()
}

func TestTickNoSuspendWhenInhibited(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })
	s.setInhibitedForTest(true)
	s.cfg.SuspendTimeout = time.Millisecond
	s.idleStarted = time.Now().Add(-2 * time.Millisecond)

	s.tick()
	// Should not clear idleStarted because inhibited check happens first.
}

func TestTickNoSuspendWhenNotTimedOut(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })
	s.cfg.SuspendTimeout = time.Hour
	s.idleStarted = time.Now()

	s.tick()
	if s.idleStarted.IsZero() {
		t.Error("idleStarted should not be cleared before timeout")
	}
}

func TestTickClearsIdleStartedOnSuspend(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })
	s.cfg.SuspendTimeout = time.Nanosecond
	s.idleStarted = time.Now().Add(-time.Millisecond)

	s.tick()
	if !s.idleStarted.IsZero() {
		t.Error("idleStarted should be cleared after suspend triggers")
	}
}

// ── Inhibition source tests ─────────────────────────────────────────────────

func TestHandleBlockInhibitedIdleActive(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.handleBlockInhibited("handle-power-key:idle:handle-lid-switch")
	if !s.inhibited.Load() {
		t.Error("expected inhibited=true when :idle: in BlockInhibited")
	}
}

func TestHandleBlockInhibitedNoIdle(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.handleBlockInhibited("handle-power-key:handle-lid-switch")
	if s.inhibited.Load() {
		t.Error("expected inhibited=false when :idle: not in BlockInhibited")
	}
}

func TestHandleBlockInhibitedEmpty(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.handleBlockInhibited("")
	if s.inhibited.Load() {
		t.Error("expected inhibited=false for empty string")
	}
}

func TestHandleBlockInhibitedOnlyIdle(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.handleBlockInhibited("idle")
	if !s.inhibited.Load() {
		t.Error("expected inhibited=true for bare 'idle'")
	}
}

func TestInhibitionFlow(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)

	s.handleBlockInhibited("idle")
	if !s.inhibited.Load() {
		t.Error("should be inhibited after idle block")
	}

	s.handleBlockInhibited("")
	if s.inhibited.Load() {
		t.Error("should not be inhibited after empty block")
	}
}

// TestInhibitionSourcesDontOverride verifies that multiple inhibition sources
// are OR-ed together and one source clearing its flag does not clobber another.
func TestInhibitionSourcesDontOverride(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)

	// ScreenSaver inhibits
	s.mu.Lock()
	s.screensaverInhibited = true
	s.mu.Unlock()
	s.recomputeInhibition()
	if !s.inhibited.Load() {
		t.Fatal("should be inhibited after screensaver inhibit")
	}

	// logind BlockInhibited changes to something without :idle: — should NOT clear inhibition
	s.handleBlockInhibited("handle-power-key")
	if !s.inhibited.Load() {
		t.Error("inhibition should persist — screensaver is still active")
	}

	// ScreenSaver uninhibits — now inhibition should clear
	s.mu.Lock()
	s.screensaverInhibited = false
	s.mu.Unlock()
	s.recomputeInhibition()
	if s.inhibited.Load() {
		t.Error("inhibition should clear when all sources are gone")
	}
}

// TestFullscreenInhibition verifies fullscreen count drives inhibition.
func TestFullscreenInhibition(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)

	// Window goes fullscreen
	s.mu.Lock()
	s.fullscreenCount++
	s.mu.Unlock()
	s.recomputeInhibition()
	if !s.inhibited.Load() {
		t.Error("should be inhibited when fullscreen window exists")
	}

	// Another window goes fullscreen
	s.mu.Lock()
	s.fullscreenCount++
	s.mu.Unlock()
	s.recomputeInhibition()
	if !s.inhibited.Load() {
		t.Error("should still be inhibited with 2 fullscreen windows")
	}

	// One exits fullscreen
	s.mu.Lock()
	s.fullscreenCount--
	s.mu.Unlock()
	s.recomputeInhibition()
	if !s.inhibited.Load() {
		t.Error("should still be inhibited with 1 fullscreen window")
	}

	// Last exits fullscreen
	s.mu.Lock()
	s.fullscreenCount--
	s.mu.Unlock()
	s.recomputeInhibition()
	if s.inhibited.Load() {
		t.Error("should not be inhibited when no fullscreen windows")
	}
}

// TestInhibitionTurnsDisplayOn verifies that activating inhibition while
// display is off turns the display back on (unlocked screen only).
func TestInhibitionTurnsDisplayOn(t *testing.T) {
	hl := &mockHL{}
	s := New(nil, nil, DefaultConfig(), hl)
	s.SetLockedProvider(func() bool { return false })

	// Turn display off
	s.setDisplay(false)
	if log := hl.dpmsLog(); len(log) != 1 || log[0] != "off" {
		t.Fatalf("expected display off, got %v", log)
	}

	// ScreenSaver inhibits — should turn display back on
	s.mu.Lock()
	s.screensaverInhibited = true
	s.mu.Unlock()
	s.recomputeInhibition()

	log := hl.dpmsLog()
	if len(log) < 2 || log[1] != "on" {
		t.Errorf("inhibition should turn display ON, got %v", log)
	}
}

// TestInhibitionDoesNotTurnDisplayOnWhenLocked verifies that inhibition
// activating while the screen is locked does NOT turn the display on.
func TestInhibitionDoesNotTurnDisplayOnWhenLocked(t *testing.T) {
	hl := &mockHL{}
	s := New(nil, nil, DefaultConfig(), hl)
	s.SetLockedProvider(func() bool { return true })

	// Simulate locked state with display off
	s.mu.Lock()
	s.lastLocked = true
	s.displayOn.Store(false)
	s.mu.Unlock()
	s.setDisplay(false)

	callsBefore := len(hl.dpmsLog())

	// ScreenSaver inhibits while locked — should NOT turn display on
	s.mu.Lock()
	s.screensaverInhibited = true
	s.mu.Unlock()
	s.recomputeInhibition()

	if len(hl.dpmsLog()) > callsBefore {
		t.Errorf("inhibition should NOT turn display on when locked, got %v", hl.dpmsLog())
	}
}

// ── Service constructor test ────────────────────────────────────────────────

func TestServiceNew(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	if s.bus == nil {
		t.Error("bus should not be nil")
	}
	if s.cfg.IdleTimeout != 5*time.Minute {
		t.Error("config should be default")
	}
}

// ── Wayland timer recreation tests ──────────────────────────────────────────

func TestResumedHandlerRecreatesTimers(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })

	var recreateCount int
	s.SetOnRecreateTimers(func() { recreateCount++ })

	s.testResumedHandler = func() {
		s.setDisplay(true)
		s.recreateTimers()
	}

	s.recreateTimers()
	if recreateCount != 1 {
		t.Fatalf("expected 1 initial recreate, got %d", recreateCount)
	}

	s.testResumedHandler()
	if recreateCount != 2 {
		t.Errorf("expected 2 recreates after resume, got %d", recreateCount)
	}
}

func TestResumedHandlerRecreatesTimersWhenUnlocked(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return false })

	var recreateCount int
	s.SetOnRecreateTimers(func() { recreateCount++ })

	s.testResumedHandler = func() {
		s.setDisplay(true)
		s.recreateTimers()
	}

	s.recreateTimers()
	if recreateCount != 1 {
		t.Fatalf("expected 1 initial recreate, got %d", recreateCount)
	}

	s.testResumedHandler()
	if recreateCount != 2 {
		t.Errorf("expected 2 recreates after resume (unlocked), got %d", recreateCount)
	}
}

func TestRecreateTimersHookFiresEvenWithNilManager(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	var count int
	s.SetOnRecreateTimers(func() { count++ })
	s.recreateTimers()
	if count != 1 {
		t.Errorf("hook should fire even with nil manager, got %d", count)
	}
}
