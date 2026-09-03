package store

import "testing"

func TestNewsletterReactionsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	must(t, s.SetNewsletterReaction("1@newsletter", "p1", "🔥"))
	must(t, s.SetNewsletterReaction("1@newsletter", "p2", "❤️"))
	must(t, s.SetNewsletterReaction("2@newsletter", "p1", "👍"))
	must(t, s.SetNewsletterReaction("1@newsletter", "p1", "😂")) // replaces
	must(t, s.SetNewsletterReaction("1@newsletter", "p2", ""))  // withdraws

	got, err := s.NewsletterReactions("1@newsletter")
	must(t, err)
	if len(got) != 1 || got["p1"] != "😂" {
		t.Fatalf("got %v, want only p1 → 😂", got)
	}
	other, err := s.NewsletterReactions("2@newsletter")
	must(t, err)
	if other["p1"] != "👍" {
		t.Fatalf("channels must not share reactions: got %v", other)
	}
}

func TestReadReceiptsKeepEarliestAndOrder(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertReadReceipt("status@broadcast", "s1", "b@s.whatsapp.net", 200))
	must(t, s.UpsertReadReceipt("status@broadcast", "s1", "a@s.whatsapp.net", 100))
	must(t, s.UpsertReadReceipt("status@broadcast", "s1", "b@s.whatsapp.net", 50)) // earlier wins
	must(t, s.UpsertReadReceipt("status@broadcast", "s1", "b@s.whatsapp.net", 300))
	must(t, s.UpsertReadReceipt("status@broadcast", "s2", "c@s.whatsapp.net", 10))

	got, err := s.ReadReceipts("status@broadcast", "s1")
	must(t, err)
	if len(got) != 2 || got[0].ReaderJID != "b@s.whatsapp.net" || got[0].TS != 50 || got[1].ReaderJID != "a@s.whatsapp.net" {
		t.Fatalf("got %+v, want b@50 then a@100", got)
	}
}

func TestStatusMutesRoundTrip(t *testing.T) {
	s := newTestStore(t)
	must(t, s.SetStatusMuted("b@s.whatsapp.net", true))
	must(t, s.SetStatusMuted("a@s.whatsapp.net", true))
	must(t, s.SetStatusMuted("a@s.whatsapp.net", true)) // idempotent
	got, err := s.MutedStatusPosters()
	must(t, err)
	if len(got) != 2 || got[0] != "a@s.whatsapp.net" {
		t.Fatalf("got %v", got)
	}
	must(t, s.SetStatusMuted("a@s.whatsapp.net", false))
	got, err = s.MutedStatusPosters()
	must(t, err)
	if len(got) != 1 || got[0] != "b@s.whatsapp.net" {
		t.Fatalf("after unmute got %v", got)
	}
}

func TestStatusesHonourSinceCutoff(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertMessage(MessageRow{ChatJID: "status@broadcast", MsgID: "old", FromJID: "a@s.whatsapp.net", Text: "old", TS: 100}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "status@broadcast", MsgID: "new", FromJID: "a@s.whatsapp.net", Text: "new", TS: 500}))
	got, err := s.Statuses(200, 50)
	must(t, err)
	if len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("got %+v, want only the update at/after the cutoff", got)
	}
}

func TestChatsExcludeNewsletters(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "dm@s.whatsapp.net", Name: "Friend"}))
	// A channel post arrives through the normal message path and bumps a
	// chat row for the channel, which belongs to the Channels tab, not here.
	must(t, s.BumpChatActivity("123@newsletter", false, 100, 1))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "123@newsletter", MsgID: "p1", FromJID: "123@newsletter", Text: "post", TS: 100}))
	chats := mustChats(t, s)
	if len(chats) != 1 || chats[0].JID != "dm@s.whatsapp.net" {
		t.Fatalf("got %+v, want only the DM", chats)
	}
}
