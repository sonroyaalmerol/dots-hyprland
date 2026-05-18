package idle

import (
	"sync"
	"testing"
)

func TestScreenSaverInhibitReturnsUniqueIDs(t *testing.T) {
	ss := newScreenSaver(newBus())
	id1, _ := ss.Inhibit("firefox", "video")
	id2, _ := ss.Inhibit("vlc", "playback")
	if id1 == id2 {
		t.Errorf("ids should be unique, got %d and %d", id1, id2)
	}
}

func TestScreenSaverInhibitPublishesInhibitFalseOnLastUnInhibit(t *testing.T) {
	b := newBus()
	ss := newScreenSaver(b)

	var events []bool
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		events = append(events, active)
	})

	id1, _ := ss.Inhibit("app1", "reason1")
	_ = ss.UnInhibit(id1)

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %v", len(events), events)
	}
	if !events[0] {
		t.Error("first event should be true (inhibit)")
	}
	if events[len(events)-1] {
		t.Error("last event should be false (uninhibit)")
	}
}

func TestScreenSaverUnInhibitOnlyPublishesFalseWhenEmpty(t *testing.T) {
	b := newBus()
	ss := newScreenSaver(b)

	var inhibitEvents []bool
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		active, _ := e.Data.(bool)
		inhibitEvents = append(inhibitEvents, active)
	})

	id1, _ := ss.Inhibit("a", "r")
	id2, _ := ss.Inhibit("b", "r")

	// Remove first — should still be inhibited (id2 remains)
	_ = ss.UnInhibit(id1)

	// Remove second — now empty
	_ = ss.UnInhibit(id2)

	// Events: Inhibit(true), Inhibit(true), UnInhibit(false)
	// But UnInhibit only publishes false when the set goes empty.
	// So: true, true, false
	if len(inhibitEvents) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(inhibitEvents), inhibitEvents)
	}
	if !inhibitEvents[0] {
		t.Error("event 0 should be true")
	}
	if !inhibitEvents[1] {
		t.Error("event 1 should be true")
	}
	if inhibitEvents[2] {
		t.Error("event 2 should be false (last uninhibit)")
	}
}

func TestScreenSaverUnInhibitKeepsInhibitedWhenNotEmpty(t *testing.T) {
	b := newBus()
	ss := newScreenSaver(b)

	var eventCount int
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		eventCount++
	})

	// Add two inhibitors
	id1, _ := ss.Inhibit("a", "r")
	id2, _ := ss.Inhibit("b", "r")

	// Remove one — should NOT publish (still inhibited)
	beforeRemove := eventCount
	_ = ss.UnInhibit(id1)
	if eventCount != beforeRemove {
		t.Errorf("UnInhibit should not publish when inhibitors remain: before=%d after=%d", beforeRemove, eventCount)
	}

	// Remove last — should publish false
	_ = ss.UnInhibit(id2)
	if eventCount != beforeRemove+1 {
		t.Errorf("last UnInhibit should publish false: before=%d after=%d", beforeRemove, eventCount)
	}
}

func TestScreenSaverConcurrentInhibit(t *testing.T) {
	b := newBus()
	ss := newScreenSaver(b)

	var ids []uint32
	var mu sync.Mutex
	var wg sync.WaitGroup

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

	// All 50 IDs should be unique.
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
	b := newBus()
	ss := newScreenSaver(b)

	var inhibitEvents int
	b.subscribe(topicIdleInhibit, func(e busEvent) {
		inhibitEvents++
	})

	var ids []uint32
	for range 20 {
		id, _ := ss.Inhibit("test", "reason")
		ids = append(ids, id)
	}

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			_ = ss.UnInhibit(id)
		}(id)
	}
	wg.Wait()

	// After all uninhibits, verify the map is empty.
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
	b := newBus()
	ss := newScreenSaver(b)

	var locked bool
	b.subscribe(topicScreenLock, func(e busEvent) {
		locked, _ = e.Data.(bool)
	})

	_ = ss.Lock()
	if !locked {
		t.Error("Lock() should publish screenlock true")
	}
}

func TestScreenSaverGetActive(t *testing.T) {
	ss := newScreenSaver(newBus())
	active, err := ss.GetActive()
	if err != nil {
		t.Errorf("GetActive returned error: %v", err)
	}
	if active {
		t.Error("GetActive should return false")
	}
}

func TestScreenSaverSimulateUserActivity(t *testing.T) {
	ss := newScreenSaver(newBus())
	err := ss.SimulateUserActivity()
	if err != nil {
		t.Errorf("SimulateUserActivity returned error: %v", err)
	}
}
