package client

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

func TestOfflineProgress(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var p offlineProgress
	if _, ok := p.message(t0); ok {
		t.Fatal("a live message outside a backlog must not report progress")
	}
	if got := p.preview(&events.OfflineSyncPreview{Total: 12, Messages: 10}, t0); got != (OfflineSync{Total: 12, Messages: 10}) {
		t.Fatalf("preview = %+v", got)
	}
	if _, ok := p.message(t0.Add(offlineProgressEvery / 2)); ok {
		t.Fatal("progress must be spaced out")
	}
	got, ok := p.message(t0.Add(offlineProgressEvery))
	if !ok || got.Received != 2 {
		t.Fatalf("progress = %+v %v, want 2 received", got, ok)
	}
	if got := p.completed(); !got.Done || got.Received != 10 {
		t.Fatalf("completed = %+v", got)
	}
	if _, ok := p.message(t0.Add(time.Hour)); ok {
		t.Fatal("no progress after the backlog ended")
	}
}
