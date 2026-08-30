package store

import "testing"

func TestResolveChatNamePushNameOnly(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "1234567890@s.whatsapp.net"}))
	must(t, s.UpsertContact(ContactRow{JID: "1234567890@s.whatsapp.net", PushName: "Alice"}))

	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].Name != "Alice" {
		t.Fatalf("got %+v, want a single chat named Alice", chats)
	}
}

func TestResolveChatNamePhoneFallback(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "1234567890@s.whatsapp.net"}))
	// No contact row at all: unsaved number.

	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].Name != "+1234567890" {
		t.Fatalf("got %+v, want a single chat named +1234567890", chats)
	}
}

func TestResolveChatNameGroup(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "120363000000000000@g.us", IsGroup: true}))
	must(t, s.UpsertGroup(GroupRow{JID: "120363000000000000@g.us", Name: "Weekend Trip"}))

	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].Name != "Weekend Trip" {
		t.Fatalf("got %+v, want a single chat named Weekend Trip", chats)
	}
}

func TestResolveChatNameBusinessBeatsFullBeatsPush(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "1112223333@s.whatsapp.net"}))
	must(t, s.UpsertContact(ContactRow{
		JID: "1112223333@s.whatsapp.net", PushName: "push", FullName: "full", BusinessName: "Acme Corp",
	}))

	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].Name != "Acme Corp" {
		t.Fatalf("got %+v, want business name to win", chats)
	}

	// Drop the business name (empty update leaves other fields alone per the
	// upsert contract, so overwrite directly to simulate it disappearing).
	_, err := s.db.Exec(`UPDATE contacts SET business_name = NULL WHERE jid = ?`, "1112223333@s.whatsapp.net")
	must(t, err)
	chats = mustChats(t, s)
	if chats[0].Name != "full" {
		t.Fatalf("got %q, want full name to win once business name is gone", chats[0].Name)
	}
}

func TestChatsExplicitChatNameWinsOverEverything(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "1234567890@s.whatsapp.net", Name: "My Nickname"}))
	must(t, s.UpsertContact(ContactRow{JID: "1234567890@s.whatsapp.net", PushName: "Alice"}))

	chats := mustChats(t, s)
	if chats[0].Name != "My Nickname" {
		t.Fatalf("got %q, want explicit chat name to win", chats[0].Name)
	}
}

func TestChatsOrderingPinnedFloatsAboveNewerUnpinned(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A-unpinned-new", LastMessageTS: 200}))
	must(t, s.UpsertChat(ChatRow{JID: "b@s.whatsapp.net", Name: "B-pinned-old", Pinned: true, LastMessageTS: 100}))

	chats := mustChats(t, s)
	if len(chats) != 2 || chats[0].JID != "b@s.whatsapp.net" {
		t.Fatalf("got %+v, want pinned chat first despite older timestamp", chats)
	}
}

func TestChatsOrderingNewerUnpinnedFirst(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "old@s.whatsapp.net", Name: "Old", LastMessageTS: 100}))
	must(t, s.UpsertChat(ChatRow{JID: "new@s.whatsapp.net", Name: "New", LastMessageTS: 200}))

	chats := mustChats(t, s)
	if chats[0].JID != "new@s.whatsapp.net" || chats[1].JID != "old@s.whatsapp.net" {
		t.Fatalf("got %+v, want newer last_message_ts first", chats)
	}
}

func TestChatsOrderingNameTiebreakCaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "z@s.whatsapp.net", Name: "bob", LastMessageTS: 100}))
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "Alice", LastMessageTS: 100}))

	chats := mustChats(t, s)
	if chats[0].Name != "Alice" || chats[1].Name != "bob" {
		t.Fatalf("got %+v, want case-insensitive name tiebreak (Alice, bob)", chats)
	}
}

func TestChatsPreviewMediaWithCaption(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "image", Caption: "Sunset!"}))

	chats := mustChats(t, s)
	if chats[0].Preview != "Sunset!" {
		t.Fatalf("got %q, want caption as preview", chats[0].Preview)
	}
}

