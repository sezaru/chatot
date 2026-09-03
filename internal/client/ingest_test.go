package client

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waSyncAction"
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

func TestIngestViewOnceAttachmentRoundTrips(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{
		ChatJID: "1234567890@s.whatsapp.net", ID: "m1", TS: 10,
		Attachment: &Attachment{Kind: "image", MimeType: "image/jpeg", ViewOnce: true},
	}))

	msgs, err := w.store.Messages("1234567890@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || msgs[0].Attachment == nil {
		t.Fatalf("got %+v, want one stored message with an attachment", msgs)
	}
	if !msgs[0].Attachment.ViewOnce {
		t.Fatal("ViewOnce = false, want true to survive the store round trip")
	}
	if msgs[0].Attachment.Viewed {
		t.Fatal("Viewed = true, want false before it's ever opened")
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

func TestIngestReceiptSetsMessageStatus(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hi", TS: 10, FromMe: true}))

	must(t, w.ingestReceipt(&Receipt{ChatJID: "1234567890@s.whatsapp.net", MsgIDs: []string{"m1"}, Status: MessageStatusDelivered}))

	msgs, err := w.store.Messages("1234567890@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || msgs[0].Status != MessageStatusDelivered {
		t.Fatalf("got %+v, want status=delivered", msgs)
	}

	must(t, w.ingestReceipt(&Receipt{ChatJID: "1234567890@s.whatsapp.net", MsgIDs: []string{"m1"}, Read: true, Status: MessageStatusRead}))

	msgs, err = w.store.Messages("1234567890@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || msgs[0].Status != MessageStatusRead {
		t.Fatalf("got %+v, want status=read", msgs)
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
	if got := msgs[0].Reactions["👍"]; len(got) != 1 || got[0] != "friend@s.whatsapp.net" {
		t.Errorf("got reactions %+v, want 👍 from friend", msgs[0].Reactions)
	}
}

func TestIngestRevokeMarksMessageDeleted(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestMessage(&Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "oops", TS: 10}))
	must(t, w.ingestRevoke(&Revoke{ChatJID: "1234567890@s.whatsapp.net", MsgID: "m1", TS: 11}))

	msgs, err := w.store.Messages("1234567890@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || !msgs[0].Deleted {
		t.Fatalf("got %+v, want the message marked deleted", msgs)
	}
}

func TestIngestGroupReadReceiptNeedsEveryMember(t *testing.T) {
	w := newIngestFixture(t)
	const group = "g1@g.us"
	must(t, w.ingestMessage(&Message{ID: "m1", ChatJID: group, FromMe: true, Text: "hi", TS: 100}))
	must(t, w.store.UpsertGroup(store.GroupRow{JID: group, Name: "G"}))
	must(t, w.store.SetGroupParticipants(group, []store.GroupParticipant{
		{JID: "111@s.whatsapp.net"}, {JID: "222@s.whatsapp.net"},
	}))
	status := func() int {
		msgs, err := w.store.Messages(group, 10)
		must(t, err)
		return msgs[0].Status
	}
	must(t, w.ingestReceipt(&Receipt{ChatJID: group, MsgIDs: []string{"m1"}, Read: true, Status: MessageStatusRead, ReaderJID: "111@s.whatsapp.net", TS: 101}))
	if got := status(); got != MessageStatusDelivered {
		t.Fatalf("after one of two readers status = %d, want delivered (%d)", got, MessageStatusDelivered)
	}
	must(t, w.ingestReceipt(&Receipt{ChatJID: group, MsgIDs: []string{"m1"}, Read: true, Status: MessageStatusRead, ReaderJID: "222@s.whatsapp.net", TS: 102}))
	if got := status(); got != MessageStatusRead {
		t.Fatalf("after every reader status = %d, want read (%d)", got, MessageStatusRead)
	}
}

