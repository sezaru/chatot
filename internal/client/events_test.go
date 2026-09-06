package client

import (
	"strings"
	"testing"
	"time"

	"chatot/internal/store"

	waBinary "go.mau.fi/whatsmeow/binary"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func mustJID(t *testing.T, s string) types.JID {
	t.Helper()
	jid, err := types.ParseJID(s)
	if err != nil {
		t.Fatalf("parse jid %q: %v", s, err)
	}
	return jid
}

func TestTranslateMessageConversation(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	sender := mustJID(t, "1234567890@s.whatsapp.net")
	ts := time.Unix(1700000000, 0)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsFromMe: false},
			ID:            "ABC123",
			Timestamp:     ts,
		},
		Message: &waProto.Message{Conversation: proto.String("hello there")},
	}

	e := translate(evt)
	if e == nil {
		t.Fatal("translate returned nil")
	}
	if e.Kind != EventMessage {
		t.Fatalf("kind = %v, want EventMessage", e.Kind)
	}
	if e.Message == nil {
		t.Fatal("Message field is nil")
	}
	if e.Message.ID != "ABC123" {
		t.Errorf("ID = %q, want ABC123", e.Message.ID)
	}
	if e.Message.ChatJID != chat.String() {
		t.Errorf("ChatJID = %q, want %q", e.Message.ChatJID, chat.String())
	}
	if e.Message.FromJID != sender.String() {
		t.Errorf("FromJID = %q, want %q", e.Message.FromJID, sender.String())
	}
	if e.Message.FromMe {
		t.Error("FromMe = true, want false")
	}
	if e.Message.Text != "hello there" {
		t.Errorf("Text = %q, want %q", e.Message.Text, "hello there")
	}
	if e.Message.TS != ts.Unix() {
		t.Errorf("TS = %d, want %d", e.Message.TS, ts.Unix())
	}
}

func TestTranslateMessageExtendedTextWithReply(t *testing.T) {
	chat := mustJID(t, "1112223333@s.whatsapp.net")

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat, IsFromMe: true},
			ID:            "XYZ789",
		},
		Message: &waProto.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String("replying to you"),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID: proto.String("orig-id"),
				},
			},
		},
	}

	e := translate(evt)
	if e == nil || e.Message == nil {
		t.Fatal("expected a Message event")
	}
	if e.Message.Text != "replying to you" {
		t.Errorf("Text = %q, want %q", e.Message.Text, "replying to you")
	}
	if e.Message.ReplyTo == nil {
		t.Fatal("ReplyTo is nil, want a MsgRef")
	}
	if e.Message.ReplyTo.MsgID != "orig-id" {
		t.Errorf("ReplyTo.MsgID = %q, want orig-id", e.Message.ReplyTo.MsgID)
	}
	if e.Message.ReplyTo.ChatJID != chat.String() {
		t.Errorf("ReplyTo.ChatJID = %q, want %q", e.Message.ReplyTo.ChatJID, chat.String())
	}
}

func TestTranslateMessageForwardedFlag(t *testing.T) {
	chat := mustJID(t, "1112223333@s.whatsapp.net")

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "XYZ790",
		},
		Message: &waProto.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String("look at this"),
				ContextInfo: &waE2E.ContextInfo{
					IsForwarded: proto.Bool(true),
				},
			},
		},
	}

	e := translate(evt)
	if e == nil || e.Message == nil {
		t.Fatal("expected a Message event")
	}
	if !e.Message.Forwarded {
		t.Error("expected Forwarded=true when ContextInfo.IsForwarded is set")
	}

	notForwarded := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "XYZ791",
		},
		Message: &waProto.Message{Conversation: proto.String("plain")},
	}
	e2 := translate(notForwarded)
	if e2 == nil || e2.Message == nil {
		t.Fatal("expected a Message event")
	}
	if e2.Message.Forwarded {
		t.Error("expected Forwarded=false for a plain message with no ContextInfo")
	}
}

func TestTranslateReaction(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	reactor := mustJID(t, "1234567890@s.whatsapp.net")
	ts := time.Unix(1700000500, 0)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: reactor, IsFromMe: false},
			ID:            "REACT1",
			Timestamp:     ts,
		},
		Message: &waProto.Message{
			ReactionMessage: &waE2E.ReactionMessage{
				Key:  &waCommon.MessageKey{ID: proto.String("target-msg")},
				Text: proto.String("😂"),
			},
		},
	}

	e := translate(evt)
	if e == nil || e.Kind != EventReaction {
		t.Fatalf("expected EventReaction, got %+v", e)
	}
	if e.Reaction == nil {
		t.Fatal("Reaction field is nil")
	}
	if e.Reaction.MsgID != "target-msg" {
		t.Errorf("MsgID = %q, want target-msg", e.Reaction.MsgID)
	}
	if e.Reaction.Emoji != "😂" {
		t.Errorf("Emoji = %q, want 😂", e.Reaction.Emoji)
	}
	if e.Reaction.ReactorJID != reactor.String() {
		t.Errorf("ReactorJID = %q, want %q", e.Reaction.ReactorJID, reactor.String())
	}
	if e.Reaction.ChatJID != chat.String() {
		t.Errorf("ChatJID = %q, want %q", e.Reaction.ChatJID, chat.String())
	}
	if e.Reaction.TS != ts.Unix() {
		t.Errorf("TS = %d, want %d", e.Reaction.TS, ts.Unix())
	}
}

