package client

import (
	"testing"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"chatot/internal/store"
)

func TestExtractTextUnwrapsNestedWrappers(t *testing.T) {
	inner := &waProto.Message{ImageMessage: &waProto.ImageMessage{Mimetype: proto.String("image/jpeg"), Caption: proto.String("hi")}}
	m := &waProto.Message{DeviceSentMessage: &waProto.DeviceSentMessage{Message: &waProto.Message{
		EphemeralMessage: &waProto.FutureProofMessage{Message: &waProto.Message{
			ViewOnceMessageV2: &waProto.FutureProofMessage{Message: inner},
		}},
	}}}
	var msg Message
	extractText(m, &msg)
	if msg.Attachment == nil || msg.Attachment.Kind != "image" || msg.Attachment.Caption != "hi" {
		t.Fatalf("nested image not extracted: %+v", msg.Attachment)
	}
	// A DocumentWithCaption wrapper hides an ordinary document.
	msg = Message{}
	extractText(&waProto.Message{DocumentWithCaptionMessage: &waProto.FutureProofMessage{Message: &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{FileName: proto.String("a.pdf"), Caption: proto.String("cap")},
	}}}, &msg)
	if msg.Attachment == nil || msg.Attachment.Kind != "document" || msg.Attachment.Filename != "a.pdf" {
		t.Fatalf("document-with-caption not extracted: %+v", msg.Attachment)
	}
}

func TestExtractTextMoreKinds(t *testing.T) {
	var msg Message
	extractText(&waProto.Message{PtvMessage: &waProto.VideoMessage{Mimetype: proto.String("video/mp4"), Seconds: proto.Uint32(7)}}, &msg)
	if msg.Attachment == nil || msg.Attachment.Kind != "video" || msg.Attachment.DurationSecs != 7 {
		t.Fatalf("ptv: %+v", msg.Attachment)
	}
	msg = Message{}
	extractText(&waProto.Message{PollCreationMessageV3: &waProto.PollCreationMessage{
		Name: proto.String("Lunch?"), Options: []*waProto.PollCreationMessage_Option{{OptionName: proto.String("Yes")}, {OptionName: proto.String("No")}},
	}}, &msg)
	if msg.Poll == nil || msg.Poll.Name != "Lunch?" || len(msg.Poll.Options) != 2 {
		t.Fatalf("poll v3: %+v", msg.Poll)
	}
	msg = Message{}
	extractText(&waProto.Message{GroupInviteMessage: &waProto.GroupInviteMessage{GroupName: proto.String("Trip"), Caption: proto.String("join us")}}, &msg)
	if msg.Text != "👥 Group invite: Trip\njoin us" {
		t.Fatalf("group invite text %q", msg.Text)
	}
	msg = Message{}
	extractText(&waProto.Message{ButtonsMessage: &waProto.ButtonsMessage{ContentText: proto.String("Pick one")}}, &msg)
	if msg.Text != "Pick one" {
		t.Fatalf("buttons text %q", msg.Text)
	}
	msg = Message{}
	extractText(&waProto.Message{PlaceholderMessage: &waProto.PlaceholderMessage{}}, &msg)
	if msg.Text != placeholderText {
		t.Fatalf("placeholder text %q", msg.Text)
	}
}

func TestExtractTextUnsupportedPayloadGetsPlaceholder(t *testing.T) {
	var msg Message
	extractText(&waProto.Message{HighlyStructuredMessage: &waProto.HighlyStructuredMessage{}}, &msg)
	if msg.Text != unsupportedText {
		t.Fatalf("unknown payload: got %q, want the unsupported placeholder", msg.Text)
	}
}

func TestTranslateDropsProtocolOnlyMessages(t *testing.T) {
	info := types.MessageInfo{ID: "k1", MessageSource: types.MessageSource{
		Chat:   types.NewJID("1", types.DefaultUserServer),
		Sender: types.NewJID("1", types.DefaultUserServer),
	}}
	for name, m := range map[string]*waProto.Message{
		"key share":  {ProtocolMessage: &waProto.ProtocolMessage{Type: waProto.ProtocolMessage_APP_STATE_SYNC_KEY_SHARE.Enum()}},
		"sender key": {SenderKeyDistributionMessage: &waProto.SenderKeyDistributionMessage{}},
		"empty":      {},
		"pin":        {PinInChatMessage: &waProto.PinInChatMessage{}},
	} {
		if e := translate(&events.Message{Info: info, Message: m}); e != nil {
			t.Fatalf("%s: got an event %+v, want nil", name, e)
		}
	}
	if e := translate(&events.Message{Info: info, Message: &waProto.Message{Conversation: proto.String("hi")}}); e == nil || e.Message.Text != "hi" {
		t.Fatalf("real text dropped: %+v", e)
	}
}

