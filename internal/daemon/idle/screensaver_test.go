package idle

import (
	"sync"
	"testing"
)

func TestScreenSaverInhibitReturnsUniqueIDs(t *testing.T) {
	ss := newScreenSaver(nil)
	id1, _ := ss.Inhibit("firefox", "video")
	id2, _ := ss.Inhibit("vlc", "playback")
	if id1 == id2 {
		t.Errorf("ids should be unique, got %d and %d", id1, id2)
	}
}

func TestScreenSaverInhibitCallback(t *testing.T) {
	var mu sync.Mutex
	var calls []bool
	ss := newScreenSaver(func(inhibited bool) {
		mu.Lock()
		calls = append(calls, inhibited)
		mu.Unlock()
	})

	ss.Inhibit("app1", "reason1")

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || !calls[0] {
		t.Errorf("expected [true], got %v", calls)
	}
}

func TestScreenSaverUnInhibitCallbackOnLastRemove(t *testing.T) {
	var mu sync.Mutex
	var calls []bool
	ss := newScreenSaver(func(inhibited bool) {
		mu.Lock()
		calls = append(calls, inhibited)
		mu.Unlock()
	})

	id1, _ := ss.Inhibit("app1", "reason1")
	_, _ = ss.Inhibit("app2", "reason2")
	_ = ss.UnInhibit(id1)

	mu.Lock()
	got := len(calls)
	mu.Unlock()
	if got != 2 {
		t.Fatalf("expected 2 calls after partial uninhibit, got %d", got)
	}

	// Remove second inhibitor
	ss.inhibitors.Range(func(k, _ any) bool {
		ss.UnInhibit(k.(uint32))
		return false // remove first remaining
	})

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(calls), calls)
	}
	if calls[2] {
		t.Error("last callback should be false (uninhibit)")
	}
}

func TestScreenSaverUnInhibitOnlyCallbackWhenEmpty(t *testing.T) {
	var mu sync.Mutex
	var calls []bool
	ss := newScreenSaver(func(inhibited bool) {
		mu.Lock()
		calls = append(calls, inhibited)
		mu.Unlock()
	})

	id1, _ := ss.Inhibit("a", "r")
	id2, _ := ss.Inhibit("b", "r")

	// Remove first — should NOT call back (id2 remains)
	_ = ss.UnInhibit(id1)
	mu.Lock()
	before := len(calls)
	mu.Unlock()
	if before != 2 {
		t.Errorf("expected 2 calls after partial uninhibit, got %d", before)
	}

	// Remove last — should callback false
	_ = ss.UnInhibit(id2)
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(calls), calls)
	}
	if calls[2] {
		t.Error("last callback should be false")
	}
}

func TestScreenSaverConcurrentInhibit(t *testing.T) {
	var mu sync.Mutex
	var ids []uint32
	var wg sync.WaitGroup

	ss := newScreenSaver(func(bool) {})

	for range 50 {
		wg.Go(func() {
			id, err := ss.Inhibit("test", "concurrent")
			if err != nil {
				t.Errorf("Inhibit failed: %v", err)
			}
			mu.Lock()
			ids = append(ids, id)
			mu.Unlock()
		})
	}
	wg.Wait()

	seen := make(map[uint32]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID: %d", id)
		}
		seen[id] = true
	}
	if len(seen) != 50 {
		t.Errorf("expected 50 unique IDs, got %d", len(seen))
	}
}

func TestScreenSaverConcurrentUnInhibit(t *testing.T) {
	ss := newScreenSaver(func(bool) {})

	var ids []uint32
	for range 20 {
		id, _ := ss.Inhibit("test", "reason")
		ids = append(ids, id)
	}

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			_ = ss.UnInhibit(id)
		})
	}
	wg.Wait()

	remaining := 0
	ss.inhibitors.Range(func(_, _ any) bool {
		remaining++
		return true
	})
	if remaining != 0 {
		t.Errorf("expected 0 remaining inhibitors, got %d", remaining)
	}
}

func TestScreenSaverLock(t *testing.T) {
	ss := newScreenSaver(nil)
	// Lock is now a no-op, should not panic.
	_ = ss.Lock()
}

func TestScreenSaverGetActive(t *testing.T) {
	ss := newScreenSaver(nil)
	active, err := ss.GetActive()
	if err != nil {
		t.Errorf("GetActive returned error: %v", err)
	}
	if active {
		t.Error("GetActive should return false")
	}
}

func TestScreenSaverSimulateUserActivity(t *testing.T) {
	ss := newScreenSaver(nil)
	err := ss.SimulateUserActivity()
	if err != nil {
		t.Errorf("SimulateUserActivity returned error: %v", err)
	}
}
