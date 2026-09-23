package client

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"chatot/internal/store"
)

func TestQuoteParticipant(t *testing.T) {
	ownPN := types.JID{User: "554888073648", Device: 59, Server: types.DefaultUserServer}
	ownLID := types.JID{User: "64081113427987", Device: 45, Server: types.HiddenUserServer}
	cases := []struct {
		name   string
		quoted store.Message
		lid    bool
		want   string
	}{
		// The bug: our own message went out with no participant, and the
		// recipient's WhatsApp attributed the quote to its own user.
		{"own in a LID group", store.Message{FromMe: true, FromJID: "554888073648:59@s.whatsapp.net"}, true, "64081113427987@lid"},
		{"own in a PN chat", store.Message{FromMe: true, FromJID: "554888073648:59@s.whatsapp.net"}, false, "554888073648@s.whatsapp.net"},
		{"other, device suffix dropped", store.Message{FromJID: "257157073207386:3@lid"}, true, "257157073207386@lid"},
		{"other, PN", store.Message{FromJID: "1234567890@s.whatsapp.net"}, false, "1234567890@s.whatsapp.net"},
		{"other, unknown sender", store.Message{}, false, ""},
	}
	for _, c := range cases {
		if got := quoteParticipant(c.quoted, ownPN, ownLID, c.lid); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	// Without a LID of our own, the phone number is the only name we have.
	if got := quoteParticipant(store.Message{FromMe: true}, ownPN, types.EmptyJID, true); got != "554888073648@s.whatsapp.net" {
		t.Errorf("own without a LID: got %q", got)
	}
}

func TestChatAddressesByLID(t *testing.T) {
	w := newIngestFixture(t)
	lidGroup, pnGroup := "1-1@g.us", "2-2@g.us"
	must(t, w.store.UpsertMessage(store.MessageRow{ChatJID: lidGroup, MsgID: "a", FromJID: "257157073207386@lid", Text: "x", TS: 1}))
	must(t, w.store.UpsertMessage(store.MessageRow{ChatJID: lidGroup, MsgID: "b", FromJID: "554888073648:59@s.whatsapp.net", FromMe: true, Text: "y", TS: 2}))
	must(t, w.store.UpsertMessage(store.MessageRow{ChatJID: pnGroup, MsgID: "c", FromJID: "1234567890@s.whatsapp.net", Text: "z", TS: 1}))

	for jid, want := range map[string]bool{
		lidGroup:                    true,
		pnGroup:                     false,
		"257157073207386@lid":       true,
		"1234567890@s.whatsapp.net": false,
		"3-3@g.us":                  false, // nobody else has spoken yet
	} {
		if got := w.chatAddressesByLID(context.Background(), jid); got != want {
			t.Errorf("chatAddressesByLID(%s) = %v, want %v", jid, got, want)
		}
	}
}

func TestTranslateReplyKeepsQuotedAuthor(t *testing.T) {
	chat := mustJID(t, "120363000000000000@g.us")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: mustJID(t, "257672368619718@lid"), IsGroup: true},
			ID:            "R1",
		},
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String("Bem lembrado"),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:      proto.String("orig"),
					Participant:   proto.String("257157073207386@lid"),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("viu a entrevista?")},
				},
			},
		},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.ReplyTo == nil {
		t.Fatalf("expected a reply, got %+v", e)
	}
	if got := e.Message.ReplyTo.FromJID; got != "257157073207386@lid" {
		t.Errorf("ReplyTo.FromJID = %q, want the quoted participant", got)
	}
}

func TestReplyAuthorRoundTripsThroughStore(t *testing.T) {
	w := newIngestFixture(t)
	chat := "1-1@g.us"
	m := Message{ID: "r", ChatJID: chat, FromJID: "257672368619718@lid", Text: "reply", TS: 2,
		ReplyTo: &MsgRef{ChatJID: chat, MsgID: "gone", Text: "old", FromJID: "257157073207386:7@lid"}}
	must(t, w.ingestMessage(&m))

	msgs, err := w.store.Messages(chat, 10)
	must(t, err)
	if len(msgs) != 1 || msgs[0].ReplyToFrom != "257157073207386@lid" {
		t.Fatalf("stored ReplyToFrom = %+v, want the non-AD author", msgs)
	}
	if out := messageFromStore(msgs[0], ""); out.ReplyTo == nil || out.ReplyTo.FromJID != "257157073207386@lid" {
		t.Errorf("messageFromStore ReplyTo = %+v", out.ReplyTo)
	}
}