func TestTranslateReactionClear(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "REACT2",
		},
		Message: &waProto.Message{
			ReactionMessage: &waE2E.ReactionMessage{
				Key:  &waCommon.MessageKey{ID: proto.String("target-msg")},
				Text: proto.String(""),
			},
		},
	}

	e := translate(evt)
	if e == nil || e.Kind != EventReaction {
		t.Fatalf("expected EventReaction, got %+v", e)
	}
	if e.Reaction.Emoji != "" {
		t.Errorf("Emoji = %q, want empty (clear)", e.Reaction.Emoji)
	}
	if e.Reaction.MsgID != "target-msg" {
		t.Errorf("MsgID = %q, want target-msg", e.Reaction.MsgID)
	}
}

func TestTranslateMediaMessages(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")

	cases := []struct {
		name     string
		msg      *waProto.Message
		wantKind string
		wantMime string
		wantCap  string
		wantFile string
	}{
		{
			name: "image",
			msg: &waProto.Message{ImageMessage: &waE2E.ImageMessage{
				Mimetype: proto.String("image/jpeg"), Caption: proto.String("a sunset"),
			}},
			wantKind: "image", wantMime: "image/jpeg", wantCap: "a sunset",
		},
		{
			name: "video",
			msg: &waProto.Message{VideoMessage: &waE2E.VideoMessage{
				Mimetype: proto.String("video/mp4"), Caption: proto.String("clip"),
			}},
			wantKind: "video", wantMime: "video/mp4", wantCap: "clip",
		},
		{
			name: "audio",
			msg: &waProto.Message{AudioMessage: &waE2E.AudioMessage{
				Mimetype: proto.String("audio/ogg"),
			}},
			wantKind: "audio", wantMime: "audio/ogg",
		},
		{
			name: "document",
			msg: &waProto.Message{DocumentMessage: &waE2E.DocumentMessage{
				Mimetype: proto.String("application/pdf"),
				FileName: proto.String("invoice.pdf"), Caption: proto.String("the bill"),
			}},
			wantKind: "document", wantMime: "application/pdf", wantFile: "invoice.pdf", wantCap: "the bill",
		},
		{
			name: "sticker",
			msg: &waProto.Message{StickerMessage: &waE2E.StickerMessage{
				Mimetype: proto.String("image/webp"),
			}},
			wantKind: "sticker", wantMime: "image/webp",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evt := &events.Message{
				Info: types.MessageInfo{
					MessageSource: types.MessageSource{Chat: chat, Sender: chat},
					ID:            "M-" + tc.name,
				},
				Message: tc.msg,
			}
			e := translate(evt)
			if e == nil || e.Kind != EventMessage || e.Message == nil {
				t.Fatalf("expected a Message event, got %+v", e)
			}
			a := e.Message.Attachment
			if a == nil {
				t.Fatal("Attachment is nil, want populated")
			}
			if a.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", a.Kind, tc.wantKind)
			}
			if a.MimeType != tc.wantMime {
				t.Errorf("MimeType = %q, want %q", a.MimeType, tc.wantMime)
			}
			if a.Caption != tc.wantCap {
				t.Errorf("Caption = %q, want %q", a.Caption, tc.wantCap)
			}
			if a.Filename != tc.wantFile {
				t.Errorf("Filename = %q, want %q", a.Filename, tc.wantFile)
			}
		})
	}
}

func TestTranslateMediaWithReplyContext(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "M-reply",
		},
		Message: &waProto.Message{ImageMessage: &waE2E.ImageMessage{
			Mimetype:    proto.String("image/png"),
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("quoted-id")},
		}},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.ReplyTo == nil {
		t.Fatalf("expected a Message with ReplyTo, got %+v", e)
	}
	if e.Message.ReplyTo.MsgID != "quoted-id" {
		t.Errorf("ReplyTo.MsgID = %q, want quoted-id", e.Message.ReplyTo.MsgID)
	}
}

