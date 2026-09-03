package ui

import (
	"strings"
	"testing"
	"time"

	"chatot/internal/client"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func TestBubbleVM_FromMe(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	out := bubbleVM(client.Message{ID: "1", FromMe: true, Text: "hi", TS: now.Unix()}, nil, map[string]client.Message{}, now)
	if !out.FromMe {
		t.Error("expected FromMe=true for outbound message")
	}

	in := bubbleVM(client.Message{ID: "2", FromMe: false, Text: "hey", TS: now.Unix()}, nil, map[string]client.Message{}, now)
	if in.FromMe {
		t.Error("expected FromMe=false for incoming message")
	}
}

func TestBubbleVM_ReplyResolved(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")
	byID := map[string]client.Message{
		"orig": {ID: "orig", Text: "original text"},
	}
	m := client.Message{ID: "2", Text: "reply text", TS: now.Unix(), ReplyTo: &client.MsgRef{MsgID: "orig"}}

	out := bubbleVM(m, nil, byID, now)
	if !out.HasQuote {
		t.Fatal("expected HasQuote=true")
	}
	if out.QuotedText != "original text" {
		t.Errorf("QuotedText = %q, want %q", out.QuotedText, "original text")
	}
}

func TestBubbleVM_ReplyNotFoundFallback(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")
	m := client.Message{ID: "2", Text: "reply text", TS: now.Unix(), ReplyTo: &client.MsgRef{MsgID: "missing"}}

	out := bubbleVM(m, nil, map[string]client.Message{}, now)
	if !out.HasQuote {
		t.Fatal("expected HasQuote=true even when the target isn't found")
	}
	if out.QuotedText != "↩ reply" {
		t.Errorf("QuotedText = %q, want fallback %q", out.QuotedText, "↩ reply")
	}
}

func TestBubbleVM_DaySeparator(t *testing.T) {
	now := mustParse(t, "2026-08-30 18:00:00")
	day1 := mustParse(t, "2026-08-30 09:00:00")
	day2 := mustParse(t, "2026-08-31 09:00:00")

	first := client.Message{ID: "1", Text: "a", TS: day1.Unix()}
	sameDayMsg := client.Message{ID: "2", Text: "b", TS: day1.Add(time.Hour).Unix()}
	nextDayMsg := client.Message{ID: "3", Text: "c", TS: day2.Unix()}

	vFirst := bubbleVM(first, nil, nil, now)
	if !vFirst.ShowDaySeparator {
		t.Error("expected a day separator for the first message in the thread")
	}

	vSame := bubbleVM(sameDayMsg, &first, nil, now)
	if vSame.ShowDaySeparator {
		t.Error("expected no day separator for a message on the same day as prev")
	}

	vNext := bubbleVM(nextDayMsg, &sameDayMsg, nil, now)
	if !vNext.ShowDaySeparator {
		t.Error("expected a day separator when the calendar day changes")
	}
}

func TestDayText(t *testing.T) {
	now := mustParse(t, "2026-08-30 18:00:00")

	if got := dayText(now.Unix(), now); got != "Today" {
		t.Errorf("dayText(today) = %q, want Today", got)
	}
	yesterday := now.AddDate(0, 0, -1)
	if got := dayText(yesterday.Unix(), now); got != "Yesterday" {
		t.Errorf("dayText(yesterday) = %q, want Yesterday", got)
	}
	older := now.AddDate(0, 0, -10)
	want := older.Format("02/01/2006")
	if got := dayText(older.Unix(), now); got != want {
		t.Errorf("dayText(older) = %q, want %q", got, want)
	}
}

func TestBubbleVM_Reactions(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")
	m := client.Message{
		ID:        "1",
		Text:      "funny",
		TS:        now.Unix(),
		Reactions: map[string][]string{"👍": {"a@s.whatsapp.net", "c@s.whatsapp.net"}, "😂": {"b@s.whatsapp.net"}},
	}

	out := bubbleVM(m, nil, nil, now)
	if len(out.Reactions) != 2 {
		t.Fatalf("Reactions = %v, want 2 entries", out.Reactions)
	}
	// Emoji-sorted, with the count of people behind each pill.
	if out.Reactions[0].Emoji != "👍" || out.Reactions[0].Count != 2 {
		t.Errorf("first pill = %+v, want 👍 ×2", out.Reactions[0])
	}
	if out.Reactions[1].Emoji != "😂" || out.Reactions[1].Count != 1 {
		t.Errorf("second pill = %+v, want 😂 ×1", out.Reactions[1])
	}
	// The mockup only prints a number past one reaction.
	if got := reactionCountText(1); got != "" {
		t.Errorf("reactionCountText(1) = %q, want empty", got)
	}
	if got := reactionCountText(3); got != "3" {
		t.Errorf("reactionCountText(3) = %q", got)
	}
	// Our own reaction is found even when the reactor JID carries a device
	// suffix, and never matches an empty own JID.
	if !reactedBy([]string{"a@s.whatsapp.net", "1234567890:12@s.whatsapp.net"}, "1234567890@s.whatsapp.net") {
		t.Error("reactedBy missed a device-suffixed own JID")
	}
	if reactedBy([]string{"a@s.whatsapp.net"}, "") {
		t.Error("reactedBy matched an empty own JID")
	}
}

func TestBubbleVM_MediaMessage(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	m := client.Message{
		ID: "1",
		TS: now.Unix(),
		Attachment: &client.Attachment{
			Kind:     "document",
			Filename: "report.pdf",
		},
	}

	out := bubbleVM(m, nil, nil, now)
	if !out.IsMedia {
		t.Fatal("expected IsMedia=true for an attachment message")
	}
	if out.MediaChip != "📄 report.pdf" {
		t.Errorf("MediaChip = %q, want %q", out.MediaChip, "📄 report.pdf")
	}
	if out.Text != "" {
		t.Errorf("Text = %q, want empty for a media message", out.Text)
	}
}

func TestBubbleVM_TextMessage(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	m := client.Message{ID: "1", Text: "plain text", TS: now.Unix()}
	out := bubbleVM(m, nil, nil, now)
	if out.IsMedia {
		t.Error("expected IsMedia=false for a plain text message")
	}
	if out.Text != "plain text" {
		t.Errorf("Text = %q, want %q", out.Text, "plain text")
	}
}

func TestBubbleVM_EditedMarker(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	out := bubbleVM(client.Message{ID: "1", FromMe: true, Text: "typo", TS: now.Unix(), Edited: true}, nil, map[string]client.Message{}, now)
	if !out.Edited {
		t.Error("expected Edited=true on the view-model")
	}
	if out.EditedMarker == "" {
		t.Error("expected a non-empty edited marker")
	}

	plain := bubbleVM(client.Message{ID: "2", FromMe: true, Text: "hi", TS: now.Unix()}, nil, map[string]client.Message{}, now)
	if plain.Edited || plain.EditedMarker != "" {
		t.Error("expected no edited marker on an unedited message")
	}
}

func TestBubbleVM_ForwardedMarker(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	out := bubbleVM(client.Message{ID: "1", Text: "fyi", TS: now.Unix(), Forwarded: true}, nil, map[string]client.Message{}, now)
	if !out.Forwarded {
		t.Error("expected Forwarded=true on the view-model")
	}

	plain := bubbleVM(client.Message{ID: "2", Text: "hi", TS: now.Unix()}, nil, map[string]client.Message{}, now)
	if plain.Forwarded {
		t.Error("expected Forwarded=false on a non-forwarded message")
	}

	deleted := bubbleVM(client.Message{ID: "3", Text: "gone", TS: now.Unix(), Forwarded: true, Deleted: true}, nil, map[string]client.Message{}, now)
	if deleted.Forwarded {
		t.Error("a deleted message must not show the forwarded marker")
	}
}

func TestBubbleVM_DeletedRendersTombstone(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	out := bubbleVM(client.Message{ID: "1", Text: "the original text", Deleted: true, TS: now.Unix()}, nil, nil, now)
	if !out.Deleted {
		t.Error("expected Deleted=true on the view-model")
	}
	if out.Text != tombstoneText {
		t.Errorf("Text = %q, want the tombstone text", out.Text)
	}
	if out.IsMedia || out.IsLocation || out.IsContact || out.IsPoll {
		t.Error("a deleted message must not render as any rich kind")
	}
	if len(out.Reactions) != 0 {
		t.Error("a deleted message must not show its reactions")
	}
}

// TestBubbleVM_DeletedTakesPrecedenceOverMedia proves the Deleted branch in
// bubbleVM is checked before the media/location/contact/poll switch, since a
// revoke can arrive for a message of any original kind.
func TestBubbleVM_DeletedTakesPrecedenceOverMedia(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	m := client.Message{
		ID: "1", TS: now.Unix(), Deleted: true,
		Attachment: &client.Attachment{Kind: "image", Caption: "beach"},
	}
	out := bubbleVM(m, nil, nil, now)
	if out.IsMedia {
		t.Error("expected IsMedia=false; deleted must take precedence over media")
	}
	if out.Text != tombstoneText {
		t.Errorf("Text = %q, want the tombstone text even though the message carried media", out.Text)
	}
}

func TestTickVM(t *testing.T) {
	cases := []struct {
		status   int
		wantText string
		wantRead bool
	}{
		{0, "✓", false},
		{1, "✓✓", false},
		{2, "✓✓", true},
	}
	for _, c := range cases {
		text, read := tickVM(c.status)
		if text != c.wantText || read != c.wantRead {
			t.Errorf("tickVM(%d) = (%q, %v), want (%q, %v)", c.status, text, read, c.wantText, c.wantRead)
		}
	}
}

func TestBubbleVM_TickOnlyOnFromMe(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	out := bubbleVM(client.Message{ID: "1", FromMe: true, Text: "hi", TS: now.Unix(), Status: 2}, nil, nil, now)
	if out.TickText != "✓✓" || !out.TickRead {
		t.Errorf("got TickText=%q TickRead=%v, want read double-tick on a FromMe message", out.TickText, out.TickRead)
	}

	in := bubbleVM(client.Message{ID: "2", FromMe: false, Text: "hey", TS: now.Unix(), Status: 2}, nil, nil, now)
	if in.TickText != "" {
		t.Errorf("got TickText=%q, want no tick on an inbound message", in.TickText)
	}
}

func TestStarAffordanceVM(t *testing.T) {
	glyph, tooltip := starAffordanceVM(false)
	if glyph != "☆" || tooltip != "Star" {
		t.Errorf("starAffordanceVM(false) = (%q, %q), want (☆, Star)", glyph, tooltip)
	}

	glyph, tooltip = starAffordanceVM(true)
	if glyph != "★" || tooltip != "Unstar" {
		t.Errorf("starAffordanceVM(true) = (%q, %q), want (★, Unstar)", glyph, tooltip)
	}
}

func TestBubbleVM_StarAffordanceReflectsStarredState(t *testing.T) {
	now := mustParse(t, "2026-08-30 12:00:00")

	starred := bubbleVM(client.Message{ID: "1", Text: "hi", TS: now.Unix(), Starred: true}, nil, nil, now)
	if starred.StarGlyph != "★" || starred.StarTooltip != "Unstar" {
		t.Errorf("got StarGlyph=%q StarTooltip=%q, want filled star / Unstar", starred.StarGlyph, starred.StarTooltip)
	}

	unstarred := bubbleVM(client.Message{ID: "2", Text: "hey", TS: now.Unix()}, nil, nil, now)
	if unstarred.StarGlyph != "☆" || unstarred.StarTooltip != "Star" {
		t.Errorf("got StarGlyph=%q StarTooltip=%q, want outline star / Star", unstarred.StarGlyph, unstarred.StarTooltip)
	}
}

func TestNextHistoryAction(t *testing.T) {
	tests := []struct {
		name           string
		olderCount     int
		alreadyRequest bool
		wantRequest    bool
		wantExhausted  bool
	}{
		{"non-empty page keeps paging locally", 5, false, false, false},
		{"non-empty page even if already requested", 5, true, false, false},
		{"first empty page requests more history", 0, false, true, false},
		{"second empty page after a request is exhausted", 0, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, exhausted := nextHistoryAction(tt.olderCount, tt.alreadyRequest)
			if request != tt.wantRequest || exhausted != tt.wantExhausted {
				t.Errorf("nextHistoryAction(%d, %v) = (%v, %v), want (%v, %v)",
					tt.olderCount, tt.alreadyRequest, request, exhausted, tt.wantRequest, tt.wantExhausted)
			}
		})
	}
}

