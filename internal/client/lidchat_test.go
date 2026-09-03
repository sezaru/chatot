package client

import (
	"os"
	"path/filepath"
	"testing"

	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"chatot/internal/store"
)

func TestCanonicalChat(t *testing.T) {
	known := func(lid types.JID) types.JID {
		if lid.User == "64081113427987" {
			return types.NewJID("554888073648", types.DefaultUserServer)
		}
		return types.EmptyJID
	}
	cases := []struct{ in, want string }{
		{"64081113427987@lid", "554888073648@s.whatsapp.net"},
		{"64081113427987:45@lid", "554888073648@s.whatsapp.net"},
		{"999@lid", "999@lid"},
		{"554888073648@s.whatsapp.net", "554888073648@s.whatsapp.net"},
		{"120363000000000000@g.us", "120363000000000000@g.us"},
		{"not a jid", "not a jid"},
	}
	for _, c := range cases {
		if got := canonicalChat(c.in, known); got != c.want {
			t.Errorf("canonicalChat(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func newLIDFixture(t *testing.T) *Whatsmeow {
	t.Helper()
	w := newIngestFixture(t)
	w.events = newEventBus(waLog.Noop.Warnf)
	return w
}

func TestLIDMessageFiledUnderNumberChat(t *testing.T) {
	w := newLIDFixture(t)
	must(t, w.store.UpsertContact(store.ContactRow{JID: "64081113427987@lid", PNJID: "554888073648@s.whatsapp.net"}))
	e := &Event{Kind: EventMessage, Message: &Message{ID: "A", ChatJID: "64081113427987@lid", FromJID: "64081113427987@lid", FromMe: true, Text: "a", TS: 10}}
	w.canonicalizeEvent(e)
	w.ingestEvent(*e)
	if e.Message.ChatJID != "554888073648@s.whatsapp.net" {
		t.Fatalf("ChatJID = %q, want the number chat", e.Message.ChatJID)
	}
	if _, ok := chatByJID(t, w, "554888073648@s.whatsapp.net"); !ok {
		t.Fatal("no chat row under the number")
	}
	if _, ok := chatByJID(t, w, "64081113427987@lid"); ok {
		t.Fatal("a chat row was still created under the lid")
	}
	// Unknown LIDs stay where they are rather than guessing.
	e = &Event{Kind: EventMessage, Message: &Message{ID: "B", ChatJID: "999@lid", FromJID: "999@lid", Text: "b", TS: 11}}
	w.canonicalizeEvent(e)
	if e.Message.ChatJID != "999@lid" {
		t.Fatalf("unknown lid rewritten to %q", e.Message.ChatJID)
	}
}

func TestLearnedMappingMergesLIDChat(t *testing.T) {
	w := newLIDFixture(t)
	must(t, w.ingestMessage(&Message{ID: "A", ChatJID: "999@lid", FromJID: "999@lid", Text: "hi", TS: 10}))
	must(t, w.ingestMessage(&Message{ID: "B", ChatJID: "111@s.whatsapp.net", FromJID: "111@s.whatsapp.net", Text: "hey", TS: 5}))
	w.upsertContact(store.ContactRow{JID: "999@lid", PNJID: "111@s.whatsapp.net"})
	if _, ok := chatByJID(t, w, "999@lid"); ok {
		t.Fatal("lid chat survived learning its number")
	}
	msgs, err := w.store.Messages("111@s.whatsapp.net", 10)
	must(t, err)
	if len(msgs) != 2 {
		t.Fatalf("%d messages under the number chat, want 2", len(msgs))
	}
	if got, _ := w.store.ContactPNJID("999@lid"); got != "111@s.whatsapp.net" {
		t.Fatalf("ContactPNJID = %q", got)
	}
}

func TestHistoryLIDConversationFiledUnderNumber(t *testing.T) {
	w := newLIDFixture(t)
	str := func(s string) *string { return &s }
	ts := uint64(42)
	w.applyHistoryConversation(&waHistorySync.Conversation{
		ID:                    str("64081113427987@lid"),
		PnJID:                 str("554888073648@s.whatsapp.net"),
		ConversationTimestamp: &ts,
	})
	if _, ok := chatByJID(t, w, "64081113427987@lid"); ok {
		t.Fatal("conversation stored under the lid")
	}
	c, ok := chatByJID(t, w, "554888073648@s.whatsapp.net")
	if !ok || c.LastMessageTS != 42 {
		t.Fatalf("number chat = %+v, ok=%v", c, ok)
	}
}

func TestMergeLIDChatsAtStartup(t *testing.T) {
	w := newLIDFixture(t)
	must(t, w.ingestMessage(&Message{ID: "A", ChatJID: "64081113427987@lid", FromJID: "64081113427987@lid", FromMe: true, Text: "a", TS: 10}))
	must(t, w.ingestMessage(&Message{ID: "B", ChatJID: "554888073648@s.whatsapp.net", FromJID: "554888073648@s.whatsapp.net", FromMe: true, Text: "b", TS: 5}))
	must(t, w.ingestMessage(&Message{ID: "C", ChatJID: "999@lid", FromJID: "999@lid", Text: "c", TS: 7}))
	must(t, w.store.UpsertContact(store.ContactRow{JID: "64081113427987@lid", PNJID: "554888073648@s.whatsapp.net"}))
	w.mergeLIDChats()
	lids, err := w.store.LIDChatJIDs()
	must(t, err)
	if len(lids) != 1 || lids[0] != "999@lid" {
		t.Fatalf("lid chats left = %v, want only the unmapped 999@lid", lids)
	}
	msgs, err := w.store.Messages("554888073648@s.whatsapp.net", 10)
	must(t, err)
	if len(msgs) != 2 {
		t.Fatalf("%d messages under the number chat, want 2", len(msgs))
	}
}

func TestMergeLIDChatsAdoptsCachedAvatar(t *testing.T) {
	w := newLIDFixture(t)
	w.avatarDir = t.TempDir()
	must(t, os.WriteFile(filepath.Join(w.avatarDir, avatarCacheName("64081113427987@lid")), []byte("jpg"), 0o600))
	must(t, w.ingestMessage(&Message{ID: "A", ChatJID: "64081113427987@lid", FromJID: "64081113427987@lid", FromMe: true, Text: "a", TS: 10}))
	must(t, w.store.UpsertContact(store.ContactRow{JID: "64081113427987@lid", PNJID: "554888073648@s.whatsapp.net"}))
	w.mergeLIDChats()
	if _, err := os.Stat(filepath.Join(w.avatarDir, avatarCacheName("554888073648@s.whatsapp.net"))); err != nil {
		t.Fatalf("avatar not carried over: %v", err)
	}
}

func TestCanonicalizeEventCoversEveryChatReference(t *testing.T) {
	w := newLIDFixture(t)
	must(t, w.store.UpsertContact(store.ContactRow{JID: "999@lid", PNJID: "111@s.whatsapp.net"}))
	const pn = "111@s.whatsapp.net"
	e := &Event{
		Message:      &Message{ChatJID: "999@lid", ReplyTo: &MsgRef{ChatJID: "999@lid", MsgID: "R"}},
		Receipt:      &Receipt{ChatJID: "999@lid"},
		Reaction:     &Reaction{ChatJID: "999@lid"},
		Revoke:       &Revoke{ChatJID: "999@lid"},
		ChatPresence: &ChatPresence{ChatJID: "999@lid"},
		PollVote:     &PollVote{ChatJID: "999@lid"},
		ChatUpdate:   &ChatUpdate{JID: "999@lid"},
		Call:         &Call{ChatJID: "999@lid"},
		HistorySync:  &HistorySync{ChatJIDs: []string{"999@lid", "120363000000000000@g.us"}},
	}
	w.canonicalizeEvent(e)
	got := []string{e.Message.ChatJID, e.Message.ReplyTo.ChatJID, e.Receipt.ChatJID, e.Reaction.ChatJID, e.Revoke.ChatJID, e.ChatPresence.ChatJID, e.PollVote.ChatJID, e.ChatUpdate.JID, e.Call.ChatJID, e.HistorySync.ChatJIDs[0]}
	for i, g := range got {
		if g != pn {
			t.Errorf("reference %d = %q, want %q", i, g, pn)
		}
	}
	if e.HistorySync.ChatJIDs[1] != "120363000000000000@g.us" {
		t.Errorf("group rewritten to %q", e.HistorySync.ChatJIDs[1])
	}
}
