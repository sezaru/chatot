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
	if chats[0].Preview != "📷 Sunset!" {
		t.Fatalf("got %q, want caption as preview", chats[0].Preview)
	}
}

func TestChatsPreviewMediaFilenameFallback(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "document", Filename: "invoice.pdf"}))

	chats := mustChats(t, s)
	if chats[0].Preview != "📄 invoice.pdf" {
		t.Fatalf("got %q, want filename as preview", chats[0].Preview)
	}
}

func TestChatsPreviewMediaKindPlaceholder(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "document"}))

	chats := mustChats(t, s)
	if chats[0].Preview != "📄 Document" {
		t.Fatalf("got %q, want document placeholder", chats[0].Preview)
	}
}

func TestChatsPreviewStickerKind(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "sticker"}))

	chats := mustChats(t, s)
	if chats[0].Preview != "🙂 Sticker" {
		t.Fatalf("got %q, want sticker preview", chats[0].Preview)
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

func TestChatsKeepsLinkedAnnouncementGroup(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "announce@g.us", IsGroup: true, Name: "Announcements"}))
	must(t, s.UpsertGroup(GroupRow{JID: "announce@g.us", Name: "Announcements", LinkedParentJID: "parent@g.us"}))
	must(t, s.UpsertChat(ChatRow{JID: "dm@s.whatsapp.net", Name: "Friend"}))

	chats := mustChats(t, s)
	if len(chats) != 2 {
		t.Fatalf("got %+v, want the DM and the community's announcement group", chats)
	}
	for _, c := range chats {
		if c.JID == "announce@g.us" && c.Name != "Announcements" {
			t.Fatalf("announcement group named %q, want Announcements", c.Name)
		}
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
	all, err := s.Chats(0)
	must(t, err)
	if len(all) != 3 {
		t.Fatalf("Chats(0) gave %d chats, want all 3", len(all))
	}
}

func TestSetChatPinnedMutedArchivedPersist(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))

	must(t, s.SetChatPinned("a@s.whatsapp.net", true))
	must(t, s.SetChatMuted("a@s.whatsapp.net", true))
	must(t, s.SetChatArchived("a@s.whatsapp.net", true))

	chats := mustChats(t, s)
	if !chats[0].Pinned || !chats[0].Muted || !chats[0].Archived {
		t.Fatalf("got %+v, want pinned/muted/archived all true", chats[0])
	}

	must(t, s.SetChatPinned("a@s.whatsapp.net", false))
	must(t, s.SetChatMuted("a@s.whatsapp.net", false))
	must(t, s.SetChatArchived("a@s.whatsapp.net", false))

	chats = mustChats(t, s)
	if chats[0].Pinned || chats[0].Muted || chats[0].Archived {
		t.Fatalf("got %+v, want pinned/muted/archived all false", chats[0])
	}
}

func TestSetChatUnreadSetsAndClears(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))

	must(t, s.SetChatUnread("a@s.whatsapp.net", true))
	chats := mustChats(t, s)
	if chats[0].UnreadCount < 1 {
		t.Fatalf("got unread_count %d, want >= 1", chats[0].UnreadCount)
	}

	must(t, s.SetChatUnread("a@s.whatsapp.net", false))
	chats = mustChats(t, s)
	if chats[0].UnreadCount != 0 {
		t.Fatalf("got unread_count %d, want 0", chats[0].UnreadCount)
	}
}

func TestSetChatUnreadDoesNotClobberExistingCount(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A", UnreadCount: 5}))

	must(t, s.SetChatUnread("a@s.whatsapp.net", true))
	chats := mustChats(t, s)
	if chats[0].UnreadCount != 5 {
		t.Fatalf("got unread_count %d, want unchanged 5", chats[0].UnreadCount)
	}
}

func TestChatsExcludesStatusBroadcast(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "dm@s.whatsapp.net", Name: "Friend"}))
	// A status message goes through normal ingest, creating a status@broadcast
	// chat row — which must not surface as a conversation.
	must(t, s.BumpChatActivity("status@broadcast", false, 100, 1))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "status@broadcast", MsgID: "s1", FromJID: "friend@s.whatsapp.net", Text: "my status", TS: 100}))

	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].JID != "dm@s.whatsapp.net" {
		t.Fatalf("got %+v, want only the DM (status@broadcast hidden)", chats)
	}
}