func TestTranslateLocationMessage(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "LOC1",
		},
		Message: &waProto.Message{LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(51.9976),
			DegreesLongitude: proto.Float64(-0.7406),
			Name:             proto.String("Bletchley Park"),
			Address:          proto.String("Sherwood Dr"),
			ContextInfo:      &waE2E.ContextInfo{StanzaID: proto.String("quoted-id")},
		}},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.Location == nil {
		t.Fatalf("expected a Message with Location, got %+v", e)
	}
	loc := e.Message.Location
	if loc.Name != "Bletchley Park" || loc.Address != "Sherwood Dr" {
		t.Errorf("name/address = %q/%q", loc.Name, loc.Address)
	}
	if loc.Latitude != 51.9976 || loc.Longitude != -0.7406 {
		t.Errorf("lat/long = %v/%v", loc.Latitude, loc.Longitude)
	}
	if loc.IsLive {
		t.Error("static location should not be IsLive")
	}
	if e.Message.ReplyTo == nil || e.Message.ReplyTo.MsgID != "quoted-id" {
		t.Errorf("ReplyTo = %+v, want quoted-id", e.Message.ReplyTo)
	}
}

func TestTranslateLiveLocationMessage(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "LOC2",
		},
		Message: &waProto.Message{LiveLocationMessage: &waE2E.LiveLocationMessage{
			DegreesLatitude:  proto.Float64(1.5),
			DegreesLongitude: proto.Float64(2.5),
		}},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.Location == nil {
		t.Fatalf("expected a Message with Location, got %+v", e)
	}
	if !e.Message.Location.IsLive {
		t.Error("expected IsLive=true for a live location")
	}
}

func TestTranslateContactMessage(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	vcard := "BEGIN:VCARD\nVERSION:3.0\nN:;Alan Turing;;;\nFN:Alan Turing\nTEL;type=CELL;waid=447900000000:+44 7900 000000\nEND:VCARD"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "CT1",
		},
		Message: &waProto.Message{ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String("Alan Turing"),
			Vcard:       proto.String(vcard),
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("quoted-id")},
		}},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.Contact == nil {
		t.Fatalf("expected a Message with Contact, got %+v", e)
	}
	c := e.Message.Contact
	if c.DisplayName != "Alan Turing" {
		t.Errorf("DisplayName = %q", c.DisplayName)
	}
	if len(c.Phones) != 1 || c.Phones[0] != "+447900000000" {
		t.Errorf("Phones = %v", c.Phones)
	}
	if e.Message.ReplyTo == nil || e.Message.ReplyTo.MsgID != "quoted-id" {
		t.Errorf("ReplyTo = %+v, want quoted-id", e.Message.ReplyTo)
	}
}

func TestTranslateContactsArrayMessage(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	vcard := "BEGIN:VCARD\nFN:Grace Hopper\nTEL:+1 555 0100\nEND:VCARD"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "CT2",
		},
		Message: &waProto.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{
			DisplayName: proto.String("2 contacts"),
			Contacts: []*waE2E.ContactMessage{
				{DisplayName: proto.String("Grace Hopper"), Vcard: proto.String(vcard)},
				{DisplayName: proto.String("Ada Lovelace")},
			},
		}},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.Contact == nil {
		t.Fatalf("expected a Message with Contact, got %+v", e)
	}
	c := e.Message.Contact
	if c.DisplayName != "Grace Hopper" {
		t.Errorf("DisplayName = %q, want first contact's name", c.DisplayName)
	}
	if len(c.Phones) != 1 || c.Phones[0] != "+1 555 0100" {
		t.Errorf("Phones = %v", c.Phones)
	}
}

func TestBuildVCard(t *testing.T) {
	got := buildVCard(Contact{DisplayName: "Alan Turing", Phones: []string{"+44 20 7946 0958", "+1 555 0100"}})
	if !strings.HasPrefix(got, "BEGIN:VCARD\r\nVERSION:3.0\r\n") || !strings.HasSuffix(got, "END:VCARD") {
		t.Fatalf("buildVCard produced malformed envelope: %q", got)
	}
	phones := parseVCardPhones(got)
	if len(phones) != 2 || phones[0] != "+44 20 7946 0958" || phones[1] != "+1 555 0100" {
		t.Errorf("round-tripped phones = %v", phones)
	}
	if !strings.Contains(got, "FN:Alan Turing\r\n") {
		t.Errorf("buildVCard missing FN line: %q", got)
	}
}

