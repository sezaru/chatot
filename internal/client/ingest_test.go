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

// TestIngestRepeatedMessageCountsUnreadOnce: a message delivered again (a
// sender's resend, or the live copy of one the history already stored)
// is one unread, not two.
func TestIngestRepeatedMessageCountsUnreadOnce(t *testing.T) {
	w := newIngestFixture(t)
	m := &Message{ChatJID: "1234567890@s.whatsapp.net", ID: "m1", Text: "hi", TS: 10}
	must(t, w.ingestMessage(m))
	must(t, w.ingestMessage(m))

	c, _ := chatByJID(t, w, "1234567890@s.whatsapp.net")
	if c.UnreadCount != 1 {
		t.Errorf("UnreadCount = %d, want 1 after the same message twice", c.UnreadCount)
	}
}

func TestIngestCallOfferLogsMissedCall(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestCall(&Call{ChatJID: "1234567890@s.whatsapp.net", CallerJID: "1234567890@s.whatsapp.net", CallID: "C1", Offer: true, TS: 500}))

	c, ok := chatByJID(t, w, "1234567890@s.whatsapp.net")
	if !ok {
		t.Fatal("chat not created for the call")
	}
	if c.UnreadCount != 1 || c.LastMessageTS != 500 || c.Preview != "📞 Missed voice call" {
		t.Fatalf("got unread=%d ts=%d preview=%q", c.UnreadCount, c.LastMessageTS, c.Preview)
	}
	rows, err := w.store.Messages("1234567890@s.whatsapp.net", 10)
	must(t, err)
	if len(rows) != 1 || rows[0].ID != "call:C1" || rows[0].Kind != "call" || rows[0].TS != 500 {
		t.Fatalf("rows = %+v, want one call row stamped with the call time", rows)
	}
	// The server replaying the same offer is not a second call.
	must(t, w.ingestCall(&Call{ChatJID: "1234567890@s.whatsapp.net", CallID: "C1", Offer: true, TS: 500}))
	c, _ = chatByJID(t, w, "1234567890@s.whatsapp.net")
	if c.UnreadCount != 1 {
		t.Fatalf("unread = %d after a replayed offer, want 1", c.UnreadCount)
	}
}

func TestIngestCallAcceptSettlesAnswered(t *testing.T) {
	w := newIngestFixture(t)
	jid := "1234567890@s.whatsapp.net"
	must(t, w.ingestCall(&Call{ChatJID: jid, CallID: "C1", Offer: true, TS: 500, Video: true}))
	must(t, w.ingestCall(&Call{ChatJID: jid, CallID: "C1", Outcome: CallAnswered, TS: 510}))

	c, _ := chatByJID(t, w, jid)
	if c.UnreadCount != 0 || c.Preview != "🎥 Video call" {
		t.Fatalf("got unread=%d preview=%q, want the answered call and no badge", c.UnreadCount, c.Preview)
	}
	// The store read directly: Messages() also wants a live session for
	// the own-JID poll marking.
	readCall := func() *CallLog {
		rows, err := w.store.Messages(jid, 10)
		must(t, err)
		if len(rows) != 1 {
			t.Fatalf("rows = %+v, want one", rows)
		}
		return messageFromStore(rows[0], "").CallLog
	}
	if cl := readCall(); cl == nil || cl.Outcome != CallAnswered || !cl.Video {
		t.Fatalf("call = %+v", cl)
	}
	// A late timeout can't turn an answered call back into a missed one.
	must(t, w.ingestCall(&Call{ChatJID: jid, CallID: "C1", Outcome: CallMissed, TS: 560}))
	if cl := readCall(); cl.Outcome != CallAnswered {
		t.Fatalf("outcome = %q after a late timeout, want answered", cl.Outcome)
	}
}

func TestIngestCallSignalWithoutOfferIsIgnored(t *testing.T) {
	w := newIngestFixture(t)
	must(t, w.ingestCall(&Call{ChatJID: "1234567890@s.whatsapp.net", CallID: "C9", Outcome: CallAnswered, TS: 5}))
	if _, ok := chatByJID(t, w, "1234567890@s.whatsapp.net"); ok {
		t.Fatal("an accept for a call never offered here must not create a chat")
	}
}

func TestIngestReactionOnOwnMessageDescribesTargetAndBumpsChat(t *testing.T) {
	w := newIngestFixture(t)
	jid := "1234567890@s.whatsapp.net"
	must(t, w.ingestMessage(&Message{ChatJID: jid, ID: "m1", Text: "see you at 7", TS: 10, FromMe: true}))
	r := &Reaction{ChatJID: jid, MsgID: "m1", ReactorJID: jid, Emoji: "👍", TS: 20}
	must(t, w.ingestReaction(r))

	if !r.TargetFromMe || r.TargetPreview != "see you at 7" {
		t.Fatalf("reaction not described: %+v", r)
	}
	c, _ := chatByJID(t, w, jid)
	if c.LastMessageTS != 20 || c.UnreadCount != 0 {
		t.Fatalf("got ts=%d unread=%d, want the chat bumped to the reaction and no badge", c.LastMessageTS, c.UnreadCount)
	}
	if c.LastReaction == nil || c.LastReaction.Emoji != "👍" {
		t.Fatalf("LastReaction = %+v", c.LastReaction)
	}
}

func TestIngestReactionOnOthersMessageIsQuiet(t *testing.T) {
	w := newIngestFixture(t)
	jid := "1234567890@s.whatsapp.net"
	must(t, w.ingestMessage(&Message{ChatJID: jid, ID: "m1", Text: "hi", TS: 10}))
	r := &Reaction{ChatJID: jid, MsgID: "m1", ReactorJID: jid, Emoji: "👍", TS: 20}
	must(t, w.ingestReaction(r))

	if r.TargetFromMe || r.TargetPreview != "" {
		t.Fatalf("a reaction to their own message is not ours: %+v", r)
	}
	c, _ := chatByJID(t, w, jid)
	if c.LastMessageTS != 10 {
		t.Fatalf("ts = %d, want the chat left where the message put it", c.LastMessageTS)
	}
}