func TestStatusesReturnsOnlyBroadcastNewestFirst(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "dm@s.whatsapp.net", Name: "Friend"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "dm@s.whatsapp.net", MsgID: "n1", Text: "normal", TS: 50}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "status@broadcast", MsgID: "s1", FromJID: "a@s.whatsapp.net", Text: "first", TS: 100}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "status@broadcast", MsgID: "s2", FromJID: "b@s.whatsapp.net", Text: "second", TS: 200}))

	statuses, err := s.Statuses(0, 50)
	must(t, err)
	if len(statuses) != 2 {
		t.Fatalf("got %d statuses, want 2 (normal-chat message excluded)", len(statuses))
	}
	if statuses[0].ID != "s2" || statuses[1].ID != "s1" {
		t.Fatalf("got %+v, want newest-first (s2, s1)", statuses)
	}
	if statuses[0].FromJID != "b@s.whatsapp.net" {
		t.Fatalf("got poster %q, want b@s.whatsapp.net (FromJID = poster)", statuses[0].FromJID)
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

func TestResolveChatNameLIDFallsBackToPhone(t *testing.T) {
	if got := resolveChatName("", "", "", "", "", "", "5511999@s.whatsapp.net", "123456789012345@lid"); got != "+5511999" {
		t.Fatalf("LID chat with known PN: got %q, want +5511999", got)
	}
	// A LID with no known phone number is not a dialable number: fall back
	// to the raw JID rather than pretending it is one.
	if got := resolveChatName("", "", "", "", "", "", "", "123456789012345@lid"); got != "123456789012345@lid" {
		t.Fatalf("LID chat without PN: got %q", got)
	}
	if got := resolveChatName("", "", "", "Ada", "", "", "5511999@s.whatsapp.net", "123456789012345@lid"); got != "Ada" {
		t.Fatalf("names win over the number: got %q", got)
	}
}

func TestChatsResolveLIDContactViaPNJID(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertChat(ChatRow{JID: "123456789012345@lid", LastMessageTS: 10}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertContact(ContactRow{JID: "123456789012345@lid", PNJID: "5511999@s.whatsapp.net"}); err != nil {
		t.Fatal(err)
	}
	chats, err := s.Chats(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 1 || chats[0].Name != "+5511999" {
		t.Fatalf("got %+v, want one chat named +5511999", chats)
	}
	// A later push name takes over, and an empty PNJID leaves the mapping.
	if err := s.UpsertContact(ContactRow{JID: "123456789012345@lid", PushName: "Ada"}); err != nil {
		t.Fatal(err)
	}
	chats, _ = s.Chats(10)
	if chats[0].Name != "Ada" {
		t.Fatalf("after push name: %q", chats[0].Name)
	}
}

func TestBuildPreviewVocabulary(t *testing.T) {
	cases := []struct {
		in   previewInput
		want string
	}{
		{previewInput{MediaKind: "audio", MediaSeconds: 6}, "🎤 0:06"},
		{previewInput{MediaKind: "audio"}, "🎤 Voice message"},
		{previewInput{MediaKind: "video", MediaCaption: "clip"}, "🎥 clip"},
		{previewInput{MediaKind: "video", MediaIsGIF: true}, "🎞 GIF"},
		{previewInput{FromMe: true, MediaKind: "image"}, "You: 📷 Photo"},
		{previewInput{Kind: "location", Payload: `{"name":"Estação","lat":1,"long":2}`}, "📍 Estação"},
		{previewInput{Kind: "location", Payload: `{"lat":1,"long":2,"live":true}`}, "📍 Live location"},
		{previewInput{Kind: "contact", Payload: `{"name":"Nina Costa"}`}, "👤 Nina Costa"},
		{previewInput{Kind: "poll", Payload: `{"name":"When?","options":["a"]}`}, "📊 When?"},
		{previewInput{Text: "hi"}, "hi"},
	}
	for _, c := range cases {
		if got := buildPreview(c.in); got != c.want {
			t.Errorf("buildPreview(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMergeChatFoldsLIDChatIntoNumber(t *testing.T) {
	s := newTestStore(t)
	const lid, pn = "64081113427987@lid", "554888073648@s.whatsapp.net"
	must(t, s.UpsertChat(ChatRow{JID: lid, Pinned: true, UnreadCount: 2, LastMessageTS: 200}))
	must(t, s.UpsertChat(ChatRow{JID: pn, UnreadCount: 1, LastMessageTS: 100}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: lid, MsgID: "A", Text: "from the phone", TS: 200}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: lid, MsgID: "SHARED", Text: "lid copy", TS: 150}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: pn, MsgID: "SHARED", Text: "pn copy", TS: 150}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: pn, MsgID: "B", Text: "from here", TS: 100}))
	must(t, s.SetChatLabel("7", lid, true))
	// The same reactor and reader under both chats: one row survives.
	must(t, s.UpsertReaction(ReactionRow{ChatJID: lid, MsgID: "SHARED", ReactorJID: "2@lid", Emoji: "👍", TS: 1}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: pn, MsgID: "SHARED", ReactorJID: "2@lid", Emoji: "❤", TS: 2}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: lid, MsgID: "A", ReactorJID: "3@lid", Emoji: "😂", TS: 3}))
	must(t, s.UpsertReadReceipt(lid, "SHARED", "2@lid", 5))
	must(t, s.UpsertReadReceipt(pn, "SHARED", "2@lid", 6))

	merged, err := s.MergeChat(lid, pn)
	must(t, err)
	if !merged {
		t.Fatal("MergeChat reported nothing to merge")
	}
	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].JID != pn {
		t.Fatalf("chats after merge = %+v, want only %s", chats, pn)
	}
	// The lid row was active last (ts 200): its unread count is the live one.
	if c := chats[0]; !c.Pinned || c.UnreadCount != 2 || c.LastMessageTS != 200 {
		t.Fatalf("merged chat = %+v, want pinned, 2 unread, ts 200", c)
	}
	msgs, err := s.Messages(pn, 10)
	must(t, err)
	if len(msgs) != 3 {
		t.Fatalf("messages under %s = %d, want 3 (shared id kept once)", pn, len(msgs))
	}
	if m, ok, _ := s.MessageByID(pn, "SHARED"); !ok || m.Text != "pn copy" {
		t.Fatalf("shared id = %+v, want the number chat's own copy kept", m)
	}
	if left, _ := s.Messages(lid, 10); len(left) != 0 {
		t.Fatalf("%d messages still under the lid chat", len(left))
	}
	labels, err := s.LabelsForChat(pn)
	must(t, err)
	if len(labels) != 1 || labels[0] != "7" {
		t.Fatalf("labels under %s = %v, want [7]", pn, labels)
	}
	if lids, _ := s.LIDChatJIDs(); len(lids) != 0 {
		t.Fatalf("LIDChatJIDs = %v, want none", lids)
	}
	var reactions, receipts int
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM reactions WHERE chat_jid = ?`, pn).Scan(&reactions))
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM read_receipts WHERE chat_jid = ?`, pn).Scan(&receipts))
	if reactions != 2 || receipts != 1 {
		t.Fatalf("reactions=%d receipts=%d under %s, want 2 and 1", reactions, receipts, pn)
	}
	var stray int
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM reactions WHERE chat_jid = ?`, lid).Scan(&stray))
	if stray != 0 {
		t.Fatalf("%d reactions still under the lid chat", stray)
	}
}

func TestMergeChatRenamesWhenTargetMissing(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "1@lid", Name: "Ada", UnreadCount: 4, LastMessageTS: 9}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "1@lid", MsgID: "A", Text: "hi", TS: 9}))
	merged, err := s.MergeChat("1@lid", "1@s.whatsapp.net")
	must(t, err)
	if !merged {
		t.Fatal("MergeChat reported nothing to merge")
	}
	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].JID != "1@s.whatsapp.net" || chats[0].Name != "Ada" || chats[0].UnreadCount != 4 {
		t.Fatalf("chats = %+v, want the row renamed intact", chats)
	}
	if merged, _ := s.MergeChat("1@lid", "1@s.whatsapp.net"); merged {
		t.Fatal("second merge reported work")
	}
}

func TestRepairUnreadCountsCapsAtUnansweredInbound(t *testing.T) {
	s := newTestStore(t)
	// Replied after everything: nothing can be unread.
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", UnreadCount: 395, LastMessageTS: 30}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "1", TS: 10}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "2", TS: 20}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "3", FromMe: true, TS: 30}))
	// Two inbound after the last reply: a stale 99 caps at 2, a real 1 stays.
	must(t, s.UpsertChat(ChatRow{JID: "b@s.whatsapp.net", UnreadCount: 99, LastMessageTS: 30}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "b@s.whatsapp.net", MsgID: "1", FromMe: true, TS: 10}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "b@s.whatsapp.net", MsgID: "2", TS: 20}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "b@s.whatsapp.net", MsgID: "3", TS: 30}))
	must(t, s.UpsertChat(ChatRow{JID: "c@s.whatsapp.net", UnreadCount: 1, LastMessageTS: 30}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "c@s.whatsapp.net", MsgID: "1", TS: 20}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "c@s.whatsapp.net", MsgID: "2", TS: 30}))
	// No stored thread: left alone.
	must(t, s.UpsertChat(ChatRow{JID: "d@s.whatsapp.net", UnreadCount: 7, LastMessageTS: 30}))
	// Marked unread by hand after replying: exactly 1, left alone.
	must(t, s.UpsertChat(ChatRow{JID: "e@s.whatsapp.net", LastMessageTS: 30}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "e@s.whatsapp.net", MsgID: "1", TS: 20}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "e@s.whatsapp.net", MsgID: "2", FromMe: true, TS: 30}))
	must(t, s.SetChatUnread("e@s.whatsapp.net", true))

	n, err := s.RepairUnreadCounts()
	must(t, err)
	if n != 2 {
		t.Fatalf("repaired %d chats, want 2", n)
	}
	want := map[string]int{"a@s.whatsapp.net": 0, "b@s.whatsapp.net": 2, "c@s.whatsapp.net": 1, "d@s.whatsapp.net": 7, "e@s.whatsapp.net": 1}
	for _, c := range mustChats(t, s) {
		if c.UnreadCount != want[c.JID] {
			t.Errorf("%s unread = %d, want %d", c.JID, c.UnreadCount, want[c.JID])
		}
	}
}

func TestMergeChatKeepsUnreadOfNewerRow(t *testing.T) {
	s := newTestStore(t)
	// The number chat was active last: its count wins over the lid's stale one.
	must(t, s.UpsertChat(ChatRow{JID: "1@lid", UnreadCount: 395, LastMessageTS: 100}))
	must(t, s.UpsertChat(ChatRow{JID: "1@s.whatsapp.net", UnreadCount: 0, LastMessageTS: 200}))
	_, err := s.MergeChat("1@lid", "1@s.whatsapp.net")
	must(t, err)
	if c := mustChats(t, s)[0]; c.UnreadCount != 0 {
		t.Fatalf("unread = %d, want the newer row's 0", c.UnreadCount)
	}
}

func TestUpsertChatFlagsOlderSnapshotKeepsLiveUnread(t *testing.T) {
	s := newTestStore(t)
	jid := "5551112222@s.whatsapp.net"
	if err := s.BumpChatActivity(jid, false, 200, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertChatFlags(ChatRow{JID: jid, UnreadCount: 0, LastMessageTS: 100}, false, false); err != nil {
		t.Fatal(err)
	}
	chats, err := s.Chats(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 1 || chats[0].UnreadCount != 3 || chats[0].LastMessageTS != 200 {
		t.Fatalf("after an older snapshot: %+v, want unread 3 at ts 200", chats)
	}
	if err := s.UpsertChatFlags(ChatRow{JID: jid, UnreadCount: 5, LastMessageTS: 300}, false, false); err != nil {
		t.Fatal(err)
	}
	chats, _ = s.Chats(10)
	if chats[0].UnreadCount != 5 || chats[0].LastMessageTS != 300 {
		t.Fatalf("after a newer snapshot: %+v, want unread 5 at ts 300", chats[0])
	}
}

func TestChatsPreviewCall(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "call:1", TS: 1, Kind: "call", Payload: `{"outcome":"missed"}`}))
	if got := mustChats(t, s)[0].Preview; got != "📞 Missed voice call" {
		t.Fatalf("missed voice: got %q", got)
	}
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "call:2", TS: 2, Kind: "call", Payload: `{"video":true,"outcome":"answered"}`, FromMe: true}))
	if got := mustChats(t, s)[0].Preview; got != "🎥 Video call" {
		t.Fatalf("answered video (no You: prefix): got %q", got)
	}
}

func TestCallText(t *testing.T) {
	cases := []struct {
		video   bool
		outcome string
		want    string
	}{
		{false, "missed", "Missed voice call"},
		{true, "missed", "Missed video call"},
		{false, "declined", "Declined voice call"},
		{true, "failed", "Failed video call"},
		{false, "answered", "Voice call"},
		{true, "", "Video call"},
	}
	for _, tc := range cases {
		if got := CallText(tc.video, tc.outcome); got != tc.want {
			t.Errorf("CallText(%v, %q) = %q, want %q", tc.video, tc.outcome, got, tc.want)
		}
	}
}

func TestChatsLastReactionOnOwnMessageNewerThanLastMessage(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "see you at 7", TS: 10, FromMe: true}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "a@s.whatsapp.net", Emoji: "👍", TS: 20}))

	lr := mustChats(t, s)[0].LastReaction
	if lr == nil {
		t.Fatal("LastReaction = nil, want the 👍 on our message")
	}
	if lr.Emoji != "👍" || lr.ReactorJID != "a@s.whatsapp.net" || lr.TS != 20 || lr.TargetPreview != "see you at 7" {
		t.Fatalf("got %+v", lr)
	}
}

func TestChatsLastReactionOlderThanLastMessageIgnored(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 10, FromMe: true}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "a@s.whatsapp.net", Emoji: "👍", TS: 20}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", Text: "ok", TS: 30}))

	c := mustChats(t, s)[0]
	if c.LastReaction != nil {
		t.Fatalf("LastReaction = %+v, want nil once a newer message exists", c.LastReaction)
	}
	if c.Preview != "ok" {
		t.Fatalf("Preview = %q, want the newer message", c.Preview)
	}
}

func TestChatsLastReactionOnOthersMessageIgnored(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 10}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "me@s.whatsapp.net", Emoji: "👍", TS: 20}))
	if lr := mustChats(t, s)[0].LastReaction; lr != nil {
		t.Fatalf("LastReaction = %+v, want nil for a reaction to someone else's message", lr)
	}
}

func TestChatsLastReactionClearedFallsBack(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 10, FromMe: true}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "a@s.whatsapp.net", Emoji: "👍", TS: 20}))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", ReactorJID: "a@s.whatsapp.net", Emoji: "", TS: 25}))
	if lr := mustChats(t, s)[0].LastReaction; lr != nil {
		t.Fatalf("LastReaction = %+v, want nil once the reaction was taken back", lr)
	}
}

func TestMessagePreview(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hello", TS: 1, FromMe: true}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", TS: 2}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", Kind: "image", Caption: "Sunset!"}))

	preview, fromMe, ok, err := s.MessagePreview("a@s.whatsapp.net", "m1")
	must(t, err)
	if !ok || !fromMe || preview != "hello" {
		t.Fatalf("m1: ok=%v fromMe=%v preview=%q, want our plain text with no You: prefix", ok, fromMe, preview)
	}
	preview, fromMe, ok, err = s.MessagePreview("a@s.whatsapp.net", "m2")
	must(t, err)
	if !ok || fromMe || preview != "📷 Sunset!" {
		t.Fatalf("m2: ok=%v fromMe=%v preview=%q", ok, fromMe, preview)
	}
	if _, _, ok, err = s.MessagePreview("a@s.whatsapp.net", "nope"); err != nil || ok {
		t.Fatalf("unknown: ok=%v err=%v, want ok=false", ok, err)
	}
}