func TestParseVCardPhones(t *testing.T) {
	cases := []struct {
		name  string
		vcard string
		want  []string
	}{
		{"single", "BEGIN:VCARD\nTEL;type=CELL:+123\nEND:VCARD", []string{"+123"}},
		{"multiple", "BEGIN:VCARD\nTEL;type=CELL:+123\nTEL;type=HOME:+456\nEND:VCARD", []string{"+123", "+456"}},
		{"waid", "BEGIN:VCARD\nTEL;type=CELL;waid=554899010873:+55 48\u00a09901\u20110873\nEND:VCARD", []string{"+554899010873"}},
		{"none", "BEGIN:VCARD\nFN:No Phone\nEND:VCARD", nil},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVCardPhones(tc.vcard)
			if len(got) != len(tc.want) {
				t.Fatalf("parseVCardPhones(%q) = %v, want %v", tc.vcard, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("phone[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestContactStoreRoundTrip(t *testing.T) {
	m := &Message{
		ID: "CT1", ChatJID: "a@s.whatsapp.net",
		Contact: &Contact{DisplayName: "Alan Turing", Phones: []string{"+44 7900 000000"}},
	}
	row := storeMessageRow(m)
	if row.Kind != "contact" {
		t.Fatalf("Kind = %q, want contact", row.Kind)
	}
	if row.Payload == "" {
		t.Fatal("expected a non-empty payload")
	}

	back := messageFromStore(store.Message{
		ID: row.MsgID, ChatJID: row.ChatJID, Kind: row.Kind, Payload: row.Payload,
	}, "")
	if back.Contact == nil {
		t.Fatal("expected Contact to decode back")
	}
	if back.Contact.DisplayName != "Alan Turing" || len(back.Contact.Phones) != 1 ||
		back.Contact.Phones[0] != "+44 7900 000000" {
		t.Errorf("round-trip mismatch: %+v", back.Contact)
	}
}

func TestLocationStoreRoundTrip(t *testing.T) {
	m := &Message{
		ID: "LOC1", ChatJID: "a@s.whatsapp.net",
		Location: &Location{Name: "Home", Address: "1 Main St", Latitude: 51.5, Longitude: -0.12},
	}
	row := storeMessageRow(m)
	if row.Kind != "location" {
		t.Fatalf("Kind = %q, want location", row.Kind)
	}
	if row.Payload == "" {
		t.Fatal("expected a non-empty payload")
	}

	back := messageFromStore(store.Message{
		ID: row.MsgID, ChatJID: row.ChatJID, Kind: row.Kind, Payload: row.Payload,
	}, "")
	if back.Location == nil {
		t.Fatal("expected Location to decode back")
	}
	if back.Location.Name != "Home" || back.Location.Address != "1 Main St" ||
		back.Location.Latitude != 51.5 || back.Location.Longitude != -0.12 {
		t.Errorf("round-trip mismatch: %+v", back.Location)
	}
}

func TestLiveLocationStoreRoundTrip(t *testing.T) {
	m := &Message{
		ID: "LOC2", ChatJID: "a@s.whatsapp.net",
		Location: &Location{Latitude: 51.5, Longitude: -0.12, IsLive: true, LiveUntil: 1735689600},
	}
	row := storeMessageRow(m)
	if row.Kind != "location" {
		t.Fatalf("Kind = %q, want location", row.Kind)
	}

	back := messageFromStore(store.Message{
		ID: row.MsgID, ChatJID: row.ChatJID, Kind: row.Kind, Payload: row.Payload,
	}, "")
	if back.Location == nil {
		t.Fatal("expected Location to decode back")
	}
	if !back.Location.IsLive || back.Location.LiveUntil != 1735689600 ||
		back.Location.Latitude != 51.5 || back.Location.Longitude != -0.12 {
		t.Errorf("round-trip mismatch: %+v", back.Location)
	}
}

func TestTranslateEventMessage(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "EV1",
		},
		Message: &waProto.Message{EventMessage: &waE2E.EventMessage{
			Name:        proto.String("Team offsite"),
			Description: proto.String("Bring laptops"),
			Location:    &waE2E.LocationMessage{Name: proto.String("Bletchley Park")},
			StartTime:   proto.Int64(1735689600),
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("quoted-id")},
		}},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.EventInvite == nil {
		t.Fatalf("expected a Message with EventInvite, got %+v", e)
	}
	ei := e.Message.EventInvite
	if ei.Name != "Team offsite" || ei.Description != "Bring laptops" {
		t.Errorf("name/description = %q/%q", ei.Name, ei.Description)
	}
	if ei.Location != "Bletchley Park" {
		t.Errorf("Location = %q, want Bletchley Park", ei.Location)
	}
	if ei.StartTS != 1735689600 {
		t.Errorf("StartTS = %d, want 1735689600", ei.StartTS)
	}
	if ei.Canceled {
		t.Error("expected Canceled=false")
	}
	if e.Message.ReplyTo == nil || e.Message.ReplyTo.MsgID != "quoted-id" {
		t.Errorf("ReplyTo = %+v, want quoted-id", e.Message.ReplyTo)
	}
}

func TestEventStoreRoundTrip(t *testing.T) {
	m := &Message{
		ID: "EV1", ChatJID: "a@s.whatsapp.net",
		EventInvite: &EventInvite{
			Name: "Team offsite", Description: "Bring laptops", Location: "Bletchley Park",
			StartTS: 1735689600, EndTS: 1735693200,
		},
	}
	row := storeMessageRow(m)
	if row.Kind != "event" {
		t.Fatalf("Kind = %q, want event", row.Kind)
	}
	if row.Payload == "" {
		t.Fatal("expected a non-empty payload")
	}

	back := messageFromStore(store.Message{
		ID: row.MsgID, ChatJID: row.ChatJID, Kind: row.Kind, Payload: row.Payload,
	}, "")
	if back.EventInvite == nil {
		t.Fatal("expected EventInvite to decode back")
	}
	if back.EventInvite.Name != "Team offsite" || back.EventInvite.Description != "Bring laptops" ||
		back.EventInvite.Location != "Bletchley Park" ||
		back.EventInvite.StartTS != 1735689600 || back.EventInvite.EndTS != 1735693200 {
		t.Errorf("round-trip mismatch: %+v", back.EventInvite)
	}
}

func TestTranslateReceipt(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Receipt{
		MessageSource: types.MessageSource{Chat: chat},
		MessageIDs:    []types.MessageID{"m1", "m2"},
		Type:          types.ReceiptTypeRead,
	}

	e := translate(evt)
	if e == nil || e.Kind != EventReceipt {
		t.Fatalf("expected EventReceipt, got %+v", e)
	}
	if !e.Receipt.Read {
		t.Error("Read = false, want true for ReceiptTypeRead")
	}
	if e.Receipt.ChatJID != chat.String() {
		t.Errorf("ChatJID = %q, want %q", e.Receipt.ChatJID, chat.String())
	}
	if len(e.Receipt.MsgIDs) != 2 || e.Receipt.MsgIDs[0] != "m1" || e.Receipt.MsgIDs[1] != "m2" {
		t.Errorf("MsgIDs = %v, want [m1 m2]", e.Receipt.MsgIDs)
	}
}

func TestTranslateReceiptDelivered(t *testing.T) {
	evt := &events.Receipt{Type: types.ReceiptTypeDelivered}
	e := translate(evt)
	if e == nil || e.Kind != EventReceipt {
		t.Fatalf("expected EventReceipt, got %+v", e)
	}
	if e.Receipt.Read {
		t.Error("Read = true, want false for ReceiptTypeDelivered")
	}
	if e.Receipt.Status != MessageStatusDelivered {
		t.Errorf("Status = %d, want %d (delivered)", e.Receipt.Status, MessageStatusDelivered)
	}
}

func TestTranslateReceiptStatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		typ        types.ReceiptType
		wantStatus int
		wantRead   bool
	}{
		{"delivered", types.ReceiptTypeDelivered, MessageStatusDelivered, false},
		{"read", types.ReceiptTypeRead, MessageStatusRead, true},
		{"read-self", types.ReceiptTypeReadSelf, MessageStatusRead, true},
		{"played", types.ReceiptTypePlayed, MessageStatusRead, true},
		{"played-self", types.ReceiptTypePlayedSelf, MessageStatusRead, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := translate(&events.Receipt{Type: c.typ, MessageIDs: []types.MessageID{"m1"}})
			if e == nil || e.Kind != EventReceipt {
				t.Fatalf("expected EventReceipt, got %+v", e)
			}
			if e.Receipt.Status != c.wantStatus {
				t.Errorf("Status = %d, want %d", e.Receipt.Status, c.wantStatus)
			}
			if e.Receipt.Read != c.wantRead {
				t.Errorf("Read = %v, want %v", e.Receipt.Read, c.wantRead)
			}
		})
	}
}

