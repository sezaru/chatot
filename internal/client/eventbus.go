package client

import "sync"

// subscriberBufferSize is each subscriber's channel capacity. Generous on
// purpose: Publish runs synchronously inside whatsmeow's dispatch goroutine
// and must never block, so a slow/absent reader drops events rather than
// stalling the publisher.
const subscriberBufferSize = 256

// eventBus fans every published Event out to all subscribers. Each Subscribe
// returns a fresh channel, so the several UI goroutines that each range over
// Events() all see every event instead of competing for one shared channel.
// Delivery is non-blocking: a full subscriber drops the event (optionally
// logged via warn) rather than blocking the publisher or starving the others.
type eventBus struct {
	mu   sync.Mutex
	subs []chan Event
	warn func(format string, args ...interface{})
}

func newEventBus(warn func(format string, args ...interface{})) *eventBus {
	return &eventBus{warn: warn}
}

// Subscribe registers and returns a new buffered subscriber channel. The
// channel lives for the process lifetime; there is no unsubscribe.
func (b *eventBus) Subscribe() <-chan Event {
	ch := make(chan Event, subscriberBufferSize)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

// Publish broadcasts e to every subscriber, dropping (per subscriber) on a
// full channel instead of blocking.
func (b *eventBus) Publish(e Event) {
	b.mu.Lock()
	subs := b.subs
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			if b.warn != nil {
				b.warn("event subscriber full, dropping event kind=%d", e.Kind)
			}
		}
	}
}
