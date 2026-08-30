package store

import "testing"

func TestMessagesReturnsChronologicalOrder(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "first", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", Text: "second", TS: 2}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m3", Text: "third", TS: 3}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}
	if msgs[0].Text != "first" || msgs[1].Text != "second" || msgs[2].Text != "third" {
		t.Fatalf("got %+v, want chronological order", msgs)
	}
}

func TestMessagesOnlyReturnsRequestedChat(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertChat(ChatRow{JID: "b@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "for a", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "b@s.whatsapp.net", MsgID: "m2", Text: "for b", TS: 2}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || msgs[0].Text != "for a" {
		t.Fatalf("got %+v, want only chat a's message", msgs)
	}
}

func TestMessagesReplyContext(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "orig", Text: "original", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "reply", Text: "replying", TS: 2, ReplyToMsgID: "orig"}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[1].ReplyToMsgID != "orig" {
		t.Fatalf("got ReplyToMsgID = %q, want orig", msgs[1].ReplyToMsgID)
	}
}

func TestMessagesReactionsAttached(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "funny", TS: 1}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "friend@s.whatsapp.net", Emoji: "😂", TS: 2}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if got := msgs[0].Reactions["😂"]; got != "friend@s.whatsapp.net" {
		t.Fatalf("got reactions %+v, want 😂 from friend@s.whatsapp.net", msgs[0].Reactions)
	}
}

func TestMessagesReactionClearedByEmptyEmoji(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "funny", TS: 1}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "friend@s.whatsapp.net", Emoji: "😂", TS: 2}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "friend@s.whatsapp.net", Emoji: "", TS: 3}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs[0].Reactions) != 0 {
		t.Fatalf("got reactions %+v, want none after clearing", msgs[0].Reactions)
	}
}

func TestMessagesMediaAttachment(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "image", Caption: "nice", MimeType: "image/jpeg"}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if msgs[0].Attachment == nil {
		t.Fatal("Attachment is nil, want populated")
	}
	if msgs[0].Attachment.Kind != "image" || msgs[0].Attachment.Caption != "nice" || msgs[0].Attachment.MimeType != "image/jpeg" {
		t.Fatalf("got %+v, want image/nice/image-jpeg", msgs[0].Attachment)
	}
}

func TestMessagesBeforePagesOlderInChronologicalOrder(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	for i, id := range []string{"m1", "m2", "m3", "m4", "m5"} {
		must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: id, TS: int64(i + 1)}))
	}

	// The newest page (limit 2) is [m4 m5]; paging before m4 yields the next
	// two older, chronological.
	older, err := s.MessagesBefore("a@s.whatsapp.net", "m4", 2)
	must(t, err)
	if len(older) != 2 || older[0].ID != "m2" || older[1].ID != "m3" {
		t.Fatalf("got %+v, want [m2 m3]", older)
	}
}

func TestMessagesBeforeBreaksTiesBySameTimestamp(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	// All same second: the rowid cursor must still page without skips/dupes.
	for _, id := range []string{"m1", "m2", "m3"} {
		must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: id, TS: 100}))
	}
	older, err := s.MessagesBefore("a@s.whatsapp.net", "m3", 10)
	must(t, err)
	if len(older) != 2 || older[0].ID != "m1" || older[1].ID != "m2" {
		t.Fatalf("got %+v, want [m1 m2]", older)
	}
}

func TestMessagesBeforeUnknownCursorReturnsNothing(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))

	older, err := s.MessagesBefore("a@s.whatsapp.net", "nope", 10)
	must(t, err)
	if len(older) != 0 {
		t.Fatalf("got %+v, want none for unknown cursor", older)
	}
}

func TestMessagesRespectsLimit(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	for i, id := range []string{"m1", "m2", "m3"} {
		must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: id, TS: int64(i + 1)}))
	}
	msgs, err := s.Messages("a@s.whatsapp.net", 2)
	must(t, err)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	// newest-window: with limit 2 we keep m2, m3, in chronological order.
	if msgs[0].ID != "m2" || msgs[1].ID != "m3" {
		t.Fatalf("got %+v, want [m2 m3]", msgs)
	}
}

func TestUpsertMessageEditUpdatesTextAndFlag(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "original", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "edited", TS: 2, Edited: true}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (edit updates in place)", len(msgs))
	}
	if msgs[0].Text != "edited" || !msgs[0].Edited {
		t.Fatalf("got text=%q edited=%v, want edited text + flag", msgs[0].Text, msgs[0].Edited)
	}
}

func TestUpsertMessageEditedIsSticky(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "original", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "edited", TS: 2, Edited: true}))
	// A later non-edit re-upsert (e.g. a history redelivery) must not clear it.
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "edited", TS: 2, Edited: false}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if !msgs[0].Edited {
		t.Fatal("edited flag was cleared by a non-edit re-upsert; want sticky")
	}
}

func TestMarkMessageDeletedInsertsStubForUnseenMessage(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	// A revoke can arrive before (or without) the original message.
	must(t, s.MarkMessageDeleted("a@s.whatsapp.net", "m1", 5))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || !msgs[0].Deleted {
		t.Fatalf("got %+v, want one deleted stub row", msgs)
	}
}

func TestMarkMessageDeletedMarksExistingRow(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "original", TS: 1}))
	must(t, s.MarkMessageDeleted("a@s.whatsapp.net", "m1", 5))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || !msgs[0].Deleted || msgs[0].Text != "original" {
		t.Fatalf("got %+v, want the original row marked deleted", msgs)
	}
}

func TestUpsertMessageDeletedIsSticky(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "original", TS: 1}))
	must(t, s.MarkMessageDeleted("a@s.whatsapp.net", "m1", 2))
	// A later redelivery of the original must not resurrect it.
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "original", TS: 1}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if !msgs[0].Deleted {
		t.Fatal("deleted flag was cleared by a later redelivery of the original; want sticky")
	}
}

func TestSetMessagesStatusAdvances(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 1, FromMe: true}))

	must(t, s.SetMessagesStatus("a@s.whatsapp.net", []string{"m1"}, 1))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if msgs[0].Status != 1 {
		t.Fatalf("got Status = %d, want 1 (delivered)", msgs[0].Status)
	}
}

func TestSetMessagesStatusIsMonotonic(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 1, FromMe: true}))
	must(t, s.SetMessagesStatus("a@s.whatsapp.net", []string{"m1"}, 2)) // read

	// A late delivered receipt for the same message must not downgrade it.
	must(t, s.SetMessagesStatus("a@s.whatsapp.net", []string{"m1"}, 1))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if msgs[0].Status != 2 {
		t.Fatalf("got Status = %d, want 2 (read status must stick despite a later delivered receipt)", msgs[0].Status)
	}
}
