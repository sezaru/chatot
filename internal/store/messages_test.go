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
	if got := msgs[0].Reactions["😂"]; len(got) != 1 || got[0] != "friend@s.whatsapp.net" {
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

func TestMessagesMediaGifFlagRoundTrips(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "video", MimeType: "video/mp4", IsGif: true}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if msgs[0].Attachment == nil || !msgs[0].Attachment.IsGif {
		t.Fatalf("got %+v, want IsGif=true", msgs[0].Attachment)
	}

	row, ok, err := s.MediaByMsgID("m1")
	must(t, err)
	if !ok || !row.IsGif {
		t.Fatalf("MediaByMsgID = %+v ok=%v, want IsGif=true", row, ok)
	}
}

func TestMessagesMediaViewOnceRoundTripsAndSetMediaViewedIsSticky(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "image", MimeType: "image/jpeg", ViewOnce: true}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if msgs[0].Attachment == nil || !msgs[0].Attachment.ViewOnce || msgs[0].Attachment.Viewed {
		t.Fatalf("got %+v, want ViewOnce=true Viewed=false", msgs[0].Attachment)
	}

	must(t, s.SetMediaViewed("a@s.whatsapp.net", "m1"))

	msgs, err = s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if !msgs[0].Attachment.Viewed {
		t.Fatalf("got %+v, want Viewed=true after SetMediaViewed", msgs[0].Attachment)
	}

	row, ok, err := s.MediaByMsgID("m1")
	must(t, err)
	if !ok || !row.ViewOnce || !row.Viewed {
		t.Fatalf("MediaByMsgID = %+v ok=%v, want ViewOnce=true Viewed=true", row, ok)
	}

	// A re-upsert (e.g. a retried decrypt refreshing the row) must not
	// clobber the sticky viewed flag back to unset.
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "image", MimeType: "image/jpeg", ViewOnce: true}))
	row, ok, err = s.MediaByMsgID("m1")
	must(t, err)
	if !ok || !row.Viewed {
		t.Fatalf("MediaByMsgID after re-upsert = %+v ok=%v, want Viewed still true", row, ok)
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

func TestUpsertMessageForwardedRoundTrips(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "fyi", TS: 1, Forwarded: true}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || !msgs[0].Forwarded {
		t.Fatalf("got %+v, want the forwarded flag to round-trip", msgs)
	}
}

func TestUpsertMessageForwardedIsSticky(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "fyi", TS: 1, Forwarded: true}))
	// A later non-forwarded re-upsert (e.g. a history redelivery) must not clear it.
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "fyi", TS: 1, Forwarded: false}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if !msgs[0].Forwarded {
		t.Fatal("forwarded flag was cleared by a non-forwarded re-upsert; want sticky")
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

func TestSetMessageStarredPersistsAndClears(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 1}))

	must(t, s.SetMessageStarred("a@s.whatsapp.net", "m1", true))
	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if !msgs[0].Starred {
		t.Fatal("got Starred = false, want true")
	}

	must(t, s.SetMessageStarred("a@s.whatsapp.net", "m1", false))
	msgs, err = s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if msgs[0].Starred {
		t.Fatal("got Starred = true, want false after unstar")
	}
}

func TestUpsertMessageDoesNotClobberStarred(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 1}))
	must(t, s.SetMessageStarred("a@s.whatsapp.net", "m1", true))

	// A re-delivery of the same message (e.g. a receipt-driven re-upsert)
	// must not unstar it.
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 1}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if !msgs[0].Starred {
		t.Fatal("got Starred = false after re-upsert, want it to stick")
	}
}

func TestStarredMessagesReturnsOnlyStarredAcrossChatsNewestFirst(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertChat(ChatRow{JID: "b@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "old starred", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", Text: "not starred", TS: 2}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "b@s.whatsapp.net", MsgID: "m3", Text: "new starred", TS: 3}))
	must(t, s.SetMessageStarred("a@s.whatsapp.net", "m1", true))
	must(t, s.SetMessageStarred("b@s.whatsapp.net", "m3", true))

	starred, err := s.StarredMessages(50)
	must(t, err)
	if len(starred) != 2 {
		t.Fatalf("got %d starred messages, want 2: %+v", len(starred), starred)
	}
	if starred[0].ID != "m3" || starred[0].ChatJID != "b@s.whatsapp.net" {
		t.Fatalf("got starred[0] = %+v, want newest (m3 in chat b) first", starred[0])
	}
	if starred[1].ID != "m1" || starred[1].ChatJID != "a@s.whatsapp.net" {
		t.Fatalf("got starred[1] = %+v, want m1 in chat a", starred[1])
	}
	for _, m := range starred {
		if !m.Starred {
			t.Fatalf("got Starred = false on returned row %+v, want true", m)
		}
	}
}

