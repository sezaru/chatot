package client

import (
	"testing"
	"time"

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
