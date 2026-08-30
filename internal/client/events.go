package client

import (
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// translate maps a raw whatsmeow event to an internal Event. It returns nil
// for events chatot doesn't act on (e.g. events.QR, which is instead
// consumed from GetQRChannel so QR codes have a single delivery path).
func translate(evt interface{}) *Event {
	switch v := evt.(type) {
	case *events.Message:
		if r := v.Message.GetReactionMessage(); r != nil {
			return translateReaction(v, r)
		}
		return translateMessage(v)
	case *events.Receipt:
		read := v.Type == types.ReceiptTypeRead || v.Type == types.ReceiptTypeReadSelf
		return &Event{Kind: EventReceipt, Receipt: &Receipt{
			ChatJID: v.Chat.String(),
			MsgIDs:  append([]types.MessageID(nil), v.MessageIDs...),
			Read:    read,
		}}
	case *events.Presence:
		var lastSeen int64
		if !v.LastSeen.IsZero() {
			lastSeen = v.LastSeen.Unix()
		}
		return &Event{Kind: EventPresence, Presence: &Presence{
			JID:      v.From.String(),
			Online:   !v.Unavailable,
			LastSeen: lastSeen,
		}}
	case *events.ChatPresence:
		return &Event{Kind: EventChatPresence, ChatPresence: &ChatPresence{
			ChatJID: v.Chat.String(),
			JID:     v.Sender.String(),
			State:   string(v.State),
		}}
	case *events.CallOffer:
		return &Event{Kind: EventCall, Call: &Call{
			ChatJID: v.From.String(),
			CallID:  v.CallID,
			Offer:   true,
		}}
	case *events.Connected:
		return &Event{Kind: EventConnection, Connection: &Connection{Connected: true}}
	case *events.Disconnected:
		return &Event{Kind: EventConnection, Connection: &Connection{Connected: false}}
	case *events.LoggedOut:
		return &Event{Kind: EventLoggedOut}
	case *events.PairSuccess:
		return &Event{Kind: EventPairSuccess}
	case *events.HistorySync:
		var jids []string
		if v.Data != nil {
			for _, c := range v.Data.GetConversations() {
				if jid := c.GetID(); jid != "" {
					jids = append(jids, jid)
				}
			}
		}
		return &Event{Kind: EventHistorySync, HistorySync: &HistorySync{ChatJIDs: jids}}
	default:
		return nil
	}
}

func translateMessage(v *events.Message) *Event {
	msg := Message{
		ID:      v.Info.ID,
		ChatJID: v.Info.Chat.String(),
		FromJID: v.Info.Sender.String(),
		FromMe:  v.Info.IsFromMe,
		TS:      v.Info.Timestamp.Unix(),
	}
	extractText(v.Message, &msg)
	return &Event{Kind: EventMessage, Message: &msg}
}

// translateReaction maps a reaction update (delivered as an events.Message
// whose payload is a ReactionMessage, not a separate whatsmeow event type)
// to an internal Reaction. An empty emoji means the reaction was cleared.
func translateReaction(v *events.Message, r *waProto.ReactionMessage) *Event {
	return &Event{Kind: EventReaction, Reaction: &Reaction{
		ChatJID:    v.Info.Chat.String(),
		MsgID:      r.GetKey().GetID(),
		ReactorJID: v.Info.Sender.String(),
		Emoji:      r.GetText(),
		TS:         v.Info.Timestamp.Unix(),
	}}
}

// extractText fills in Text, ReplyTo, Attachment and Location from the leaf
// proto message. It covers plain/quoted text, the common media kinds and
// (live) locations; other rich kinds (poll, contact) are left for later.
func extractText(m *waProto.Message, msg *Message) {
	if m == nil {
		return
	}
	var ctx *waProto.ContextInfo
	switch {
	case m.GetConversation() != "":
		msg.Text = m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		ext := m.GetExtendedTextMessage()
		msg.Text = ext.GetText()
		ctx = ext.GetContextInfo()
	case m.GetImageMessage() != nil:
		img := m.GetImageMessage()
		msg.Attachment = &Attachment{Kind: "image", MimeType: img.GetMimetype(), Caption: img.GetCaption(), ProtoBlob: marshalMedia(img)}
		ctx = img.GetContextInfo()
	case m.GetVideoMessage() != nil:
		vid := m.GetVideoMessage()
		msg.Attachment = &Attachment{Kind: "video", MimeType: vid.GetMimetype(), Caption: vid.GetCaption(), ProtoBlob: marshalMedia(vid)}
		ctx = vid.GetContextInfo()
	case m.GetAudioMessage() != nil:
		aud := m.GetAudioMessage()
		msg.Attachment = &Attachment{Kind: "audio", MimeType: aud.GetMimetype(), ProtoBlob: marshalMedia(aud)}
		ctx = aud.GetContextInfo()
	case m.GetDocumentMessage() != nil:
		doc := m.GetDocumentMessage()
		msg.Attachment = &Attachment{
			Kind: "document", MimeType: doc.GetMimetype(),
			Filename: doc.GetFileName(), Caption: doc.GetCaption(),
			ProtoBlob: marshalMedia(doc),
		}
		ctx = doc.GetContextInfo()
	case m.GetStickerMessage() != nil:
		sticker := m.GetStickerMessage()
		msg.Attachment = &Attachment{Kind: "sticker", MimeType: sticker.GetMimetype(), ProtoBlob: marshalMedia(sticker)}
		ctx = sticker.GetContextInfo()
	case m.GetLocationMessage() != nil:
		loc := m.GetLocationMessage()
		msg.Location = &Location{
			Name: loc.GetName(), Address: loc.GetAddress(),
			Latitude: loc.GetDegreesLatitude(), Longitude: loc.GetDegreesLongitude(),
		}
		ctx = loc.GetContextInfo()
	case m.GetLiveLocationMessage() != nil:
		// Live locations carry no name/address on the wire, only coordinates.
		live := m.GetLiveLocationMessage()
		msg.Location = &Location{
			Latitude: live.GetDegreesLatitude(), Longitude: live.GetDegreesLongitude(),
			IsLive: true,
		}
		ctx = live.GetContextInfo()
	}
	if ctx == nil {
		return
	}
	if id := ctx.GetStanzaID(); id != "" {
		msg.ReplyTo = &MsgRef{ChatJID: msg.ChatJID, MsgID: id}
	}
}

// marshalMedia serializes a media sub-message (ImageMessage, VideoMessage,
// ...) so DownloadMedia can later proto.Unmarshal it back into the concrete
// type whatsmeow.Download needs. Returns nil on marshal failure — download
// on that message will then fail with a clear "no descriptor" error rather
// than a silent corrupt one.
func marshalMedia(m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}