func TestChatsPreviewMediaFilenameFallback(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "document", Filename: "invoice.pdf"}))

	chats := mustChats(t, s)
	if chats[0].Preview != "invoice.pdf" {
		t.Fatalf("got %q, want filename as preview", chats[0].Preview)
	}
}

func TestChatsPreviewMediaKindPlaceholder(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "sticker"}))

	chats := mustChats(t, s)
	if chats[0].Preview != "[sticker]" {
		t.Fatalf("got %q, want [sticker] placeholder", chats[0].Preview)
	}
}

func TestChatsPreviewLocation(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1, Kind: "location", Payload: `{"lat":1,"long":2}`}))

	chats := mustChats(t, s)
	if chats[0].Preview != "📍 Location" {
		t.Fatalf("got %q, want location preview", chats[0].Preview)
	}
}

func TestMessagesRoundTripKindPayload(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1, Kind: "location", Payload: `{"lat":1.5,"long":2.5}`}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 || msgs[0].Kind != "location" || msgs[0].Payload != `{"lat":1.5,"long":2.5}` {
		t.Fatalf("got %+v, want kind/payload preserved", msgs)
	}
}

func TestChatsPreviewTextFromMePrefixed(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "on my way", FromMe: true, TS: 1}))

	chats := mustChats(t, s)
	if chats[0].Preview != "You: on my way" {
		t.Fatalf("got %q, want from-me prefixed text preview", chats[0].Preview)
	}
}

func TestChatsPreviewPlainTextNotFromMe(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hey there", TS: 1}))

	chats := mustChats(t, s)
	if chats[0].Preview != "hey there" {
		t.Fatalf("got %q, want plain text preview", chats[0].Preview)
	}
}

func TestChatsFilterDropsCommunityParent(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "parent@g.us", IsGroup: true, Name: "Community"}))
	must(t, s.UpsertGroup(GroupRow{JID: "parent@g.us", Name: "Community", IsParent: true}))
	must(t, s.UpsertChat(ChatRow{JID: "normal@g.us", IsGroup: true, Name: "Regular Group"}))
	must(t, s.UpsertGroup(GroupRow{JID: "normal@g.us", Name: "Regular Group"}))

	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].JID != "normal@g.us" {
		t.Fatalf("got %+v, want only the regular group", chats)
	}
}

func TestChatsFilterDropsLinkedAnnouncementChannel(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "announce@g.us", IsGroup: true, Name: "Announcements"}))
	must(t, s.UpsertGroup(GroupRow{JID: "announce@g.us", Name: "Announcements", LinkedParentJID: "parent@g.us"}))
	must(t, s.UpsertChat(ChatRow{JID: "dm@s.whatsapp.net", Name: "Friend"}))

	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].JID != "dm@s.whatsapp.net" {
		t.Fatalf("got %+v, want only the DM (linked channel dropped)", chats)
	}
}

func TestChatsFilterKeepsDMsAndNormalGroups(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "dm@s.whatsapp.net", Name: "Friend"}))
	must(t, s.UpsertChat(ChatRow{JID: "group@g.us", IsGroup: true, Name: "Group"}))
	must(t, s.UpsertGroup(GroupRow{JID: "group@g.us", Name: "Group"}))

	chats := mustChats(t, s)
	if len(chats) != 2 {
		t.Fatalf("got %+v, want both DM and normal group kept", chats)
	}
}

func TestChatsRespectsLimit(t *testing.T) {
	s := newTestStore(t)
	for _, jid := range []string{"a@s.whatsapp.net", "b@s.whatsapp.net", "c@s.whatsapp.net"} {
		must(t, s.UpsertChat(ChatRow{JID: jid, Name: jid}))
	}
	chats, err := s.Chats(2)
	must(t, err)
	if len(chats) != 2 {
		t.Fatalf("got %d chats, want 2", len(chats))
	}
}

func mustChats(t *testing.T, s *Store) []Chat {
	t.Helper()
	chats, err := s.Chats(50)
	must(t, err)
	return chats
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