func TestTranslateReceiptIgnoredKinds(t *testing.T) {
	for _, typ := range []types.ReceiptType{types.ReceiptTypeSender, types.ReceiptTypeRetry, types.ReceiptTypeServerError, types.ReceiptTypeHistorySync} {
		if e := translate(&events.Receipt{Type: typ}); e != nil {
			t.Errorf("translate(%q) = %+v, want nil (not a delivery/read state)", typ, e)
		}
	}
}

func TestTranslateConnected(t *testing.T) {
	e := translate(&events.Connected{})
	if e == nil || e.Kind != EventConnection {
		t.Fatalf("expected EventConnection, got %+v", e)
	}
	if !e.Connection.Connected {
		t.Error("Connected = false, want true")
	}
}

func TestTranslateDisconnected(t *testing.T) {
	e := translate(&events.Disconnected{})
	if e == nil || e.Kind != EventConnection {
		t.Fatalf("expected EventConnection, got %+v", e)
	}
	if e.Connection.Connected {
		t.Error("Connected = true, want false")
	}
}

func TestTranslateLoggedOut(t *testing.T) {
	e := translate(&events.LoggedOut{OnConnect: true, Reason: events.ConnectFailureLoggedOut})
	if e == nil || e.Kind != EventLoggedOut {
		t.Fatalf("expected EventLoggedOut, got %+v", e)
	}
}

func TestTranslatePairSuccess(t *testing.T) {
	e := translate(&events.PairSuccess{ID: mustJID(t, "1234567890@s.whatsapp.net")})
	if e == nil || e.Kind != EventPairSuccess {
		t.Fatalf("expected EventPairSuccess, got %+v", e)
	}
}

func TestTranslateUnknownEventIsNoop(t *testing.T) {
	if e := translate(&events.KeepAliveTimeout{}); e != nil {
		t.Fatalf("expected nil for unhandled event, got %+v", e)
	}
}

