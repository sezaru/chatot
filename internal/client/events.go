package client

import (
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// translate maps a raw whatsmeow event to an internal Event. It returns nil
// for events chatot doesn't act on (e.g. events.QR, which is instead
// consumed from GetQRChannel so QR codes have a single delivery path).
func translate(evt interface{}) *Event {
	switch v := evt.(type) {
	case *events.Message:
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

// extractText fills in Text and ReplyTo from the leaf proto message. It
// covers plain and quoted text; media/location/poll extraction is F3's job
// once the store needs to render those message kinds.
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
	}
	if ctx == nil {
		return
	}
	if id := ctx.GetStanzaID(); id != "" {
		msg.ReplyTo = &MsgRef{ChatJID: msg.ChatJID, MsgID: id}
	}
}
