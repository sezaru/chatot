package ui

import "time"

// typingDebounce is how long to wait after the last keystroke before
// telling the peer we've stopped composing.
const typingDebounce = 3 * time.Second

// typingModel is the composer's pure typing-indicator state machine: it
// decides when to emit a composing/paused SendTyping call from a stream of
// keystrokes, without spamming "composing" on every character. GTK-free so
// it can be unit-tested with an injected clock; Composer drives it from the
// entry's "changed" signal plus a periodic timer.
type typingModel struct {
	timeout       time.Duration
	composing     bool
	lastKeystroke time.Time
}

func newTypingModel(timeout time.Duration) *typingModel {
	return &typingModel{timeout: timeout}
}

// Keystroke records a keystroke at now. send is true only on the first
// keystroke of a burst (i.e. when we weren't already marked composing) —
// callers should call SendTyping(jid, composing) only when send is true.
func (t *typingModel) Keystroke(now time.Time) (send, composing bool) {
	t.lastKeystroke = now
	if t.composing {
		return false, true
	}
	t.composing = true
	return true, true
}

// Tick checks whether the debounce window has elapsed since the last
// keystroke; if so it flips to paused and reports send=true. Callers should
// invoke this periodically (e.g. every second) while composing is active.
func (t *typingModel) Tick(now time.Time) (send, composing bool) {
	if t.composing && now.Sub(t.lastKeystroke) >= t.timeout {
		t.composing = false
		return true, false
	}
	return false, t.composing
}

// Cleared forces paused state, e.g. right after a send or on chat switch.
// send is true only if we were actually composing (no point telling the
// peer we stopped typing if we never told them we started).
func (t *typingModel) Cleared() (send bool) {
	if !t.composing {
		return false
	}
	t.composing = false
	return true
}