func TestTranslateQRIsSkipped(t *testing.T) {
	// QR codes are delivered exclusively via QRCodes()/GetQRChannel; translate
	// must not also surface them as an Event to avoid double delivery.
	if e := translate(&events.QR{Codes: []string{"code1"}}); e != nil {
		t.Fatalf("expected nil for QR event, got %+v", e)
	}
}

func TestTranslatePollCreationMessage(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "POLL1",
		},
		Message: &waProto.Message{PollCreationMessage: &waE2E.PollCreationMessage{
			Name: proto.String("Lunch?"),
			Options: []*waE2E.PollCreationMessage_Option{
				{OptionName: proto.String("Pizza")},
				{OptionName: proto.String("Sushi")},
			},
			SelectableOptionsCount: proto.Uint32(1),
		}},
	}
	e := translate(evt)
	if e == nil || e.Message == nil || e.Message.Poll == nil {
		t.Fatalf("expected a Message with Poll, got %+v", e)
	}
	p := e.Message.Poll
	if p.Name != "Lunch?" || p.SelectableCount != 1 || len(p.Options) != 2 {
		t.Fatalf("unexpected poll %+v", p)
	}
	if p.Options[0].Name != "Pizza" || p.Options[1].Name != "Sushi" {
		t.Fatalf("unexpected options %+v", p.Options)
	}
}

func TestTranslatePollUpdateReturnsNil(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: chat}, ID: "V1"},
		Message: &waProto.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}},
	}
	if e := translate(evt); e != nil {
		t.Fatalf("poll-update message should translate to nil, got %+v", e)
	}
}

func TestPollStoreRoundTripTallies(t *testing.T) {
	m := &Message{
		ID: "POLL1", ChatJID: "a@s.whatsapp.net",
		Poll: &Poll{Name: "Lunch?", SelectableCount: 1, Options: []PollOption{{Name: "Pizza"}, {Name: "Sushi"}}},
	}
	row := storeMessageRow(m)
	if row.Kind != "poll" || row.Payload == "" {
		t.Fatalf("expected poll kind + payload, got kind=%q payload=%q", row.Kind, row.Payload)
	}

	self := "me@s.whatsapp.net"
	back := messageFromStore(store.Message{
		ID: row.MsgID, ChatJID: row.ChatJID, Kind: row.Kind, Payload: row.Payload,
		PollVotes: []store.PollVoteRow{
			{VoterJID: "friend@s.whatsapp.net", OptionHash: hashPollOption("Pizza")},
			{VoterJID: self, OptionHash: hashPollOption("Pizza")},
			{VoterJID: "other@s.whatsapp.net", OptionHash: hashPollOption("Sushi")},
		},
	}, self)
	if back.Poll == nil {
		t.Fatal("expected Poll to decode back")
	}
	if back.Poll.Name != "Lunch?" || back.Poll.SelectableCount != 1 {
		t.Fatalf("poll definition mismatch: %+v", back.Poll)
	}
	if back.Poll.Options[0].Count != 2 || back.Poll.Options[1].Count != 1 {
		t.Fatalf("tally mismatch: %+v", back.Poll.Options)
	}
	if !back.Poll.Options[0].Voted {
		t.Fatal("expected Pizza to be marked Voted (self voted for it)")
	}
	if back.Poll.Options[1].Voted {
		t.Fatal("Sushi should not be marked Voted for self")
	}
}

func TestTranslateMessageLiveEdit(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	sender := mustJID(t, "1234567890@s.whatsapp.net")

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: sender},
			ID:            "EDIT-STANZA-ID",
			Timestamp:     time.Unix(1700000100, 0),
		},
		Message: &waProto.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Key:           &waCommon.MessageKey{ID: proto.String("ORIGINAL-ID")},
			Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			EditedMessage: &waE2E.Message{Conversation: proto.String("fixed typo")},
		}},
	}

	e := translate(evt)
	if e == nil || e.Message == nil {
		t.Fatal("translate returned nil for a live edit")
	}
	if !e.Message.Edited {
		t.Error("Edited = false, want true")
	}
	if e.Message.ID != "ORIGINAL-ID" {
		t.Errorf("ID = %q, want the original id from the ProtocolMessage key", e.Message.ID)
	}
	if e.Message.Text != "fixed typo" {
		t.Errorf("Text = %q, want the edited content", e.Message.Text)
	}
}

func TestTranslateMessageHistoryEdit(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	sender := mustJID(t, "1234567890@s.whatsapp.net")

	// History path: whatsmeow already rewrote Info.ID to the original and
	// unwrapped the edited content, leaving only Info.Edit to mark it.
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: sender},
			ID:            "ORIGINAL-ID",
			Edit:          types.EditAttributeMessageEdit,
			Timestamp:     time.Unix(1700000100, 0),
		},
		Message: &waProto.Message{Conversation: proto.String("history edited")},
	}

	e := translate(evt)
	if e == nil || e.Message == nil {
		t.Fatal("translate returned nil for a history edit")
	}
	if !e.Message.Edited {
		t.Error("Edited = false, want true")
	}
	if e.Message.ID != "ORIGINAL-ID" || e.Message.Text != "history edited" {
		t.Errorf("got ID=%q Text=%q, want original id + edited text", e.Message.ID, e.Message.Text)
	}
}

