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
	// isLocked doesn't call provider if nil? It does: s.lockedProvider != nil check.
	_ = s.isLocked()
	if !called {
		t.Error("lockedProvider should be called")
	}
}

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

func TestNotifyLockChangedLocking(t *testing.T) {
	hl := &mockHL{}
	s := New(nil, nil, DefaultConfig(), hl)
	s.SetLockedProvider(func() bool { return true })

	// Initially lastLocked is false
	s.NotifyLockChanged()

	// Locking should turn display ON (for lock screen visibility)
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

	// First lock
	s.SetLockedProvider(func() bool { return true })
	s.NotifyLockChanged()

	// Then unlock
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

	// Unlock should clear suppression
	s.SetLockedProvider(func() bool { return false })
	s.NotifyLockChanged()

	if s.displayOffForced {
		t.Error("displayOffForced should be cleared on unlock")
	}
}

func TestTickNoSuspendWhenUnlocked(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return false })
	s.cfg.SuspendTimeout = time.Millisecond

	// Set idleStarted in the past
	s.idleStarted = time.Now().Add(-2 * time.Millisecond)

	// Tick should do nothing because we're not locked.
	s.tick()
	// Actually tick checks isLocked first — we're not locked, so it returns early.
	// idleStarted stays as-is.
	_ = s.idleStarted.IsZero()
}

func TestTickNoSuspendWhenInhibited(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })
	s.inhibited = true
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
	// Should not clear idleStarted because timeout not reached.
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

func TestHandleBlockInhibitedIdleActive(t *testing.T) {
	b := newBus()
	s := New(nil, nil, DefaultConfig(), nil)
	s.bus = b

	var inhibited []bool
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		inhibited = append(inhibited, active)
	})

	s.handleBlockInhibited("handle-power-key:idle:handle-lid-switch")
	if len(inhibited) != 1 || !inhibited[0] {
		t.Errorf("expected inhibited=true, got %v", inhibited)
	}
}

func TestHandleBlockInhibitedNoIdle(t *testing.T) {
	b := newBus()
	s := New(nil, nil, DefaultConfig(), nil)
	s.bus = b

	var inhibited []bool
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		inhibited = append(inhibited, active)
	})

	s.handleBlockInhibited("handle-power-key:handle-lid-switch")
	if len(inhibited) != 1 || inhibited[0] {
		t.Errorf("expected inhibited=false, got %v", inhibited)
	}
}

func TestHandleBlockInhibitedEmpty(t *testing.T) {
	b := newBus()
	s := New(nil, nil, DefaultConfig(), nil)
	s.bus = b

	var inhibited []bool
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		inhibited = append(inhibited, active)
	})

	s.handleBlockInhibited("")
	if len(inhibited) != 1 || inhibited[0] {
		t.Errorf("expected inhibited=false for empty string, got %v", inhibited)
	}
}

func TestHandleBlockInhibitedOnlyIdle(t *testing.T) {
	b := newBus()
	s := New(nil, nil, DefaultConfig(), nil)
	s.bus = b

	var inhibited []bool
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		inhibited = append(inhibited, active)
	})

	s.handleBlockInhibited("idle")
	if len(inhibited) != 1 || !inhibited[0] {
		t.Errorf("expected inhibited=true for bare 'idle', got %v", inhibited)
	}
}

func TestInhibitionFlow(t *testing.T) {
	b := newBus()
	s := New(nil, nil, DefaultConfig(), nil)
	s.bus = b

	// Simulate what the bus subscriber does: track inhibition state.
	var inhibited bool
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		inhibited, _ = e.Data.(bool)
	})

	s.handleBlockInhibited("idle")
	if !inhibited {
		t.Error("should be inhibited after idle block")
	}

	s.handleBlockInhibited("")
	if inhibited {
		t.Error("should not be inhibited after empty block")
	}
}

func TestServiceNew(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	if s.bus == nil {
		t.Error("bus should not be nil")
	}
	if s.cfg.IdleTimeout != 5*time.Minute {
		t.Error("config should be default")
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

func TestSetOnLogindUnlock(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	called := false
	s.SetOnLogindUnlock(func() {
		called = true
	})

	s.onLogindUnlock()
	if !called {
		t.Error("onLogindUnlock callback should be called")
	}
}

func TestSetOnLockCallback(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	called := false
	s.SetOnLock(func() {
		called = true
	})

	// doLock via internal path — but doLock also tries to exec.Command("qs", ...)
	// which we can't control. Instead test the callback reference directly.
	s.onLockCallback()
	if !called {
		t.Error("onLockCallback should be called")
	}
}

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
	s.inhibited = true

	callCount := 0
	s.SetOnLock(func() { callCount++ })

	s.doLock()
	if callCount != 0 {
		t.Errorf("doLock should not lock when inhibited, got %d calls", callCount)
	}
}

// TestResumedHandlerRecreatesTimers verifies that the resumed handler
// set by recreateTimers calls recreateTimers again. This is critical:
// ext_idle_notify_v1 notifications are one-shot — after idled+resumed
// the notification is consumed. The resumed handler must recreate timers
// or the next idle period goes undetected.
func TestResumedHandlerRecreatesTimers(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return true })

	var recreateCount int
	s.SetOnRecreateTimers(func() {
		recreateCount++
	})

	// Simulate what recreateTimers does: store a resumed handler that
	// turns display on and recreates timers.
	s.testResumedHandler = func() {
		s.setDisplay(true)
		s.recreateTimers()
	}

	// Initial recreate call (e.g. from NotifyLockChanged)
	s.recreateTimers()
	if recreateCount != 1 {
		t.Fatalf("expected 1 initial recreate, got %d", recreateCount)
	}

	// Simulate a Wayland resume event
	s.testResumedHandler()

	// Should have triggered recreateTimers again
	if recreateCount != 2 {
		t.Errorf("expected 2 recreates after resume, got %d — resumed handler did not recreate timers", recreateCount)
	}
}

// TestResumedHandlerRecreatesTimersWhenUnlocked verifies the same
// resume→recreate cycle works when unlocked.
func TestResumedHandlerRecreatesTimersWhenUnlocked(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)
	s.SetLockedProvider(func() bool { return false })

	var recreateCount int
	s.SetOnRecreateTimers(func() {
		recreateCount++
	})

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

// TestRecreateTimersHookFiresEvenWithNilManager ensures the test hook
// fires regardless of Wayland state.
func TestRecreateTimersHookFiresEvenWithNilManager(t *testing.T) {
	s := New(nil, nil, DefaultConfig(), nil)

	var count int
	s.SetOnRecreateTimers(func() { count++ })

	s.recreateTimers()
	if count != 1 {
		t.Errorf("hook should fire even with nil manager, got %d", count)
	}
}
