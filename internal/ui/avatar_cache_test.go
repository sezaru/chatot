package ui

import "testing"

// One fetch per jid however many rows ask; a failed fetch keeps its rows
// waiting for a retry, a successful one hands them back once.
func TestAvatarCacheWaiters(t *testing.T) {
	c := newAvatarCache()
	if !c.addWaiter("a", avatarWaiter{initial: "A"}) {
		t.Fatal("first waiter should start a fetch")
	}
	if c.addWaiter("a", avatarWaiter{initial: "A2"}) {
		t.Fatal("second waiter must not start another fetch")
	}
	if got := c.failedJIDs(); len(got) != 0 {
		t.Fatalf("in-flight jid listed as failed: %v", got)
	}
	if ws := c.takeWaiters("a", false); ws != nil {
		t.Fatalf("failure must keep the waiters, got %d", len(ws))
	}
	if got := c.failedJIDs(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("failed jids = %v, want [a]", got)
	}
	if !c.addWaiter("a", avatarWaiter{initial: "A3"}) {
		t.Fatal("after a failure the next ask should fetch again")
	}
	ws := c.takeWaiters("a", true)
	if len(ws) != 3 {
		t.Fatalf("success should hand back all 3 waiters, got %d", len(ws))
	}
	if ws = c.takeWaiters("a", true); len(ws) != 0 {
		t.Fatalf("waiters handed back twice: %d", len(ws))
	}
	if got := c.failedJIDs(); len(got) != 0 {
		t.Fatalf("resolved jid listed as failed: %v", got)
	}
}

func TestAvatarDecodeSideFor(t *testing.T) {
	if got := avatarDecodeSideFor(chatRowAvatarSize); got != 96 {
		t.Fatalf("row avatar decode side = %d, want 96", got)
	}
	if got := avatarDecodeSideFor(96); got != avatarDecodeSide {
		t.Fatalf("large avatar decode side = %d, want %d", got, avatarDecodeSide)
	}
}