func TestTranslateRevoke(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	sender := mustJID(t, "1234567890@s.whatsapp.net")

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: sender},
			ID:            "REVOKE-STANZA-ID",
			Timestamp:     time.Unix(1700000200, 0),
		},
		Message: &waProto.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Key:  &waCommon.MessageKey{ID: proto.String("REVOKED-ID")},
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		}},
	}

	e := translate(evt)
	if e == nil {
		t.Fatal("translate returned nil for a revoke")
	}
	if e.Kind != EventRevoke {
		t.Fatalf("Kind = %v, want EventRevoke", e.Kind)
	}
	if e.Revoke == nil {
		t.Fatal("Revoke is nil")
	}
	if e.Revoke.MsgID != "REVOKED-ID" {
		t.Errorf("MsgID = %q, want the id from the ProtocolMessage key, not the revoke's own stanza id", e.Revoke.MsgID)
	}
	if e.Revoke.ChatJID != chat.String() {
		t.Errorf("ChatJID = %q, want %q", e.Revoke.ChatJID, chat.String())
	}
	// A REVOKE must never fall through to translateMessage/extractText: that
	// would produce a blank EventMessage instead of an EventRevoke.
	if e.Message != nil {
		t.Error("Message is non-nil; a revoke must not also be routed as a message")
	}
}

func TestTranslateMessageStoresBareSender(t *testing.T) {
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   mustJID(t, "554884131986-1482356741@g.us"),
				Sender: mustJID(t, "257157073207386:27@lid"),
			},
			ID:        "DEV1",
			Timestamp: time.Unix(1700000000, 0),
		},
		Message: &waProto.Message{Conversation: proto.String("oi")},
	}
	e := translate(evt)
	if e == nil || e.Message == nil {
		t.Fatal("translate returned no message")
	}
	if e.Message.FromJID != "257157073207386@lid" {
		t.Errorf("FromJID = %q, want the sender without its device part", e.Message.FromJID)
	}
}

func callMeta(t *testing.T, from string, ts time.Time) types.BasicCallMeta {
	t.Helper()
	return types.BasicCallMeta{
		From: mustJID(t, from), CallCreator: mustJID(t, from),
		Timestamp: ts, CallID: "CALL1",
	}
}

func TestTranslateCallOfferVoice(t *testing.T) {
	ts := time.Unix(1700000600, 0)
	evt := &events.CallOffer{
		BasicCallMeta: callMeta(t, "1234567890@s.whatsapp.net", ts),
		Data:          &waBinary.Node{Tag: "offer", Content: []waBinary.Node{{Tag: "audio"}, {Tag: "audio"}}},
	}
	e := translate(evt)
	if e == nil || e.Kind != EventCall || e.Call == nil {
		t.Fatalf("expected EventCall, got %+v", e)
	}
	c := e.Call
	if !c.Offer || c.Video || c.Outcome != "" {
		t.Errorf("offer=%v video=%v outcome=%q, want a voice offer", c.Offer, c.Video, c.Outcome)
	}
	if c.ChatJID != "1234567890@s.whatsapp.net" || c.CallerJID != "1234567890@s.whatsapp.net" {
		t.Errorf("chat=%q caller=%q", c.ChatJID, c.CallerJID)
	}
	if c.CallID != "CALL1" || c.TS != ts.Unix() {
		t.Errorf("id=%q ts=%d, want CALL1 at %d", c.CallID, c.TS, ts.Unix())
	}
}

func TestTranslateCallOfferVideo(t *testing.T) {
	evt := &events.CallOffer{
		BasicCallMeta: callMeta(t, "1234567890@s.whatsapp.net", time.Unix(1, 0)),
		Data:          &waBinary.Node{Tag: "offer", Content: []waBinary.Node{{Tag: "audio"}, {Tag: "video"}}},
	}
	e := translate(evt)
	if e == nil || e.Call == nil || !e.Call.Video {
		t.Fatalf("expected a video offer, got %+v", e)
	}
}

func TestTranslateGroupCallOfferNotice(t *testing.T) {
	meta := callMeta(t, "1234567890@s.whatsapp.net", time.Unix(1, 0))
	meta.GroupJID = mustJID(t, "120363000000000000@g.us")
	e := translate(&events.CallOfferNotice{BasicCallMeta: meta, Media: "video", Type: "group"})
	if e == nil || e.Call == nil {
		t.Fatalf("expected EventCall, got %+v", e)
	}
	if e.Call.ChatJID != "120363000000000000@g.us" || !e.Call.Offer || !e.Call.Video {
		t.Errorf("got %+v, want a video offer filed under the group", e.Call)
	}
	if e.Call.CallerJID != "1234567890@s.whatsapp.net" {
		t.Errorf("caller = %q, want the call creator", e.Call.CallerJID)
	}
}