func TestIngestGroupReadStateReconciledWhenMembersArrive(t *testing.T) {
	w := newIngestFixture(t)
	const group = "g2@g.us"
	must(t, w.ingestMessage(&Message{ID: "a", ChatJID: group, FromMe: true, Text: "1", TS: 100}))
	must(t, w.ingestMessage(&Message{ID: "b", ChatJID: group, FromMe: true, Text: "2", TS: 101}))
	// Receipts before the membership is known: nothing may be claimed read.
	must(t, w.ingestReceipt(&Receipt{ChatJID: group, MsgIDs: []string{"a", "b"}, Read: true, Status: MessageStatusRead, ReaderJID: "111@s.whatsapp.net", TS: 102}))
	msgs, err := w.store.Messages(group, 10)
	must(t, err)
	for _, m := range msgs {
		if m.Status != MessageStatusDelivered {
			t.Fatalf("%s before membership: status %d, want delivered", m.ID, m.Status)
		}
	}
	w.persistGroupInfo(&GroupInfo{JID: group, Name: "G", Participants: []GroupParticipant{{JID: "111@s.whatsapp.net"}}})
	msgs, err = w.store.Messages(group, 10)
	must(t, err)
	for _, m := range msgs {
		if m.Status != MessageStatusRead {
			t.Errorf("%s after membership: status %d, want read", m.ID, m.Status)
		}
	}
}

func TestIngestPeerReadReceiptKeepsOurUnread(t *testing.T) {
	w := newIngestFixture(t)
	const chat = "2@s.whatsapp.net"
	must(t, w.ingestMessage(&Message{ID: "A", ChatJID: chat, FromJID: chat, Text: "hi", TS: 10}))
	must(t, w.ingestMessage(&Message{ID: "B", ChatJID: chat, FromJID: "1@s.whatsapp.net", FromMe: true, Text: "yo", TS: 11}))
	if c, _ := chatByJID(t, w, chat); c.UnreadCount != 1 {
		t.Fatalf("unread before = %d, want 1", c.UnreadCount)
	}
	// They read our message: our badge is untouched.
	must(t, w.ingestReceipt(&Receipt{ChatJID: chat, MsgIDs: []string{"B"}, Read: true, Status: MessageStatusRead, ReaderJID: chat, TS: 12}))
	if c, _ := chatByJID(t, w, chat); c.UnreadCount != 1 {
		t.Fatalf("unread after peer read = %d, want 1", c.UnreadCount)
	}
	// Our other device read the chat: it clears.
	must(t, w.ingestReceipt(&Receipt{ChatJID: chat, MsgIDs: []string{"A"}, Read: true, Status: MessageStatusRead, TS: 13}))
	if c, _ := chatByJID(t, w, chat); c.UnreadCount != 0 {
		t.Fatalf("unread after self read = %d, want 0", c.UnreadCount)
	}
}

func TestApplyMarkChatAsReadHonoursRange(t *testing.T) {
	w := newIngestFixture(t)
	const chat = "2@s.whatsapp.net"
	must(t, w.ingestMessage(&Message{ID: "A", ChatJID: chat, FromJID: chat, Text: "hi", TS: 100}))
	read := true
	old := int64(50)
	must(t, w.applyMarkChatAsRead(chat, &waSyncAction.MarkChatAsReadAction{Read: &read, MessageRange: &waSyncAction.SyncActionMessageRange{LastMessageTimestamp: &old}}))
	if c, _ := chatByJID(t, w, chat); c.UnreadCount != 1 {
		t.Fatalf("a read from before the message cleared the badge (unread=%d)", c.UnreadCount)
	}
	now := int64(100)
	must(t, w.applyMarkChatAsRead(chat, &waSyncAction.MarkChatAsReadAction{Read: &read, MessageRange: &waSyncAction.SyncActionMessageRange{LastMessageTimestamp: &now}}))
	if c, _ := chatByJID(t, w, chat); c.UnreadCount != 0 {
		t.Fatalf("a read covering the message left unread=%d", c.UnreadCount)
	}
	unread := false
	must(t, w.applyMarkChatAsRead(chat, &waSyncAction.MarkChatAsReadAction{Read: &unread}))
	if c, _ := chatByJID(t, w, chat); c.UnreadCount != 1 {
		t.Fatalf("mark unread left unread=%d", c.UnreadCount)
	}
}
