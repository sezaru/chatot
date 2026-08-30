package client

import (
	"testing"

	waLog "go.mau.fi/whatsmeow/util/log"

	"chatot/internal/store"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newIngestFixture(t *testing.T) *Whatsmeow {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &Whatsmeow{log: waLog.Noop, store: s}
}

// chatByJID reads back a single chat through the store's own query so the
// ingest assertions exercise the real read path, not internal SQL.
func chatByJID(t *testing.T, w *Whatsmeow, jid string) (store.Chat, bool) {
	t.Helper()
	chats, err := w.store.Chats(100)
	if err != nil {
		t.Fatalf("store.Chats: %v", err)
	}
	for _, c := range chats {
		if c.JID == jid {
			return c, true
		}
	}
	return store.Chat{}, false
}

func TestIngestMessageGroupJIDSetsIsGroup(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "120363000000000000@g.us", ID: "m1", Text: "hi", TS: 10}))

	c, ok := chatByJID(t, w, "120363000000000000@g.us")
	if !ok {
		t.Fatal("group chat not created")
	}
	if !c.IsGroup {
		t.Error("IsGroup = false, want true for a @g.us JID")
	}
}

func TestIngestMessageDMJIDNotGroup(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hi", TS: 10}))

	c, ok := chatByJID(t, w, "1234567890@s.whatsapp.net")
	if !ok {
		t.Fatal("DM chat not created")
	}
	if c.IsGroup {
		t.Error("IsGroup = true, want false for a DM JID")
	}
}

func TestIngestInboundMessageIncrementsUnread(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hi", TS: 10, FromMe: false}))
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m2", Text: "again", TS: 11, FromMe: false}))

	c, _ := chatByJID(t, w, "1234567890@s.whatsapp.net")
	if c.UnreadCount != 2 {
		t.Errorf("UnreadCount = %d, want 2 after two inbound messages", c.UnreadCount)
	}
}

func TestIngestFromMeMessageDoesNotIncrementUnread(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hi", TS: 10, FromMe: true}))

	c, _ := chatByJID(t, w, "1234567890@s.whatsapp.net")
	if c.UnreadCount != 0 {
		t.Errorf("UnreadCount = %d, want 0 for a from-me message", c.UnreadCount)
	}
}

func TestIngestMessageStoredAndReadable(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{
		ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hello", TS: 10,
		Attachment: &Attachment{Kind: "image", Caption: "pic", MimeType: "image/jpeg"},
	}))

	msgs, err := w.store.Messages("1234567890@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Fatalf("got %+v, want one stored message", msgs)
	}
	if msgs[0].Attachment == nil || msgs[0].Attachment.Kind != "image" {
		t.Fatalf("got attachment %+v, want image", msgs[0].Attachment)
	}
}

func TestIngestReadReceiptClearsUnread(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hi", TS: 10, FromMe: false}))
	if c, _ := chatByJID(t, w, "1234567890@s.whatsapp.net"); c.UnreadCount != 1 {
		t.Fatalf("precondition: UnreadCount = %d, want 1", c.UnreadCount)
	}

	must(t, w.ingestReceipt(&Receipt{ChatJID: "1234567890@s.whatsapp.net", MsgIDs: []string{"m1"}, Read: true}))

	c, _ := chatByJID(t, w, "1234567890@s.whatsapp.net")
	if c.UnreadCount != 0 {
		t.Errorf("UnreadCount = %d, want 0 after a read receipt", c.UnreadCount)
	}
}

func TestIngestDeliveredReceiptDoesNotClearUnread(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hi", TS: 10, FromMe: false}))
	must(t, w.ingestReceipt(&Receipt{ChatJID: "1234567890@s.whatsapp.net", MsgIDs: []string{"m1"}, Read: false}))

	c, _ := chatByJID(t, w, "1234567890@s.whatsapp.net")
	if c.UnreadCount != 1 {
		t.Errorf("UnreadCount = %d, want 1 (delivered receipt must not clear unread)", c.UnreadCount)
	}
}

func TestIngestReactionStored(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "funny", TS: 10}))
	must(t, w.ingestReaction(&Reaction{
		ChatJID: "1234567890@s.whatsapp.net", MsgID: "m1",
		ReactorJID: "friend@s.whatsapp.net", Emoji: "👍", TS: 11,
	}))

	msgs, err := w.store.Messages("1234567890@s.whatsapp.net", 50)
	must(t, err)
	if got := msgs[0].Reactions["👍"]; got != "friend@s.whatsapp.net" {
		t.Errorf("got reactions %+v, want 👍 from friend", msgs[0].Reactions)
	}
}
