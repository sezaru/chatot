package ui

import (
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
		Reactions: map[string]string{"👍": "a@s.whatsapp.net", "😂": "b@s.whatsapp.net"},
	}

	out := bubbleVM(m, nil, nil, now)
	if len(out.Reactions) != 2 {
		t.Fatalf("Reactions = %v, want 2 entries", out.Reactions)
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
	if out.MediaChip != "[document] report.pdf" {
		t.Errorf("MediaChip = %q, want %q", out.MediaChip, "[document] report.pdf")
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