func TestCopyableText(t *testing.T) {
	if got := copyableText(client.Message{Text: "hi"}); got != "hi" {
		t.Errorf("text = %q", got)
	}
	// Rich kinds copy a plain-text rendering, so "Copy text" is never inert
	// on a location, contact or poll the way it was.
	loc := copyableText(client.Message{Location: &client.Location{Name: "Bletchley Park", Address: "Sherwood Dr", Latitude: 1, Longitude: 2}})
	if !strings.HasPrefix(loc, "Bletchley Park\nSherwood Dr\nhttps://www.openstreetmap.org/") {
		t.Errorf("location = %q", loc)
	}
	if got := copyableText(client.Message{Contact: &client.Contact{DisplayName: "Ada", Phones: []string{"+1"}}}); got != "Ada\n+1" {
		t.Errorf("contact = %q", got)
	}
	if got := copyableText(client.Message{Poll: &client.Poll{Name: "Lunch?", Options: []client.PollOption{{Name: "Pizza"}}}}); got != "Lunch?\n• Pizza" {
		t.Errorf("poll = %q", got)
	}
	if got := copyableText(client.Message{Attachment: &client.Attachment{Kind: "image", Caption: "cap"}}); got != "cap" {
		t.Errorf("caption = %q", got)
	}
	// Nothing to copy: a deleted message, or media with no caption.
	if got := copyableText(client.Message{Text: "x", Deleted: true}); got != "" {
		t.Errorf("deleted = %q", got)
	}
	if got := copyableText(client.Message{Attachment: &client.Attachment{Kind: "image"}}); got != "" {
		t.Errorf("bare media = %q", got)
	}
}

