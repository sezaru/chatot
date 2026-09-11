package client

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	wastore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestClockJumped(t *testing.T) {
	base := time.Date(2026, 9, 11, 14, 30, 0, 0, time.UTC)
	if jumped, _ := clockJumped(base, base.Add(wakeTick)); jumped {
		t.Error("a tick's worth of wall time reads as a sleep")
	}
	if jumped, _ := clockJumped(base, base.Add(wakeJump)); jumped {
		t.Error("exactly the threshold reads as a sleep")
	}
	if jumped, gap := clockJumped(base, base.Add(2*time.Hour)); !jumped || gap != 2*time.Hour {
		t.Errorf("a two-hour jump: jumped=%v gap=%s, want true, 2h", jumped, gap)
	}
	// Two readings taken by time.Now carry monotonic clocks that, across a
	// suspend, disagree with the wall clock; the comparison must use the
	// wall clock, which is what a reading with its monotonic part
	// stripped and shifted stands in for here.
	prev := time.Now()
	now := prev.Round(0).Add(time.Hour)
	if jumped, _ := clockJumped(prev, now); !jumped {
		t.Error("an hour's wall-clock jump against a monotonic reading was missed")
	}
}

func TestNextReconnectDelay(t *testing.T) {
	d := reconnectRetryMin
	var seen []time.Duration
	for i := 0; i < 6; i++ {
		seen = append(seen, d)
		d = nextReconnectDelay(d)
	}
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("delays = %v, want %v", seen, want)
		}
	}
}

// reconnect leaves an unlinked or superseded session alone: it returns
// before touching the socket, which a bare client has none of.
func TestReconnectSkipsUnlinkedAndStaleSessions(t *testing.T) {
	unlinked := &whatsmeow.Client{Store: &wastore.Device{}}
	w := &Whatsmeow{log: waLog.Noop, wa: unlinked}
	w.reconnect(context.Background(), unlinked, "test")

	linked := &whatsmeow.Client{Store: &wastore.Device{ID: &types.JID{User: "1", Server: types.DefaultUserServer}}}
	w.wa = unlinked
	w.reconnect(context.Background(), linked, "test")
	if w.reconnecting.Load() {
		t.Error("reconnecting flag left set")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.wa = linked
	w.reconnect(ctx, linked, "test")
}
