package client

import "sync"

// qrFanout hands the current pairing QR code to every reader that asks for
// it. Pairing codes are a "latest wins" stream: a code is only scannable
// until the next one replaces it, so each subscriber gets a one-slot channel
// where a newer code displaces an unread older one, and a subscriber that
// arrives mid-round (the Relink dialog, the manager's proxy after a switch)
// is handed the code that is current right now rather than waiting up to
// twenty seconds for the next. Clear forgets the current code once the round
// is over, so nobody is handed an expired one.
type qrFanout struct {
	mu   sync.Mutex
	subs []chan string
	last string
}

// Subscribe returns a new one-slot subscriber channel, pre-loaded with the
// current code when a pairing round is in progress. Like eventBus, there is
// no unsubscribe: a subscriber lives for the process lifetime.
func (f *qrFanout) Subscribe() <-chan string {
	ch := make(chan string, 1)
	f.mu.Lock()
	f.subs = append(f.subs, ch)
	if f.last != "" {
		ch <- f.last
	}
	f.mu.Unlock()
	return ch
}

// Publish makes code the current one and delivers it to every subscriber,
// displacing an unread older code rather than dropping the new one.
func (f *qrFanout) Publish(code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = code
	for _, ch := range f.subs {
		offerLatest(ch, code)
	}
}

// Clear forgets the current code: the round ended (paired, timed out, or
// failed), so a late subscriber must wait for the next round's first code.
func (f *qrFanout) Clear() {
	f.mu.Lock()
	f.last = ""
	f.mu.Unlock()
}

// offerLatest puts code on the one-slot ch, evicting an unread value first.
// Never blocks: with a single slot and a single writer the second send
// always succeeds, and a concurrent reader only makes room.
func offerLatest(ch chan string, code string) {
	select {
	case ch <- code:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- code:
	default:
	}
}
