package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestGroupStatuses(t *testing.T) {
	own := "1234567890@s.whatsapp.net"
	msgs := []client.Message{
		{ID: "a1", FromJID: "ada@s.whatsapp.net", TS: 100},
		{ID: "a2", FromJID: "ada@s.whatsapp.net", TS: 300},
		{ID: "g1", FromJID: "grace@s.whatsapp.net", TS: 200, Status: client.MessageStatusRead},
		{ID: "m1", FromJID: own, FromMe: true, TS: 50},
		{ID: "d1", FromJID: "gone@s.whatsapp.net", TS: 400, Deleted: true},
		{ID: "q1", FromJID: "quiet@s.whatsapp.net", TS: 250, Status: client.MessageStatusRead},
	}
	names := map[string]string{"ada@s.whatsapp.net": "Ada", "grace@s.whatsapp.net": "Grace"}
	feed := groupStatuses(msgs, own, names, map[string]bool{"quiet@s.whatsapp.net": true})

	if feed.Mine == nil || len(feed.Mine.Items) != 1 || !feed.Mine.Mine {
		t.Fatalf("own status not grouped: %+v", feed.Mine)
	}
	if len(feed.Recent) != 1 || feed.Recent[0].Name != "Ada" || len(feed.Recent[0].Items) != 2 {
		t.Fatalf("recent = %+v", feed.Recent)
	}
	// Newest update first within a poster.
	if feed.Recent[0].Items[0].ID != "a2" {
		t.Errorf("items not newest first: %v", feed.Recent[0].Items)
	}
	if len(feed.Viewed) != 1 || feed.Viewed[0].Name != "Grace" || !feed.Viewed[0].Viewed {
		t.Fatalf("viewed = %+v", feed.Viewed)
	}
	// A muted poster files under Muted updates whatever its viewed state.
	if len(feed.Muted) != 1 || feed.Muted[0].JID != "quiet@s.whatsapp.net" || !feed.Muted[0].Muted {
		t.Fatalf("muted = %+v", feed.Muted)
	}
	// The deleted update leaves no poster behind.
	for _, p := range append(feed.Recent, feed.Viewed...) {
		if p.JID == "gone@s.whatsapp.net" {
			t.Errorf("deleted status still listed")
		}
	}
}

func TestGroupStatusesOrdersPostersByLatest(t *testing.T) {
	msgs := []client.Message{
		{ID: "b", FromJID: "b@s.whatsapp.net", TS: 100},
		{ID: "a", FromJID: "a@s.whatsapp.net", TS: 900},
	}
	feed := groupStatuses(msgs, "", nil, nil)
	if len(feed.Recent) != 2 || feed.Recent[0].JID != "a@s.whatsapp.net" {
		t.Errorf("recent order = %+v", feed.Recent)
	}
}

func TestStatusRowText(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.Local)
	p := statusPoster{Name: "Ada", Items: []client.Message{{TS: now.Add(-time.Hour).Unix()}, {TS: now.Add(-2 * time.Hour).Unix()}}}
	if got := statusRowSub(p, now); got != "2 updates · 11:00" {
		t.Errorf("statusRowSub = %q", got)
	}
	if got := myStatusSub(nil, now); got != "Add to my status — it disappears after 24h" {
		t.Errorf("myStatusSub(nil) = %q", got)
	}
	mine := &statusPoster{Mine: true, Items: []client.Message{{TS: now.Add(-time.Hour).Unix()}}}
	if got := myStatusSub(mine, now); got != "Today, 11:00" {
		t.Errorf("myStatusSub = %q", got)
	}
	if got := statusViewerMeta(client.Message{TS: now.Add(-time.Hour).Unix()}, 0, 2, now); got != "11:00 · 1 of 2" {
		t.Errorf("statusViewerMeta = %q", got)
	}
}

func TestStatusProgressAndPauseTooltip(t *testing.T) {
	if got := statusProgress(0); got != 0 {
		t.Errorf("progress at 0 = %v", got)
	}
	if got := statusProgress(2500 * time.Millisecond); got < 0.49 || got > 0.51 {
		t.Errorf("progress at 2.5s = %v, want 0.5", got)
	}
	if got := statusProgress(9 * time.Second); got != 1 {
		t.Errorf("progress past the end = %v, want 1", got)
	}
	if pauseTooltip(false) != "Pause" || pauseTooltip(true) != "Resume" {
		t.Errorf("pause tooltips wrong")
	}
}

func TestGroupStatusesViewedNeedsEveryUpdateRead(t *testing.T) {
	msgs := []client.Message{
		{ID: "a1", FromJID: "a@s.whatsapp.net", TS: 100, Status: client.MessageStatusRead},
		{ID: "a2", FromJID: "a@s.whatsapp.net", TS: 200},
	}
	feed := groupStatuses(msgs, "", nil, nil)
	if len(feed.Recent) != 1 || feed.Recent[0].Viewed {
		t.Fatalf("a poster with one unread update is recent: %+v", feed)
	}
	if viewedByText(3) != "Viewed by 3" {
		t.Fatal("viewedByText")
	}
}

func TestStatusFeedPosterSearchesEverySection(t *testing.T) {
	msgs := []client.Message{
		{ID: "r1", FromJID: "recent@s.whatsapp.net", TS: 100},
		{ID: "v1", FromJID: "viewed@s.whatsapp.net", TS: 200, Status: client.MessageStatusRead},
		{ID: "m1", FromJID: "muted@s.whatsapp.net", TS: 300},
		{ID: "me1", FromJID: "own@s.whatsapp.net", FromMe: true, TS: 50},
	}
	feed := groupStatuses(msgs, "own@s.whatsapp.net", nil, map[string]bool{"muted@s.whatsapp.net": true})
	for _, jid := range []string{"recent@s.whatsapp.net", "viewed@s.whatsapp.net", "muted@s.whatsapp.net", "me"} {
		if p := feed.poster(jid); p == nil || len(p.Items) != 1 {
			t.Errorf("poster(%s) = %+v", jid, p)
		}
	}
	if feed.poster("nobody@s.whatsapp.net") != nil {
		t.Error("unknown poster must be nil")
	}
}

func TestPosterNamesFallsBackToContacts(t *testing.T) {
	c := client.NewFake()
	msgs := []client.Message{{ID: "x", FromJID: "4445556666@s.whatsapp.net"}}
	names := posterNames(c, msgs)
	if got := names["4445556666@s.whatsapp.net"]; got == "" {
		t.Fatalf("poster with no chat got no name; want the contacts table's")
	}
}