func TestClearChatDeletesOnlyTargetChat(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertChat(ChatRow{JID: "b@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "for a", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "b@s.whatsapp.net", MsgID: "m2", Text: "for b", TS: 2}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "x", Emoji: "👍"}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "b@s.whatsapp.net", MsgID: "m2", ReactorJID: "x", Emoji: "👍"}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "image", LocalPath: "/tmp/a.jpg"}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "b@s.whatsapp.net", MsgID: "m2", Kind: "image", LocalPath: "/tmp/b.jpg"}))

	if _, err := s.ClearChat("a@s.whatsapp.net", false); err != nil {
		t.Fatalf("ClearChat: %v", err)
	}

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 0 {
		t.Fatalf("got %d messages left in cleared chat, want 0", len(msgs))
	}
	other, err := s.Messages("b@s.whatsapp.net", 50)
	must(t, err)
	if len(other) != 1 || other[0].ID != "m2" {
		t.Fatalf("got %+v, want chat b untouched", other)
	}
	if len(other[0].Reactions) != 1 {
		t.Fatalf("got %+v, want chat b's reaction untouched", other[0])
	}
	media, err := s.ChatMedia("b@s.whatsapp.net")
	must(t, err)
	if len(media) != 1 {
		t.Fatalf("got %+v, want chat b's media untouched", media)
	}
}

func TestClearChatAlsoMediaReturnsLocalPaths(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", TS: 2}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "image", LocalPath: "/tmp/downloaded.jpg"}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", Kind: "document"})) // never downloaded

	paths, err := s.ClearChat("a@s.whatsapp.net", true)
	if err != nil {
		t.Fatalf("ClearChat: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/tmp/downloaded.jpg" {
		t.Fatalf("got paths %+v, want just the one downloaded local_path", paths)
	}

	media, err := s.ChatMedia("a@s.whatsapp.net")
	must(t, err)
	if len(media) != 0 {
		t.Fatalf("got %+v media rows left, want 0", media)
	}
}

func TestClearChatWithoutAlsoMediaReturnsNoPaths(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "image", LocalPath: "/tmp/downloaded.jpg"}))

	paths, err := s.ClearChat("a@s.whatsapp.net", false)
	if err != nil {
		t.Fatalf("ClearChat: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("got paths %+v, want none when alsoMedia is false", paths)
	}
}

func TestPruneEmptyMessagesKeepsRenderableRows(t *testing.T) {
	s := newTestStore(t)
	chat := "1@s.whatsapp.net"
	rows := []MessageRow{
		{ChatJID: chat, MsgID: "blank", TS: 1},
		{ChatJID: chat, MsgID: "text", Text: "hi", TS: 2},
		{ChatJID: chat, MsgID: "gone", Deleted: true, TS: 3},
		{ChatJID: chat, MsgID: "loc", Kind: "location", Payload: "{}", TS: 4},
		{ChatJID: chat, MsgID: "pic", TS: 5},
	}
	for _, r := range rows {
		if err := s.UpsertMessage(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertMedia(MediaRow{ChatJID: chat, MsgID: "pic", Kind: "image"}); err != nil {
		t.Fatal(err)
	}
	if err := pruneEmptyMessages(s.db); err != nil {
		t.Fatal(err)
	}
	msgs, err := s.Messages(chat, 10)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	if len(ids) != 4 || ids[0] == "blank" {
		t.Fatalf("after prune: %v, want text/gone/loc/pic only", ids)
	}
	for _, id := range ids {
		if id == "blank" {
			t.Fatalf("blank row survived: %v", ids)
		}
	}
}

func TestRemoveMessageDropsRowAndAttachments(t *testing.T) {
	s := newTestStore(t)
	const chat = "1@s.whatsapp.net"
	must(t, s.UpsertMessage(MessageRow{ChatJID: chat, MsgID: "A", Text: "gone", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: chat, MsgID: "B", Text: "stays", TS: 2}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: chat, MsgID: "A", ReactorJID: "2@lid", Emoji: "👍", TS: 1}))
	must(t, s.UpsertReadReceipt(chat, "A", "2@lid", 3))
	must(t, s.RemoveMessage(chat, "A"))
	if _, ok, _ := s.MessageByID(chat, "A"); ok {
		t.Fatal("A still stored")
	}
	if _, ok, _ := s.MessageByID(chat, "B"); !ok {
		t.Fatal("B removed too")
	}
	var n int
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM reactions WHERE msg_id = 'A'`).Scan(&n))
	if n != 0 {
		t.Fatalf("%d reactions left for A", n)
	}
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM read_receipts WHERE msg_id = 'A'`).Scan(&n))
	if n != 0 {
		t.Fatalf("%d receipts left for A", n)
	}
}
