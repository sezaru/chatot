package ui

import (
	"testing"
	"time"
)

func TestPresenceSubtitle(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		state PresenceState
		want  string
	}{
		{"empty", PresenceState{}, ""},
		{"online", PresenceState{Online: true}, "online"},
		{"typing beats online", PresenceState{Online: true, Typing: true}, "typing…"},
		{"typing beats last seen", PresenceState{Typing: true, LastSeen: now.Add(-time.Hour)}, "typing…"},
		{"last seen just now", PresenceState{LastSeen: now.Add(-30 * time.Second)}, "last seen just now"},
		{"last seen minutes", PresenceState{LastSeen: now.Add(-5 * time.Minute)}, "last seen 5m ago"},
		{"last seen hours", PresenceState{LastSeen: now.Add(-3 * time.Hour)}, "last seen 3h ago"},
		{"last seen days", PresenceState{LastSeen: now.Add(-49 * time.Hour)}, "last seen 2d ago"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := presenceSubtitle(tc.state, now)
			if got != tc.want {
				t.Errorf("presenceSubtitle(%+v) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}

func TestTypingModelKeystroke(t *testing.T) {
	tm := newTypingModel(3 * time.Second)
	t0 := time.Unix(1000, 0)

	send, composing := tm.Keystroke(t0)
	if !send || !composing {
		t.Fatalf("first keystroke: send=%v composing=%v, want true,true", send, composing)
	}

	// A burst of further keystrokes within the window must not re-send.
	send, composing = tm.Keystroke(t0.Add(500 * time.Millisecond))
	if send {
		t.Fatalf("keystroke mid-burst: send=%v, want false", send)
	}
	if !composing {
		t.Fatalf("keystroke mid-burst: composing=%v, want true", composing)
	}
}

func TestTypingModelTickTimeout(t *testing.T) {
	tm := newTypingModel(3 * time.Second)
	t0 := time.Unix(2000, 0)
	tm.Keystroke(t0)

	// Before the window elapses, Tick should not fire.
	if send, composing := tm.Tick(t0.Add(2 * time.Second)); send || !composing {
		t.Fatalf("tick before timeout: send=%v composing=%v, want false,true", send, composing)
	}

	// Once the window elapses with no further keystroke, Tick sends paused.
	send, composing := tm.Tick(t0.Add(3 * time.Second))
	if !send || composing {
		t.Fatalf("tick after timeout: send=%v composing=%v, want true,false", send, composing)
	}

	// A subsequent tick is a no-op: we already reported paused once.
	if send, _ := tm.Tick(t0.Add(10 * time.Second)); send {
		t.Fatalf("tick after already-paused: send=%v, want false", send)
	}
}

func TestTypingModelKeystrokeResetsTimeout(t *testing.T) {
	tm := newTypingModel(3 * time.Second)
	t0 := time.Unix(3000, 0)
	tm.Keystroke(t0)

	// A fresh keystroke just before the deadline should push it out again.
	tm.Keystroke(t0.Add(2 * time.Second))
	if send, composing := tm.Tick(t0.Add(4 * time.Second)); send || !composing {
		t.Fatalf("tick after refreshed keystroke: send=%v composing=%v, want false,true (window pushed to t0+5s)", send, composing)
	}
	if send, composing := tm.Tick(t0.Add(5 * time.Second)); !send || composing {
		t.Fatalf("tick at refreshed deadline: send=%v composing=%v, want true,false", send, composing)
	}
}

func TestTypingModelCleared(t *testing.T) {
	tm := newTypingModel(3 * time.Second)

	// Clearing while not composing is a no-op.
	if send := tm.Cleared(); send {
		t.Fatalf("Cleared while idle: send=%v, want false", send)
	}

	tm.Keystroke(time.Unix(4000, 0))
	if send := tm.Cleared(); !send {
		t.Fatalf("Cleared while composing: send=%v, want true", send)
	}

	// A second Cleared right after is a no-op: we already told the peer.
	if send := tm.Cleared(); send {
		t.Fatalf("Cleared again: send=%v, want false", send)
	}
}
