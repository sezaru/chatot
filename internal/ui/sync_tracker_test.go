package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func chunk(kind string, chats, msgs, progress int) *client.HistorySync {
	return &client.HistorySync{Type: kind, Chats: chats, Messages: msgs, Progress: progress}
}

func TestSyncTrackerBlocksUntilRecentChatsSettle(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var tr SyncTracker
	tr.Pair(t0)
	if !tr.Blocking() {
		t.Fatal("pairing must start the blocking phase")
	}
	// Push names and status chunks are not the backlog.
	tr.Chunk(chunk("pushname", 0, 0, -1), t0.Add(time.Second))
	if tr.Counts() != "" {
		t.Fatalf("counts after a pushname chunk = %q, want empty", tr.Counts())
	}
	tr.Chunk(chunk("bootstrap", 120, 300, -1), t0.Add(2*time.Second))
	tr.Chunk(chunk("recent", 40, 3913, -1), t0.Add(3*time.Second))
	if got := tr.Counts(); got != "160 chats · 4,213 messages" {
		t.Fatalf("counts = %q", got)
	}
	if tr.Tick(t0.Add(5 * time.Second)) {
		t.Fatal("must stay blocking while chunks are still fresh")
	}
	if !tr.Tick(t0.Add(3*time.Second+syncBlockingQuiet)) || tr.phase != syncIdle {
		t.Fatalf("quiet after recent chats must end the sync, phase = %v", tr.phase)
	}
}

func TestSyncTrackerFullChunksHandOverToBanner(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var tr SyncTracker
	tr.Pair(t0)
	tr.Chunk(chunk("bootstrap", 10, 20, -1), t0.Add(time.Second))
	tr.Chunk(chunk("full", 5, 500, 10), t0.Add(2*time.Second))
	if tr.Blocking() || !tr.Background() {
		t.Fatalf("a full chunk must unblock into the background phase, phase = %v", tr.phase)
	}
	if got := tr.BannerText(); got != "Syncing older messages · 10%" {
		t.Fatalf("banner = %q", got)
	}
	if f := tr.Fraction(); f != 0.1 {
		t.Fatalf("fraction = %v, want 0.1", f)
	}
	tr.Chunk(chunk("full", 5, 500, 100), t0.Add(3*time.Second))
	if tr.phase != syncIdle {
		t.Fatalf("100%% must retire the banner, phase = %v", tr.phase)
	}
}

func TestSyncTrackerTimeouts(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var tr SyncTracker
	tr.Pair(t0)
	// Nothing ever arrives: the screen still gives way.
	if tr.Tick(t0.Add(syncBlockingMax - time.Second)) {
		t.Fatal("blocking must hold until the cap")
	}
	if !tr.Tick(t0.Add(syncBlockingMax)) || tr.phase != syncIdle {
		t.Fatal("blocking must give up at the cap")
	}
	// A full chunk with no pairing (restart mid-sync) is background only.
	tr.Chunk(chunk("full", 1, 10, -1), t0)
	if !tr.Background() || tr.BannerText() != "Syncing older messages…" || tr.Fraction() != -1 {
		t.Fatalf("restart mid-sync: phase %v banner %q", tr.phase, tr.BannerText())
	}
	if !tr.Tick(t0.Add(syncBackgroundQuiet)) || tr.phase != syncIdle {
		t.Fatal("a stalled background sync must retire the banner")
	}
}

func TestGroupThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 4213: "4,213", 1234567: "1,234,567"} {
		if got := groupThousands(n); got != want {
			t.Errorf("groupThousands(%d) = %q, want %q", n, got, want)
		}
	}
	if got := plural(1, "chat"); got != "1 chat" {
		t.Errorf("plural(1) = %q", got)
	}
}
