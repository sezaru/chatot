package client

import (
	"context"
	"strings"
	"testing"
)

func TestFakeChatsReturnsSeeded(t *testing.T) {
	f := NewFake()
	chats, err := f.Chats(0)
	if err != nil {
		t.Fatalf("Chats: %v", err)
	}
	if len(chats) != 2 {
		t.Fatalf("got %d chats, want 2", len(chats))
	}
	if chats[0].Name != "Ada Lovelace" {
		t.Errorf("chats[0].Name = %q, want Ada Lovelace", chats[0].Name)
	}
}

func TestFakeChatsRespectsLimit(t *testing.T) {
	f := NewFake()
	chats, err := f.Chats(1)
	if err != nil {
		t.Fatalf("Chats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("got %d chats, want 1", len(chats))
	}
}

func TestFakeMessagesReturnsSeeded(t *testing.T) {
	f := NewFake()
	msgs, err := f.Messages("1234567890@s.whatsapp.net", 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}
	if msgs[len(msgs)-1].Text != "See you tomorrow!" {
		t.Errorf("last message text = %q", msgs[len(msgs)-1].Text)
	}
}

func TestFakeMessagesUnknownChatIsEmpty(t *testing.T) {
	f := NewFake()
	msgs, err := f.Messages("nonexistent@s.whatsapp.net", 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("got %d messages, want 0", len(msgs))
	}
}

func TestFakeSendTextAppendsAndReturnsID(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"
	before, _ := f.Messages(jid, 0)

	id, err := f.SendText(context.Background(), jid, "on my way", nil)
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if id == "" {
		t.Fatal("SendText returned empty ID")
	}

	after, _ := f.Messages(jid, 0)
	if len(after) != len(before)+1 {
		t.Fatalf("got %d messages after send, want %d", len(after), len(before)+1)
	}
	last := after[len(after)-1]
	if last.Text != "on my way" || !last.FromMe || last.ID != id {
		t.Errorf("appended message = %+v", last)
	}

	chats, _ := f.Chats(0)
	for _, c := range chats {
		if c.JID == jid && c.Preview != "on my way" {
			t.Errorf("chat preview not updated: %q", c.Preview)
		}
	}
}

func TestFakeSendContactAppendsAndReturnsID(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"
	contact := Contact{DisplayName: "Alan Turing", Phones: []string{"+44 20 7946 0958"}}

	id, err := f.SendContact(context.Background(), jid, contact, nil)
	if err != nil {
		t.Fatalf("SendContact: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty message ID")
	}

	msgs, err := f.Messages(jid, 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	last := msgs[len(msgs)-1]
	if last.ID != id || !last.FromMe || last.Contact == nil {
		t.Fatalf("last message = %+v, want appended contact send", last)
	}
	if last.Contact.DisplayName != "Alan Turing" {
		t.Errorf("Contact.DisplayName = %q", last.Contact.DisplayName)
	}

	chats, err := f.Chats(0)
	if err != nil {
		t.Fatalf("Chats: %v", err)
	}
	for _, c := range chats {
		if c.JID == jid && !strings.Contains(c.Preview, "Alan Turing") {
			t.Errorf("chat preview = %q, want it to mention the shared contact", c.Preview)
		}
	}
}

func TestFakeSendStickerAppendsAndReturnsID(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"

	id, err := f.SendSticker(context.Background(), jid, "/tmp/party.webp")
	if err != nil {
		t.Fatalf("SendSticker: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty message ID")
	}

	msgs, err := f.Messages(jid, 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	last := msgs[len(msgs)-1]
	if last.ID != id || !last.FromMe || last.Attachment == nil {
		t.Fatalf("last message = %+v, want appended sticker send", last)
	}
	if last.Attachment.Kind != "sticker" || last.Attachment.LocalPath != "/tmp/party.webp" {
		t.Errorf("Attachment = %+v, want kind sticker with the sent path", last.Attachment)
	}

	chats, err := f.Chats(0)
	if err != nil {
		t.Fatalf("Chats: %v", err)
	}
	for _, c := range chats {
		if c.JID == jid && !strings.Contains(c.Preview, "Sticker") {
			t.Errorf("chat preview = %q, want it to mention the sticker", c.Preview)
		}
	}
}

func TestFakeSearchFindsSeededMessage(t *testing.T) {
	f := NewFake()
	hits, err := f.Search("relay", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	if hits[0].MsgID != "m4" {
		t.Errorf("hit MsgID = %q, want m4", hits[0].MsgID)
	}
}

func TestFakeSearchIsCaseInsensitive(t *testing.T) {
	f := NewFake()
	hits, err := f.Search("RELAY", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
}

func TestFakeReactAndClear(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"
	if err := f.React(context.Background(), jid, "m1", "👍"); err != nil {
		t.Fatalf("React: %v", err)
	}
	msgs, _ := f.Messages(jid, 0)
	if msgs[0].Reactions["👍"] != "me" {
		t.Fatalf("reaction not recorded: %+v", msgs[0].Reactions)
	}

	if err := f.React(context.Background(), jid, "m1", ""); err != nil {
		t.Fatalf("React clear: %v", err)
	}
	msgs, _ = f.Messages(jid, 0)
	if _, ok := msgs[0].Reactions["👍"]; ok {
		t.Fatalf("reaction not cleared: %+v", msgs[0].Reactions)
	}
}

func TestFakeReactUnknownMessageErrors(t *testing.T) {
	f := NewFake()
	if err := f.React(context.Background(), "1234567890@s.whatsapp.net", "nope", "👍"); err == nil {
		t.Fatal("expected error reacting to unknown message")
	}
}

func TestFakeMarkReadClearsUnread(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"
	if err := f.MarkRead(context.Background(), jid, []string{"m1"}); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	chats, _ := f.Chats(0)
	for _, c := range chats {
		if c.JID == jid && c.UnreadCount != 0 {
			t.Errorf("UnreadCount = %d, want 0", c.UnreadCount)
		}
	}
}

func TestFakeLoggedInLifecycle(t *testing.T) {
	f := NewFake()
	if !f.LoggedIn() {
		t.Fatal("NewFake should start logged in")
	}
	if err := f.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if f.LoggedIn() {
		t.Fatal("LoggedIn should be false after Logout")
	}
}

func TestFakePushEventDeliversOnEvents(t *testing.T) {
	f := NewFake()
	// Events() is a fan-out subscription: subscribe before publishing, as the
	// real UI consumers do from their own goroutines.
	evs := f.Events()
	f.PushEvent(Event{Kind: EventConnection, Connection: &Connection{Connected: true}})
	select {
	case e := <-evs:
		if e.Kind != EventConnection || !e.Connection.Connected {
			t.Errorf("got unexpected event %+v", e)
		}
	default:
		t.Fatal("expected an event on Events()")
	}
}

func TestFakeEditMessage(t *testing.T) {
	f := NewFake()
	sub := f.Events()

	if err := f.EditMessage(context.Background(), "1234567890@s.whatsapp.net", "m2", "Yep, definitely!"); err != nil {
		t.Fatalf("EditMessage: %v", err)
	}

	select {
	case ev := <-sub:
		if ev.Kind != EventMessage || ev.Message == nil {
			t.Fatalf("got kind=%v msg=%v, want an EventMessage", ev.Kind, ev.Message)
		}
		if !ev.Message.Edited || ev.Message.ID != "m2" || ev.Message.Text != "Yep, definitely!" {
			t.Errorf("event msg = %+v, want edited m2 with new text", ev.Message)
		}
	default:
		t.Fatal("EditMessage did not publish an event")
	}

	msgs, err := f.Messages("1234567890@s.whatsapp.net", 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	for _, m := range msgs {
		if m.ID == "m2" {
			if m.Text != "Yep, definitely!" || !m.Edited {
				t.Errorf("stored m2 = %+v, want edited text + flag", m)
			}
			return
		}
	}
	t.Fatal("m2 not found after edit")
}

func TestFakeEditMessageNotFound(t *testing.T) {
	f := NewFake()
	if err := f.EditMessage(context.Background(), "1234567890@s.whatsapp.net", "nope", "x"); err == nil {
		t.Error("expected error editing a non-existent message")
	}
}

func TestFakeDeleteMessage(t *testing.T) {
	f := NewFake()
	sub := f.Events()

	if err := f.DeleteMessage(context.Background(), "1234567890@s.whatsapp.net", "m2"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	select {
	case ev := <-sub:
		if ev.Kind != EventRevoke || ev.Revoke == nil {
			t.Fatalf("got kind=%v revoke=%v, want an EventRevoke", ev.Kind, ev.Revoke)
		}
		if ev.Revoke.MsgID != "m2" {
			t.Errorf("Revoke.MsgID = %q, want m2", ev.Revoke.MsgID)
		}
	default:
		t.Fatal("DeleteMessage did not publish an event")
	}

	msgs, err := f.Messages("1234567890@s.whatsapp.net", 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	for _, m := range msgs {
		if m.ID == "m2" {
			if !m.Deleted {
				t.Errorf("stored m2 = %+v, want Deleted=true", m)
			}
			return
		}
	}
	t.Fatal("m2 not found after delete")
}

func TestFakeDeleteMessageNotFound(t *testing.T) {
	f := NewFake()
	if err := f.DeleteMessage(context.Background(), "1234567890@s.whatsapp.net", "nope"); err == nil {
		t.Error("expected error deleting a non-existent message")
	}
}

func TestFakeCheckOnWhatsApp(t *testing.T) {
	f := NewFake()

	jid, onWA, err := f.CheckOnWhatsApp(context.Background(), "+15559876543")
	if err != nil {
		t.Fatalf("CheckOnWhatsApp: %v", err)
	}
	if !onWA {
		t.Error("expected a numeric phone to be reported on WhatsApp")
	}
	if jid != "15559876543@s.whatsapp.net" {
		t.Errorf("jid = %q, want 15559876543@s.whatsapp.net", jid)
	}

	_, onWA, err = f.CheckOnWhatsApp(context.Background(), "not-a-number")
	if err != nil {
		t.Fatalf("CheckOnWhatsApp: %v", err)
	}
	if onWA {
		t.Error("expected a non-numeric input to not be on WhatsApp")
	}
}

func TestFakeStarMessage(t *testing.T) {
	f := NewFake()
	sub := f.Events()

	if err := f.StarMessage(context.Background(), "1234567890@s.whatsapp.net", "m2", true); err != nil {
		t.Fatalf("StarMessage: %v", err)
	}

	select {
	case ev := <-sub:
		if ev.Kind != EventReaction || ev.Reaction == nil || ev.Reaction.MsgID != "m2" {
			t.Fatalf("got kind=%v reaction=%v, want an EventReaction for m2", ev.Kind, ev.Reaction)
		}
	default:
		t.Fatal("StarMessage did not publish a refresh event")
	}

	starred, err := f.StarredMessages(0)
	if err != nil {
		t.Fatalf("StarredMessages: %v", err)
	}
	if len(starred) != 1 || starred[0].ID != "m2" {
		t.Fatalf("got %+v, want only m2 starred", starred)
	}

	if err := f.StarMessage(context.Background(), "1234567890@s.whatsapp.net", "m2", false); err != nil {
		t.Fatalf("StarMessage (unstar): %v", err)
	}
	starred, err = f.StarredMessages(0)
	if err != nil {
		t.Fatalf("StarredMessages: %v", err)
	}
	if len(starred) != 0 {
		t.Fatalf("got %+v, want none starred after unstar", starred)
	}
}

func TestFakeStarMessageNotFound(t *testing.T) {
	f := NewFake()
	if err := f.StarMessage(context.Background(), "1234567890@s.whatsapp.net", "nope", true); err == nil {
		t.Error("expected error starring a non-existent message")
	}
}

// TestFakeBlockUnblockCachedSet exercises the cached blocked-set semantics
// the whatsmeow client shares in spirit: block adds, unblock removes,
// IsBlocked reflects it synchronously, and Blocklist lists exactly the
// blocked JIDs.
func TestFakeBlockUnblockCachedSet(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"

	if f.IsBlocked(jid) {
		t.Fatal("jid should not start blocked")
	}

	if err := f.SetBlocked(context.Background(), jid, true); err != nil {
		t.Fatalf("SetBlocked(block): %v", err)
	}
	if !f.IsBlocked(jid) {
		t.Error("IsBlocked should be true after blocking")
	}
	list, err := f.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	if len(list) != 1 || list[0] != jid {
		t.Errorf("Blocklist() = %v, want [%s]", list, jid)
	}

	if err := f.SetBlocked(context.Background(), jid, false); err != nil {
		t.Fatalf("SetBlocked(unblock): %v", err)
	}
	if f.IsBlocked(jid) {
		t.Error("IsBlocked should be false after unblocking")
	}
	list, err = f.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("Blocklist() = %v, want empty after unblock", list)
	}
}

func TestFakeSetBlockedPublishesChatUpdate(t *testing.T) {
	f := NewFake()
	events := f.Events()
	jid := "1234567890@s.whatsapp.net"

	if err := f.SetBlocked(context.Background(), jid, true); err != nil {
		t.Fatalf("SetBlocked: %v", err)
	}
	ev := <-events
	if ev.Kind != EventChatUpdate || ev.ChatUpdate == nil || ev.ChatUpdate.JID != jid {
		t.Errorf("got event %+v, want EventChatUpdate for %s", ev, jid)
	}
}

func TestFakeRequestMoreHistorySynthesizesAndPublishes(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"
	events := f.Events()

	before, err := f.Messages(jid, 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	oldestID := before[0].ID

	if err := f.RequestMoreHistory(context.Background(), jid, oldestID, 5); err != nil {
		t.Fatalf("RequestMoreHistory: %v", err)
	}

	ev := <-events
	if ev.Kind != EventHistorySync || ev.HistorySync == nil {
		t.Fatalf("got event %+v, want EventHistorySync", ev)
	}
	if len(ev.HistorySync.ChatJIDs) != 1 || ev.HistorySync.ChatJIDs[0] != jid {
		t.Errorf("HistorySync.ChatJIDs = %v, want [%s]", ev.HistorySync.ChatJIDs, jid)
	}

	older, err := f.MessagesBefore(jid, oldestID, 50)
	if err != nil {
		t.Fatalf("MessagesBefore: %v", err)
	}
	if len(older) == 0 {
		t.Fatal("MessagesBefore returned nothing after RequestMoreHistory; expected synthesized older messages")
	}
}

func TestFakeRequestMoreHistoryOnlySyncsOnce(t *testing.T) {
	f := NewFake()
	jid := "1234567890@s.whatsapp.net"

	before, _ := f.Messages(jid, 0)
	oldestID := before[0].ID
	if err := f.RequestMoreHistory(context.Background(), jid, oldestID, 5); err != nil {
		t.Fatalf("RequestMoreHistory: %v", err)
	}

	all, err := f.Messages(jid, 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	newOldestID := all[0].ID

	// A second request for the (now oldest) synthesized message must be a
	// no-op: the fake only backfills a chat once.
	if err := f.RequestMoreHistory(context.Background(), jid, newOldestID, 5); err != nil {
		t.Fatalf("RequestMoreHistory (2nd): %v", err)
	}
	after, err := f.Messages(jid, 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(after) != len(all) {
		t.Errorf("message count changed after 2nd RequestMoreHistory: got %d, want %d", len(after), len(all))
	}
}

func TestFakeRequestMoreHistoryUnknownMessageErrors(t *testing.T) {
	f := NewFake()
	if err := f.RequestMoreHistory(context.Background(), "1234567890@s.whatsapp.net", "nonexistent", 5); err == nil {
		t.Error("RequestMoreHistory with unknown oldestMsgID should error")
	}
}
