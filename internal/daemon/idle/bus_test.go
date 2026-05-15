package idle

import (
	"sync"
	"testing"
)

func TestBusSubscribeAndPublish(t *testing.T) {
	b := newBus()
	var received []busEvent
	var mu sync.Mutex

	b.subscribe("test", func(e busEvent) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	b.publish("test", "hello")
	b.publish("test", 42)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Data != "hello" {
		t.Errorf("data[0] = %v, want hello", received[0].Data)
	}
	if received[1].Data != 42 {
		t.Errorf("data[1] = %v, want 42", received[1].Data)
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	b := newBus()
	var wg sync.WaitGroup
	counts := make([]int, 3)

	for i := range 3 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b.subscribe("shared", func(e busEvent) {
				counts[idx]++
			})
		}(i)
	}
	wg.Wait()

	b.publish("shared", nil)
	b.publish("shared", nil)

	for i, c := range counts {
		if c != 2 {
			t.Errorf("subscriber %d got %d events, want 2", i, c)
		}
	}
}

func TestBusDifferentTopics(t *testing.T) {
	b := newBus()
	var aCount, bCount int

	b.subscribe("topicA", func(e busEvent) { aCount++ })
	b.subscribe("topicB", func(e busEvent) { bCount++ })

	b.publish("topicA", nil)
	b.publish("topicA", nil)
	b.publish("topicB", nil)

	if aCount != 2 {
		t.Errorf("topicA = %d, want 2", aCount)
	}
	if bCount != 1 {
		t.Errorf("topicB = %d, want 1", bCount)
	}
}

func TestBusNoSubscribers(t *testing.T) {
	b := newBus()
	// Should not panic when no handlers are registered.
	b.publish("none", nil)
}

func TestBusTopicConstants(t *testing.T) {
	if topicScreenLock != "screenlock" {
		t.Errorf("topicScreenLock = %q, want screenlock", topicScreenLock)
	}
	if topicIdleInhibit != "idleinhibit" {
		t.Errorf("topicIdleInhibit = %q, want idleinhibit", topicIdleInhibit)
	}
}

func TestBusConcurrentPublish(t *testing.T) {
	b := newBus()
	var count int
	var mu sync.Mutex

	b.subscribe("t", func(e busEvent) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				b.publish("t", nil)
			}
		})
	}
	wg.Wait()

	if count != 1000 {
		t.Errorf("count = %d, want 1000", count)
	}
}

func TestBusConcurrentSubscribe(t *testing.T) {
	b := newBus()
	var wg sync.WaitGroup

	// Subscribe while publishing simultaneously.
	for range 5 {
		wg.Go(func() {
			for range 50 {
				b.publish("t", nil)
			}
		})
	}
	for range 5 {
		wg.Go(func() {
			for range 50 {
				b.subscribe("t", func(e busEvent) {})
			}
		})
	}
	wg.Wait()
}

func TestBusHandlerPanicRecovery(t *testing.T) {
	b := newBus()

	var secondCalled bool
	b.subscribe("t", func(e busEvent) {
		panic("boom")
	})
	b.subscribe("t", func(e busEvent) {
		secondCalled = true
	})

	// Should not panic; second handler should still fire.
	b.publish("t", "data")
	if !secondCalled {
		t.Error("second handler was not called after first panicked")
	}
}

func TestBusHandlerPanicRecoveryDataIntegrity(t *testing.T) {
	b := newBus()

	var received []int
	b.subscribe("t", func(e busEvent) {
		panic("first handler always panics")
	})
	b.subscribe("t", func(e busEvent) {
		n, ok := e.Data.(int)
		if ok {
			received = append(received, n)
		}
	})
	b.subscribe("t", func(e busEvent) {
		panic("third handler also panics")
	})
	b.subscribe("t", func(e busEvent) {
		n, ok := e.Data.(int)
		if ok {
			received = append(received, n)
		}
	})

	b.publish("t", 1)
	b.publish("t", 2)

	if len(received) != 4 {
		t.Fatalf("expected 4 events delivered, got %d", len(received))
	}
	for i, v := range received {
		expected := (i / 2) + 1
		if v != expected {
			t.Errorf("received[%d] = %d, want %d", i, v, expected)
		}
	}
}
