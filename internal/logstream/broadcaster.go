package logstream

import (
	"log/slog"
	"sync"
	"time"
)

// Entry is a serializable snapshot of a single log record.
type Entry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   map[string]string
}

// Broadcaster fans log entries out to registered subscribers in a
// non-blocking, thread-safe manner. Slow subscribers are dropped rather than
// allowed to block the logger.
type Broadcaster struct {
	mu   sync.RWMutex
	subs map[int]chan Entry
	next int
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[int]chan Entry)}
}

// Subscribe registers a new subscriber and returns a receive-only channel
// along with an unsubscribe function that must be called when done.
func (b *Broadcaster) Subscribe(bufSize int) (<-chan Entry, func()) {
	ch := make(chan Entry, bufSize)
	b.mu.Lock()
	id := b.next
	b.next++
	b.subs[id] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
		// drain so any blocked sender can proceed
		for len(ch) > 0 {
			<-ch
		}
	}
}

// Publish sends e to every registered subscriber. Subscribers whose channels
// are full are skipped — entries are dropped rather than blocking the caller.
func (b *Broadcaster) Publish(e Entry) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// HasSubscribers reports whether any subscribers are currently registered.
// Use this to skip serialization work when no one is listening.
func (b *Broadcaster) HasSubscribers() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs) > 0
}
