package ui

import (
	"context"
	"testing"

	"chatot/internal/client"
)

func TestComposeStateSubmitNoChat(t *testing.T) {
	var s composeState
	if _, ok := s.Submit("hi"); ok {
		t.Error("expected Submit to fail with no active chat")
	}
}

func TestComposeStateSubmitEmptyText(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")

	if _, ok := s.Submit(""); ok {
		t.Error("expected Submit to fail on empty text")
	}
	if _, ok := s.Submit("   "); ok {
		t.Error("expected Submit to fail on whitespace-only text")
	}
}

func TestComposeStateSubmitText(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")

	action, ok := s.Submit("  hello  ")
	if !ok {
		t.Fatal("expected Submit to succeed")
	}
	if action.JID != "a@s.whatsapp.net" {
		t.Errorf("JID = %q, want %q", action.JID, "a@s.whatsapp.net")
	}
	if action.Text != "hello" {
		t.Errorf("Text = %q, want trimmed %q", action.Text, "hello")
	}
	if action.ReplyTo != nil {
		t.Errorf("ReplyTo = %+v, want nil", action.ReplyTo)
	}
}

func TestComposeStateSubmitMediaNoChat(t *testing.T) {
	var s composeState
	if _, ok := s.SubmitMedia("/tmp/photo.png", "caption"); ok {
		t.Error("expected SubmitMedia to fail with no active chat")
	}
}

func TestComposeStateSubmitMediaEmptyPath(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")
	if _, ok := s.SubmitMedia("  ", "caption"); ok {
		t.Error("expected SubmitMedia to fail on blank path")
	}
}

func TestComposeStateSubmitMedia(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")
	s.StartReply(client.Message{ID: "m1", ChatJID: "a@s.whatsapp.net", Text: "original"})

	action, ok := s.SubmitMedia("/tmp/photo.png", "  a caption  ")
	if !ok {
		t.Fatal("expected SubmitMedia to succeed")
	}
	if action.JID != "a@s.whatsapp.net" || action.Path != "/tmp/photo.png" {
		t.Errorf("action = %+v", action)
	}
	if action.Caption != "a caption" {
		t.Errorf("Caption = %q, want trimmed %q", action.Caption, "a caption")
	}
	if action.ReplyTo == nil || action.ReplyTo.MsgID != "m1" {
		t.Errorf("ReplyTo = %+v, want MsgID m1", action.ReplyTo)
	}
	if _, ok := s.ReplyTarget(); ok {
		t.Error("expected reply mode cleared after SubmitMedia")
	}
}

func TestGuessAttachmentKind(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/tmp/photo.png", "image"},
		{"/tmp/clip.mp4", "video"},
		{"/tmp/note.ogg", "audio"},
		{"/tmp/report.pdf", "document"},
		{"/tmp/unknownext.xyz123", "document"},
	}
	for _, tc := range cases {
		if got := guessAttachmentKind(tc.path); got != tc.want {
			t.Errorf("guessAttachmentKind(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestComposeStateStartReplyThenSubmit(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")
	s.StartReply(client.Message{ID: "m1", ChatJID: "a@s.whatsapp.net", Text: "original"})

	if _, ok := s.ReplyTarget(); !ok {
		t.Fatal("expected ReplyTarget set after StartReply")
	}

	action, ok := s.Submit("reply text")
	if !ok {
		t.Fatal("expected Submit to succeed")
	}
	if action.ReplyTo == nil || action.ReplyTo.MsgID != "m1" || action.ReplyTo.ChatJID != "a@s.whatsapp.net" {
		t.Errorf("ReplyTo = %+v, want {ChatJID: a@s.whatsapp.net, MsgID: m1}", action.ReplyTo)
	}

	if _, ok := s.ReplyTarget(); ok {
		t.Error("expected reply mode cleared after Submit")
	}
}

func TestComposeStateCancelReply(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")
	s.StartReply(client.Message{ID: "m1"})

	s.CancelReply()

	if _, ok := s.ReplyTarget(); ok {
		t.Error("expected ReplyTarget cleared after CancelReply")
	}

	action, ok := s.Submit("no reply")
	if !ok {
		t.Fatal("expected Submit to succeed")
	}
	if action.ReplyTo != nil {
		t.Errorf("ReplyTo = %+v, want nil after CancelReply", action.ReplyTo)
	}
}

func TestComposeStateSetChatClearsReply(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")
	s.StartReply(client.Message{ID: "m1"})

	s.SetChat("b@s.whatsapp.net")

	if _, ok := s.ReplyTarget(); ok {
		t.Error("expected switching chats to clear pending reply")
	}
}

func TestUnreadMessageIDsTakesLastNInbound(t *testing.T) {
	msgs := []client.Message{
		{ID: "1", FromMe: false},
		{ID: "2", FromMe: true},
		{ID: "3", FromMe: false},
		{ID: "4", FromMe: false},
	}

	ids := unreadMessageIDs(msgs, 2)
	if len(ids) != 2 || ids[0] != "4" || ids[1] != "3" {
		t.Errorf("ids = %v, want [4 3]", ids)
	}
}

func TestUnreadMessageIDsZeroCount(t *testing.T) {
	msgs := []client.Message{{ID: "1", FromMe: false}}
	if ids := unreadMessageIDs(msgs, 0); ids != nil {
		t.Errorf("ids = %v, want nil for zero unread count", ids)
	}
}

func TestMarkReadOnOpenSkippedByDefault(t *testing.T) {
	if SendReadReceipts {
		t.Fatal("expected SendReadReceipts to default to false")
	}

	f := client.NewFake()
	msgs, err := f.Messages("1234567890@s.whatsapp.net", 0)
	if err != nil {
		t.Fatal(err)
	}

	MarkReadOnOpen(context.Background(), f, "1234567890@s.whatsapp.net", msgs, 2)

	chats, err := f.Chats(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chats {
		if c.JID == "1234567890@s.whatsapp.net" && c.UnreadCount == 0 {
			t.Error("expected MarkRead not called (UnreadCount untouched) when SendReadReceipts is false")
		}
	}
}

func TestMarkReadOnOpenSendsWhenEnabled(t *testing.T) {
	SendReadReceipts = true
	defer func() { SendReadReceipts = false }()

	f := client.NewFake()
	msgs, err := f.Messages("1234567890@s.whatsapp.net", 0)
	if err != nil {
		t.Fatal(err)
	}

	MarkReadOnOpen(context.Background(), f, "1234567890@s.whatsapp.net", msgs, 2)

	chats, err := f.Chats(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chats {
		if c.JID == "1234567890@s.whatsapp.net" && c.UnreadCount != 0 {
			t.Errorf("UnreadCount = %d, want 0 after MarkRead", c.UnreadCount)
		}
	}
}
