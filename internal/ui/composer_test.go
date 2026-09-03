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

// TestMarkReadOnOpenClearsBadgeWithoutReceipts: with receipts off the
// badge still clears locally (the user has seen the chat); only the receipt
// to the sender is withheld.
func TestMarkReadOnOpenClearsBadgeWithoutReceipts(t *testing.T) {
	SendReadReceipts = false
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
			t.Errorf("UnreadCount = %d, want 0: opening a chat clears its badge even without receipts", c.UnreadCount)
		}
	}
}

func TestMarkReadOnArrivalIgnoresOwnMessages(t *testing.T) {
	f := client.NewFake()
	chats, _ := f.Chats(0)
	var before int
	for _, c := range chats {
		if c.JID == "1234567890@s.whatsapp.net" {
			before = c.UnreadCount
		}
	}
	if before == 0 {
		t.Skip("fixture chat has no unread messages")
	}
	MarkReadOnArrival(context.Background(), f, client.Message{ID: "x", ChatJID: "1234567890@s.whatsapp.net", FromMe: true})
	chats, _ = f.Chats(0)
	for _, c := range chats {
		if c.JID == "1234567890@s.whatsapp.net" && c.UnreadCount != before {
			t.Errorf("own message changed UnreadCount %d -> %d", before, c.UnreadCount)
		}
	}
	MarkReadOnArrival(context.Background(), f, client.Message{ID: "y", ChatJID: "1234567890@s.whatsapp.net"})
	chats, _ = f.Chats(0)
	for _, c := range chats {
		if c.JID == "1234567890@s.whatsapp.net" && c.UnreadCount != 0 {
			t.Errorf("inbound arrival left UnreadCount = %d, want 0", c.UnreadCount)
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

func TestComposeStateSubmitContactNoChat(t *testing.T) {
	var s composeState
	if _, ok := s.SubmitContact(client.Contact{DisplayName: "Ada"}); ok {
		t.Error("expected SubmitContact to fail with no active chat")
	}
}

func TestComposeStateSubmitContactClearsReply(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")
	s.StartReply(client.Message{ID: "m1", ChatJID: "a@s.whatsapp.net"})

	contact := client.Contact{DisplayName: "Alan Turing", Phones: []string{"+44 20 7946 0958"}}
	action, ok := s.SubmitContact(contact)
	if !ok {
		t.Fatal("expected SubmitContact to succeed")
	}
	if action.Contact.DisplayName != contact.DisplayName || len(action.Contact.Phones) != 1 {
		t.Errorf("Contact = %+v, want %+v", action.Contact, contact)
	}
	if action.ReplyTo == nil || action.ReplyTo.MsgID != "m1" {
		t.Errorf("ReplyTo = %+v, want m1", action.ReplyTo)
	}
	if _, replying := s.ReplyTarget(); replying {
		t.Error("expected reply to be cleared after SubmitContact")
	}
}

func TestPhoneFromJID(t *testing.T) {
	cases := map[string]string{
		"15551234567@s.whatsapp.net": "+15551234567",
		"":                           "",
		"noatsign":                   "",
		"@s.whatsapp.net":            "",
	}
	for jid, want := range cases {
		if got := phoneFromJID(jid); got != want {
			t.Errorf("phoneFromJID(%q) = %q, want %q", jid, got, want)
		}
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

func TestComposeStateEditFlow(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")

	if _, ok := s.EditTarget(); ok {
		t.Fatal("expected no edit target before StartEdit")
	}

	s.StartEdit(client.Message{ID: "m1", Text: "before"})
	target, ok := s.EditTarget()
	if !ok || target.MsgID != "m1" || target.Text != "before" {
		t.Fatalf("EditTarget = %+v, %v; want the armed message", target, ok)
	}

	action, ok := s.SubmitEdit("  after  ")
	if !ok {
		t.Fatal("expected SubmitEdit to succeed")
	}
	if action.JID != "a@s.whatsapp.net" || action.MsgID != "m1" || action.Text != "after" {
		t.Errorf("action = %+v, want jid/m1/after (trimmed)", action)
	}
	if _, ok := s.EditTarget(); ok {
		t.Error("expected edit mode cleared after a successful SubmitEdit")
	}
}

func TestComposeStateEditCancelAndGuards(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")

	if _, ok := s.SubmitEdit("x"); ok {
		t.Error("expected SubmitEdit to fail when not in edit mode")
	}

	s.StartEdit(client.Message{ID: "m1", Text: "before"})
	if _, ok := s.SubmitEdit("   "); ok {
		t.Error("expected SubmitEdit to fail on blank text")
	}
	s.CancelEdit()
	if _, ok := s.EditTarget(); ok {
		t.Error("expected CancelEdit to clear edit mode")
	}
}

func TestComposeStateEditAndReplyMutuallyExclusive(t *testing.T) {
	var s composeState
	s.SetChat("a@s.whatsapp.net")

	s.StartReply(client.Message{ID: "r1", ChatJID: "a@s.whatsapp.net", Text: "quoted"})
	s.StartEdit(client.Message{ID: "m1", Text: "before"})
	if _, ok := s.ReplyTarget(); ok {
		t.Error("StartEdit should clear a pending reply")
	}

	s.StartReply(client.Message{ID: "r2", ChatJID: "a@s.whatsapp.net", Text: "quoted2"})
	if _, ok := s.EditTarget(); ok {
		t.Error("StartReply should clear a pending edit")
	}
}

func TestSendEnabled(t *testing.T) {
	if sendEnabled("") || sendEnabled("   ") {
		t.Error("send should be disabled for empty/whitespace entry")
	}
	if !sendEnabled("hi") {
		t.Error("send should be enabled once there's text")
	}
}

func TestContactPicksSkipGroupsAndKeepPhones(t *testing.T) {
	chats := []client.Chat{
		{JID: "111@s.whatsapp.net", Name: "Ana", Phone: "111"},
		{JID: "g@g.us", Name: "Team", IsGroup: true},
		{JID: "9@lid", Name: "+55 9", Phone: ""},
		{JID: "n@newsletter", Name: "News"},
		{JID: "status@broadcast", Name: "Status"},
	}
	picks := contactPicks(chats)
	if len(picks) != 2 || picks[0].Phone != "111" || picks[1].JID != "9@lid" {
		t.Errorf("contactPicks = %+v", picks)
	}
	if got := filterContactPicks(picks, "an"); len(got) != 1 || got[0].Name != "Ana" {
		t.Errorf("filter by name = %+v", got)
	}
	if got := filterContactPicks(picks, "11"); len(got) != 1 || got[0].JID != "111@s.whatsapp.net" {
		t.Errorf("filter by phone = %+v", got)
	}
	if got := contactPhoneLabel(""); got != "" {
		t.Errorf("empty phone label = %q", got)
	}
	if got := contactPhoneLabel("5548999"); got != "+5548999" {
		t.Errorf("phone label = %q", got)
	}
	if contactSelectionLabel(0) == contactSelectionLabel(2) {
		t.Error("selection label should change with the count")
	}
}