func TestMessageInfoRows(t *testing.T) {
	now := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
	ts := time.Date(2026, 9, 2, 14, 30, 0, 0, time.UTC).Unix()
	rows := messageInfoRows(client.Message{FromMe: true, TS: ts, Status: client.MessageStatusRead, Starred: true}, now)
	want := [][2]string{{"Sent", "14:30"}, {"Delivered", "✓✓"}, {"Read", "✓✓"}, {"Starred", "Yes"}}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %v, want %v", i, rows[i], want[i])
		}
	}
	// A received message only says when; an older one carries the date.
	old := messageInfoRows(client.Message{TS: ts - 86400*3}, now)
	if len(old) != 1 || old[0][0] != "Received" || !strings.Contains(old[0][1], "/") {
		t.Errorf("received = %v", old)
	}
}

func TestBubbleSigDetectsLiveChanges(t *testing.T) {
	base := client.Message{ID: "m1", FromMe: true, Text: "hi", TS: 100, Status: client.MessageStatusDelivered}

	// Same content → identical signature (refreshInPlace skips the row).
	if bubbleSig(base) != bubbleSig(base) {
		t.Fatal("bubbleSig not stable for identical messages")
	}

	// Each live mutation must change the signature so the row re-renders.
	cases := map[string]func(m client.Message) client.Message{
		"read receipt": func(m client.Message) client.Message { m.Status = client.MessageStatusRead; return m },
		"reaction":     func(m client.Message) client.Message { m.Reactions = map[string][]string{"👍": {"x@s"}}; return m },
		"revoke":       func(m client.Message) client.Message { m.Deleted = true; return m },
		"edit":         func(m client.Message) client.Message { m.Edited = true; m.Text = "hello"; return m },
		"star":         func(m client.Message) client.Message { m.Starred = true; return m },
		"poll tally": func(m client.Message) client.Message {
			m.Poll = &client.Poll{Options: []client.PollOption{{Name: "A", Count: 2}}}
			return m
		},
	}
	for name, mut := range cases {
		if bubbleSig(base) == bubbleSig(mut(base)) {
			t.Errorf("%s: signature unchanged, row would not refresh", name)
		}
	}
}

func TestAlignPopoverX(t *testing.T) {
	// An incoming bubble's card starts at the bubble's left edge; an
	// outgoing bubble's card ends at its right edge, so a card under a bubble
	// hugging the pane's right margin grows inward rather than off-window.
	if got := alignPopoverX(100, 80, 244, false); got != 100 {
		t.Errorf("incoming x = %d, want 100", got)
	}
	if got := alignPopoverX(700, 80, 244, true); got != 700+80-244 {
		t.Errorf("outgoing x = %d, want %d", got, 700+80-244)
	}
}

func TestRowOnScreen(t *testing.T) {
	cases := []struct {
		y, h, viewport float64
		want           bool
	}{
		{100, 40, 600, true},
		{-20, 40, 600, true},  // partly above
		{590, 40, 600, true},  // partly below
		{-50, 40, 600, false}, // fully above
		{600, 40, 600, false}, // fully below
	}
	for _, c := range cases {
		if got := rowOnScreen(c.y, c.h, c.viewport); got != c.want {
			t.Errorf("rowOnScreen(%v, %v, %v) = %v, want %v", c.y, c.h, c.viewport, got, c.want)
		}
	}
}
