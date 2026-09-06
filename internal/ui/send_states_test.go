package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestTickVMUnsentStatesShowNoTick(t *testing.T) {
	for _, status := range []int{client.MessageStatusPending, client.MessageStatusFailed} {
		if text, read := tickVM(status); text != "" || read {
			t.Errorf("tickVM(%d) = %q,%v; want no tick", status, text, read)
		}
	}
}

func TestBubbleVMPendingAndFailedFlags(t *testing.T) {
	now := time.Now()
	pending := bubbleVM(client.Message{ID: "local-1", FromMe: true, Text: "hi", TS: now.Unix(), Status: client.MessageStatusPending}, nil, nil, now)
	if !pending.Pending || pending.Failed || pending.TickText != "" {
		t.Errorf("pending: %+v", pending)
	}
	failed := bubbleVM(client.Message{ID: "local-2", FromMe: true, Text: "hi", TS: now.Unix(), Status: client.MessageStatusFailed}, nil, nil, now)
	if !failed.Failed || failed.Pending || failed.TickText != "" {
		t.Errorf("failed: %+v", failed)
	}
	sent := bubbleVM(client.Message{ID: "m", FromMe: true, Text: "hi", TS: now.Unix()}, nil, nil, now)
	if sent.Pending || sent.Failed || sent.TickText != "✓" {
		t.Errorf("sent: %+v", sent)
	}
	// Inbound messages never carry a send state, whatever Status says.
	in := bubbleVM(client.Message{ID: "x", Text: "hi", TS: now.Unix(), Status: client.MessageStatusFailed}, nil, nil, now)
	if in.Pending || in.Failed {
		t.Errorf("inbound: %+v", in)
	}
}

func TestIsUnsent(t *testing.T) {
	if !isUnsent(client.Message{Status: client.MessageStatusPending}) || !isUnsent(client.Message{Status: client.MessageStatusFailed}) {
		t.Error("pending/failed should be unsent")
	}
	for _, s := range []int{client.MessageStatusSent, client.MessageStatusDelivered, client.MessageStatusRead} {
		if isUnsent(client.Message{Status: s}) {
			t.Errorf("status %d should not be unsent", s)
		}
	}
}

func TestCaptionText(t *testing.T) {
	cases := []struct {
		att  client.Attachment
		want string
	}{
		{client.Attachment{Kind: "image", Caption: "  beach day "}, "beach day"},
		{client.Attachment{Kind: "video", Caption: "clip"}, "clip"},
		{client.Attachment{Kind: "document", Caption: "the deck", Filename: "deck.pdf"}, "the deck"},
		// A nameless document is titled by its caption; no second line.
		{client.Attachment{Kind: "document", Caption: "the deck"}, ""},
		{client.Attachment{Kind: "image"}, ""},
		{client.Attachment{Kind: "sticker", Caption: "never"}, ""},
		{client.Attachment{Kind: "audio", Caption: "never"}, ""},
	}
	for _, c := range cases {
		if got := captionText(c.att); got != c.want {
			t.Errorf("captionText(%s %q) = %q, want %q", c.att.Kind, c.att.Caption, got, c.want)
		}
	}
}

func TestBubbleVMMediaCaptionText(t *testing.T) {
	now := time.Now()
	v := bubbleVM(client.Message{ID: "m", TS: now.Unix(), Attachment: &client.Attachment{Kind: "image", Caption: "sunset"}}, nil, nil, now)
	if !v.IsMedia || v.CaptionText != "sunset" {
		t.Errorf("CaptionText = %q (IsMedia %v), want sunset", v.CaptionText, v.IsMedia)
	}
	// A deleted media message is a tombstone: no caption survives.
	d := bubbleVM(client.Message{ID: "m", TS: now.Unix(), Deleted: true, Attachment: &client.Attachment{Kind: "image", Caption: "sunset"}}, nil, nil, now)
	if d.CaptionText != "" {
		t.Errorf("deleted CaptionText = %q", d.CaptionText)
	}
}

