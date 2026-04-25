package idle

import "sync"

// busTopic identifies a message channel.
type busTopic string

const (
	topicScreenLock  busTopic = "screenlock"
	topicIdleInhibit busTopic = "idleinhibit"
)

type busEvent struct {
	Topic busTopic
	Data  any
}

type busHandler func(busEvent)

// bus is a minimal publish/subscribe replacement for snry-shell's bus.Bus.
// It uses sync.Map and simple callback slices — no external dependencies.
type bus struct {
	mu   sync.RWMutex
	subs map[busTopic][]busHandler
}

func newBus() *bus {
	return &bus{subs: make(map[busTopic][]busHandler)}
}

func (b *bus) subscribe(topic busTopic, h busHandler) {
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], h)
	b.mu.Unlock()
}

func (b *bus) publish(topic busTopic, data any) {
	ev := busEvent{Topic: topic, Data: data}
	b.mu.RLock()
	handlers := b.subs[topic]
	b.mu.RUnlock()
	for _, h := range handlers {
		h(ev)
	}
}
