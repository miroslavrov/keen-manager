package engine

import "sync"

// EventType is the SSE event name emitted to the browser.
type EventType string

const (
	// EventState tells the UI to refetch aggregate state (something changed).
	EventState EventType = "state"
	// EventLog carries a single {service, line} log frame for the Logs page.
	EventLog EventType = "log"
)

// Event is one message on the bus.
type Event struct {
	Type EventType
	Data any
}

// bus is a tiny fan-out pub/sub used to feed the /api/events SSE stream.
// Publishing never blocks: a slow subscriber simply drops frames.
type bus struct {
	mu   sync.Mutex
	subs map[int]chan Event
	next int
}

func newBus() *bus { return &bus{subs: map[int]chan Event{}} }

// subscribe registers a new subscriber and returns its id + channel.
func (b *bus) subscribe() (int, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, 64)
	b.subs[id] = ch
	return id, ch
}

func (b *bus) unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(ch)
	}
}

func (b *bus) publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default: // subscriber is behind; drop this frame rather than block
		}
	}
}

func (b *bus) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
