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

func TestParseLocationValid(t *testing.T) {
	loc, ok := parseLocation("  Home  ", "  1 Main St ", " 51.5 ", " -0.12 ")
	if !ok {
		t.Fatal("expected parseLocation to succeed on valid coords")
	}
	if loc.Name != "Home" || loc.Address != "1 Main St" {
		t.Errorf("got name=%q addr=%q, want trimmed", loc.Name, loc.Address)
	}
	if loc.Latitude != 51.5 || loc.Longitude != -0.12 {
		t.Errorf("got lat=%v long=%v", loc.Latitude, loc.Longitude)
	}
}

func TestParseLocationRejectsBadInput(t *testing.T) {
	cases := [][2]string{
		{"", "0"},      // empty lat
		{"abc", "0"},   // non-numeric lat
		{"0", "xyz"},   // non-numeric long
		{"91", "0"},    // lat out of range
		{"0", "181"},   // long out of range
		{"-90.1", "0"}, // lat just out of range
	}
	for _, c := range cases {
		if _, ok := parseLocation("", "", c[0], c[1]); ok {
			t.Errorf("parseLocation(%q,%q) succeeded, want rejection", c[0], c[1])
		}
	}
}

func TestComposeStateSubmitLocationNoChat(t *testing.T) {
	var s composeState
	if _, ok := s.SubmitLocation("", "", "1", "2"); ok {
		t.Error("expected SubmitLocation to fail with no active chat")
	}
}

func TestComposeStateSubmitLocationClearsReply(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")
	s.StartReply(client.Message{ID: "m1", ChatJID: "a@s.whatsapp.net"})

	action, ok := s.SubmitLocation("Home", "", "51.5", "-0.12")
	if !ok {
		t.Fatal("expected SubmitLocation to succeed")
	}
	if action.ReplyTo == nil || action.ReplyTo.MsgID != "m1" {
		t.Errorf("ReplyTo = %+v, want m1", action.ReplyTo)
	}
	if _, replying := s.ReplyTarget(); replying {
		t.Error("expected reply to be cleared after SubmitLocation")
	}
}

func TestParsePollFormValid(t *testing.T) {
	name, opts, sel, ok := parsePollForm("  Lunch?  ", []string{" Pizza ", "", "Sushi", "  "}, 1)
	if !ok {
		t.Fatal("expected valid poll form")
	}
	if name != "Lunch?" {
		t.Errorf("name = %q, want Lunch?", name)
	}
	if len(opts) != 2 || opts[0] != "Pizza" || opts[1] != "Sushi" {
		t.Errorf("opts = %v, want [Pizza Sushi]", opts)
	}
	if sel != 1 {
		t.Errorf("sel = %d, want 1", sel)
	}
}

func TestParsePollFormRejectsBlankQuestion(t *testing.T) {
	if _, _, _, ok := parsePollForm("   ", []string{"a", "b"}, 1); ok {
		t.Error("expected blank question to be rejected")
	}
}

func TestParsePollFormRejectsTooFewOptions(t *testing.T) {
	if _, _, _, ok := parsePollForm("Q", []string{"only", " "}, 1); ok {
		t.Error("expected <2 options to be rejected")
	}
}

func TestParsePollFormClampsSelectable(t *testing.T) {
	_, opts, sel, ok := parsePollForm("Q", []string{"a", "b", "c"}, 9)
	if !ok {
		t.Fatal("expected valid form")
	}
	if sel != len(opts) {
		t.Errorf("sel = %d, want %d (clamped to option count)", sel, len(opts))
	}
	if _, _, sel, _ = parsePollForm("Q", []string{"a", "b"}, 0); sel != 1 {
		t.Errorf("sel = %d, want 1 (clamped up from 0)", sel)
	}
}

func TestComposeStateSubmitPollNoChat(t *testing.T) {
	var s composeState
	if _, ok := s.SubmitPoll("Q", []string{"a", "b"}, 1); ok {
		t.Error("expected SubmitPoll to fail with no active chat")
	}
}