func TestTranslateCallSignalsSettleOutcome(t *testing.T) {
	meta := callMeta(t, "1234567890@s.whatsapp.net", time.Unix(1, 0))
	cases := []struct {
		name string
		evt  interface{}
		want string
	}{
		{"accept", &events.CallAccept{BasicCallMeta: meta}, CallAnswered},
		{"terminate timeout", &events.CallTerminate{BasicCallMeta: meta, Reason: "timeout"}, CallMissed},
		{"terminate other", &events.CallTerminate{BasicCallMeta: meta, Reason: "busy"}, ""},
		{"reject", &events.CallReject{BasicCallMeta: meta}, CallDeclined},
	}
	for _, tc := range cases {
		e := translate(tc.evt)
		if e == nil || e.Kind != EventCall || e.Call == nil {
			t.Fatalf("%s: expected EventCall, got %+v", tc.name, e)
		}
		if e.Call.Offer {
			t.Errorf("%s: Offer = true, want false", tc.name)
		}
		if e.Call.Outcome != tc.want {
			t.Errorf("%s: Outcome = %q, want %q", tc.name, e.Call.Outcome, tc.want)
		}
	}
}

func TestTranslateCallLogMessage(t *testing.T) {
	chat := mustJID(t, "1234567890@s.whatsapp.net")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "CL1", Timestamp: time.Unix(1700000700, 0),
		},
		Message: &waProto.Message{
			CallLogMesssage: &waE2E.CallLogMessage{
				IsVideo:      proto.Bool(true),
				CallOutcome:  waE2E.CallLogMessage_CONNECTED.Enum(),
				DurationSecs: proto.Int64(151),
			},
		},
	}
	e := translate(evt)
	if e == nil || e.Kind != EventMessage || e.Message == nil || e.Message.CallLog == nil {
		t.Fatalf("expected a call-log message, got %+v", e)
	}
	cl := e.Message.CallLog
	if !cl.Video || cl.Outcome != CallAnswered || cl.DurationSecs != 151 {
		t.Errorf("got %+v, want an answered 151s video call", cl)
	}

	outcomes := map[waE2E.CallLogMessage_CallOutcome]string{
		waE2E.CallLogMessage_MISSED:             CallMissed,
		waE2E.CallLogMessage_SILENCED_BY_DND:    CallMissed,
		waE2E.CallLogMessage_REJECTED:           CallDeclined,
		waE2E.CallLogMessage_FAILED:             CallFailed,
		waE2E.CallLogMessage_ACCEPTED_ELSEWHERE: CallAnswered,
	}
	for in, want := range outcomes {
		got := translateCallLog(&waE2E.CallLogMessage{CallOutcome: in.Enum()})
		if got.Outcome != want {
			t.Errorf("outcome %v -> %q, want %q", in, got.Outcome, want)
		}
	}
}

func TestCallStoreRoundTrip(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in := &Message{
		ID: "call:C1", ChatJID: "1234567890@s.whatsapp.net", FromJID: "1234567890@s.whatsapp.net", TS: 100,
		CallLog: &CallLog{Video: true, Outcome: CallAnswered, DurationSecs: 42},
	}
	if err := s.UpsertMessage(storeMessageRow(in)); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Messages(in.ChatJID, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Messages: %v, %d rows", err, len(rows))
	}
	out := messageFromStore(rows[0], "")
	if out.CallLog == nil || *out.CallLog != *in.CallLog {
		t.Fatalf("round trip = %+v, want %+v", out.CallLog, in.CallLog)
	}
	if rows[0].Kind != "call" {
		t.Errorf("kind = %q, want call", rows[0].Kind)
	}
}

func TestCallIsStale(t *testing.T) {
	now := time.Unix(1700000000, 0)
	if callIsStale(now.Add(-5*time.Second).Unix(), now, true) {
		t.Error("a five-second-old offer is live even inside the sync window")
	}
	if !callIsStale(now.Add(-58*time.Minute).Unix(), now, false) {
		t.Error("an offer from 58 minutes ago was replayed and must not ring")
	}
	if !callIsStale(now.Add(-3*time.Minute).Unix(), now, false) {
		t.Error("an offer older than a ring's length is stale")
	}
	if callIsStale(0, now, false) || !callIsStale(0, now, true) {
		t.Error("an unstamped signal follows the sync window")
	}
}

func TestReactionText(t *testing.T) {
	if got := ReactionText("", "👍", "see you at 7"); got != `Reacted 👍 to "see you at 7"` {
		t.Errorf("DM: got %q", got)
	}
	if got := ReactionText("Alice", "😂", "📷 Sunset"); got != `Alice: Reacted 😂 to "📷 Sunset"` {
		t.Errorf("group: got %q", got)
	}
	if got := ReactionText("", "❤️", ""); got != "Reacted ❤️ to " {
		t.Errorf("unknown target: got %q", got)
	}
}
