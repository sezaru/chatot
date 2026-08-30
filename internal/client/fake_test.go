package client

import (
	"context"
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