func TestReactionTooltipListsNames(t *testing.T) {
	cases := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"You"}, "You"},
		{[]string{"You", "Ana"}, "You and Ana"},
		{[]string{"You", "Ana", "Marco"}, "You, Ana and Marco"},
	}
	for _, c := range cases {
		if got := reactionTooltip(c.names); got != c.want {
			t.Errorf("reactionTooltip(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}

func TestReactorNamesResolveAndFallBack(t *testing.T) {
	name := func(jid string) string {
		if jid == "me@s.whatsapp.net" {
			return "You"
		}
		if jid == "5511@s.whatsapp.net" {
			return "Ana"
		}
		return ""
	}
	got := reactorNames([]string{"me@s.whatsapp.net", "5511@s.whatsapp.net", "5522:3@s.whatsapp.net"}, name)
	want := []string{"You", "Ana", "5522"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if bare := reactorNames([]string{"5533@s.whatsapp.net"}, nil); bare[0] != "5533" {
		t.Errorf("nil resolver: %v", bare)
	}
}

func TestReactorListCaption(t *testing.T) {
	if got := reactorListCaption(reactionView{Emoji: "👍", Count: 1}); got != "👍 · 1 reaction" {
		t.Errorf("one: %q", got)
	}
	if got := reactorListCaption(reactionView{Emoji: "❤️", Count: 3}); got != "❤️ · 3 reactions" {
		t.Errorf("three: %q", got)
	}
}

func TestMediaVMDocumentTitleIsFilename(t *testing.T) {
	mv := mediaVM(client.Message{Attachment: &client.Attachment{Kind: "document", Filename: "deck.pdf", Caption: "the deck"}})
	if mv.Caption != "deck.pdf" {
		t.Errorf("document title = %q, want the filename", mv.Caption)
	}
	nameless := mediaVM(client.Message{Attachment: &client.Attachment{Kind: "document", Caption: "the deck"}})
	if nameless.Caption != "the deck" {
		t.Errorf("nameless document title = %q, want the caption", nameless.Caption)
	}
	pic := mediaVM(client.Message{Attachment: &client.Attachment{Kind: "image", Filename: "a.jpg", Caption: "beach"}})
	if pic.Caption != "beach" {
		t.Errorf("image caption = %q", pic.Caption)
	}
}

func TestWithUnsentAppendsAfterStoredPage(t *testing.T) {
	cv := &ConversationView{unsent: map[string][]client.Message{}}
	stored := []client.Message{{ID: "a"}, {ID: "b"}}
	if got := cv.withUnsent("j", stored); len(got) != 2 {
		t.Fatalf("no unsent: %d rows", len(got))
	}
	cv.unsent["j"] = []client.Message{{ID: "local-1", ChatJID: "j", Status: client.MessageStatusPending}}
	cv.unsent["k"] = []client.Message{{ID: "local-9", ChatJID: "k", Status: client.MessageStatusFailed}}
	got := cv.withUnsent("j", stored)
	if len(got) != 3 || got[2].ID != "local-1" {
		t.Fatalf("got %v", got)
	}
	// The stored page is left alone: the result is a fresh slice.
	got[0].ID = "changed"
	if len(stored) != 2 || stored[0].ID != "a" {
		t.Error("withUnsent aliased the stored page")
	}
}

func TestStoredPositionsSkipUnsentAndTyping(t *testing.T) {
	cv := &ConversationView{msgs: []client.Message{
		{ID: "a"},
		{ID: "local-1", Status: client.MessageStatusPending},
		{ID: "b"},
		{ID: "local-2", Status: client.MessageStatusFailed},
		{ID: typingSentinelID},
	}}
	got := cv.storedPositions()
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("storedPositions = %v, want [0 2]", got)
	}
}

func TestLocalMessageIDsAreUnique(t *testing.T) {
	c := &Composer{}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := c.localMessageID()
		if seen[id] {
			t.Fatalf("duplicate local id %q", id)
		}
		seen[id] = true
	}
	if !isUnsent(client.Message{Status: client.MessageStatusPending}) {
		t.Error("pending row must count as unsent")
	}
}