func TestApplyHistoryMessageStubs(t *testing.T) {
	w := newIngestFixture(t)
	chat := "1@s.whatsapp.net"
	key := func(id string) *waCommon.MessageKey {
		return &waCommon.MessageKey{ID: proto.String(id), FromMe: proto.Bool(true), RemoteJID: proto.String(chat)}
	}
	w.applyHistoryMessage(chat, &waWeb.WebMessageInfo{Key: key("revoked"), MessageTimestamp: proto.Uint64(5), MessageStubType: waWeb.WebMessageInfo_REVOKE.Enum()})
	w.applyHistoryMessage(chat, &waWeb.WebMessageInfo{Key: key("stub"), MessageTimestamp: proto.Uint64(6), MessageStubType: waWeb.WebMessageInfo_GROUP_PARTICIPANT_ADD.Enum()})
	w.applyHistoryMessage(chat, &waWeb.WebMessageInfo{Key: key("keys"), MessageTimestamp: proto.Uint64(7), Message: &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{Type: waProto.ProtocolMessage_HISTORY_SYNC_NOTIFICATION.Enum()},
	}})
	w.applyHistoryMessage(chat, &waWeb.WebMessageInfo{Key: key("text"), MessageTimestamp: proto.Uint64(8), Message: &waProto.Message{Conversation: proto.String("hello")}})

	msgs, err := w.store.Messages(chat, 10)
	if err != nil {
		t.Fatal(err)
	}
	var revoked *store.Message
	for i := range msgs {
		if msgs[i].ID == "revoked" {
			revoked = &msgs[i]
		}
	}
	if revoked == nil || !revoked.Deleted {
		t.Fatalf("revoked stub: %+v, want a deleted tombstone", revoked)
	}
	for _, id := range []string{"stub", "keys"} {
		if _, ok, _ := w.store.MessageByID(chat, id); ok {
			t.Fatalf("%s: stored, want dropped", id)
		}
	}
	if m, ok, _ := w.store.MessageByID(chat, "text"); !ok || m.Text != "hello" {
		t.Fatalf("text: ok=%v %+v", ok, m)
	}
}

// Kinds WhatsApp added lately: each lands as the bubble chatot already has
// for it, or as a line of text, never as "Unsupported".
func TestExtractTextNewerKinds(t *testing.T) {
	var msg Message
	extractText(&waE2E.Message{InstantImageMessage: &waE2E.ImageMessage{Mimetype: proto.String("image/jpeg"), Caption: proto.String("now")}}, &msg)
	if msg.Attachment == nil || msg.Attachment.Kind != "image" || msg.Attachment.Caption != "now" {
		t.Fatalf("instant image: %+v", msg.Attachment)
	}

	msg = Message{}
	extractText(&waE2E.Message{AudioStickerMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{Mimetype: proto.String("image/webp")},
	}}}, &msg)
	if msg.Attachment == nil || msg.Attachment.Kind != "sticker" {
		t.Fatalf("sticker with sound: %+v", msg.Attachment)
	}

	msg = Message{}
	extractText(&waE2E.Message{EventInviteMessage: &waE2E.EventInviteMessage{
		EventTitle: proto.String("Picnic"), Caption: proto.String("bring food"), LocationName: proto.String("Park"),
		StartTime: proto.Int64(1_790_000_000_000), EndTime: proto.Int64(1_790_003_600),
		Status: waE2E.EventInviteMessage_CANCELED.Enum(),
	}}, &msg)
	if ev := msg.EventInvite; ev == nil || ev.Name != "Picnic" || ev.Location != "Park" || ev.StartTS != 1_790_000_000 || ev.EndTS != 1_790_003_600 || !ev.Canceled {
		t.Fatalf("event invite: %+v", msg.EventInvite)
	}

	for _, c := range []struct {
		name string
		m    *waE2E.Message
		want string
	}{
		{"song", &waE2E.Message{MusicMessage: &waE2E.MusicMessage{
			EmbeddedMusic: &waE2E.EmbeddedMusic{Title: proto.String("Garota de Ipanema"), Author: proto.String("Tom Jobim")},
			SongUri:       proto.String("https://open.spotify.com/track/x"),
		}}, "🎵 Garota de Ipanema · Tom Jobim\nhttps://open.spotify.com/track/x"},
		{"untitled song", &waE2E.Message{MusicMessage: &waE2E.MusicMessage{}}, "🎵 Song"},
		{"location request", &waE2E.Message{RequestLocationMessage: &waE2E.RequestLocationMessage{}}, "📍 Location request"},
		{"shared history", &waE2E.Message{MessageHistoryNotice: &waE2E.MessageHistoryNotice{
			MessageHistoryMetadata: &waE2E.MessageHistoryMetadata{MessageCount: proto.Int64(42)},
		}}, "🕘 42 recent messages shared with new members"},
	} {
		msg = Message{}
		extractText(c.m, &msg)
		if msg.Text != c.want {
			t.Errorf("%s: text %q, want %q", c.name, msg.Text, c.want)
		}
	}

	// Bookkeeping that rides along with nothing to read stays out.
	for name, m := range map[string]*waE2E.Message{
		"location request cancelled": {RequestLocationUpdateMessage: &waE2E.RequestLocationUpdateMessage{}},
		"chat privacy setting":       {Acp2SettingMessage: &waE2E.FutureProofMessage{}},
	} {
		msg = Message{}
		extractText(m, &msg)
		if hasContent(&msg) {
			t.Errorf("%s: became a bubble %+v", name, msg)
		}
	}
}
