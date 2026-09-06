package client

import (
	"fmt"
	"strings"

	waBinary "go.mau.fi/whatsmeow/binary"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waE2E"
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
		// Poll votes are decrypted + tallied out-of-band in whatsmeow.go's
		// handleRaw; the pure translate returns nil so nothing double-handles.
		if v.Message.GetPollUpdateMessage() != nil {
			return nil
		}
		if pm := v.Message.GetProtocolMessage(); pm != nil && pm.GetType() == waProto.ProtocolMessage_REVOKE {
			return &Event{Kind: EventRevoke, Revoke: &Revoke{
				ChatJID: v.Info.Chat.String(),
				MsgID:   pm.GetKey().GetID(),
				TS:      v.Info.Timestamp.Unix(),
			}}
		}
		if r := v.Message.GetReactionMessage(); r != nil {
			return translateReaction(v, r)
		}
		return translateMessage(v)
	case *events.Receipt:
		var status int
		switch v.Type {
		case types.ReceiptTypeDelivered:
			status = MessageStatusDelivered
		case types.ReceiptTypeRead, types.ReceiptTypeReadSelf, types.ReceiptTypePlayed, types.ReceiptTypePlayedSelf:
			status = MessageStatusRead
		default:
			// sender/retry/server-error/hist_sync/... acks aren't a
			// delivery/read state chatot tracks.
			return nil
		}
		r := &Receipt{
			ChatJID: v.Chat.String(),
			MsgIDs:  append([]types.MessageID(nil), v.MessageIDs...),
			Read:    status == MessageStatusRead,
			Status:  status,
			TS:      v.Timestamp.Unix(),
		}
		// Someone else read our message (a self receipt is our other
		// device catching up, not a reader). whatsmeow always fills Sender
		// on a receipt (per-participant for grouped ones), and a status view
		// arrives addressed from the viewer, so the sender is the reader.
		if (v.Type == types.ReceiptTypeRead || v.Type == types.ReceiptTypePlayed) && !v.Sender.IsEmpty() {
			r.ReaderJID = v.Sender.String()
		}
		return &Event{Kind: EventReceipt, Receipt: r}
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
			Media:   string(v.Media),
		}}
	case *events.CallOffer:
		c := translateCallMeta(v.BasicCallMeta)
		c.Offer = true
		c.Video = callOfferIsVideo(v.Data)
		return &Event{Kind: EventCall, Call: c}
	case *events.CallOfferNotice:
		// A group call rings as an offer notice rather than an offer.
		c := translateCallMeta(v.BasicCallMeta)
		c.Offer = true
		c.Video = v.Media == "video"
		return &Event{Kind: EventCall, Call: c}
	case *events.CallAccept:
		c := translateCallMeta(v.BasicCallMeta)
		c.Outcome = CallAnswered
		return &Event{Kind: EventCall, Call: c}
	case *events.CallTerminate:
		c := translateCallMeta(v.BasicCallMeta)
		c.Outcome = callTerminateOutcome(v.Reason)
		return &Event{Kind: EventCall, Call: c}
	case *events.CallReject:
		c := translateCallMeta(v.BasicCallMeta)
		c.Outcome = CallDeclined
		return &Event{Kind: EventCall, Call: c}
	case *events.Connected:
		return &Event{Kind: EventConnection, Connection: &Connection{Connected: true}}
	case *events.Disconnected:
		return &Event{Kind: EventConnection, Connection: &Connection{Connected: false}}
	case *events.LoggedOut:
		return &Event{Kind: EventLoggedOut}
	case *events.PairSuccess:
		return &Event{Kind: EventPairSuccess}
	case *events.HistorySync:
		return &Event{Kind: EventHistorySync, HistorySync: historySyncSummary(v.Data)}
	default:
		return nil
	}
}

func translateMessage(v *events.Message) *Event {
	// A message edit arrives either as a MESSAGE_EDIT ProtocolMessage (live
	// path) or, on the history path, already unwrapped with only Info.Edit set.
	// Derive the original message id + edited content explicitly: the live
	// path leaves Info.ID as the edit's own id, so never trust it as the
	// original — read it from the ProtocolMessage key when present.
	content := v.Message
	id := v.Info.ID
	pm := v.Message.GetProtocolMessage()
	edited := v.Info.Edit == types.EditAttributeMessageEdit ||
		(pm != nil && pm.GetType() == waProto.ProtocolMessage_MESSAGE_EDIT)
	if edited && pm != nil {
		if k := pm.GetKey(); k.GetID() != "" {
			id = k.GetID()
		}
		if em := pm.GetEditedMessage(); em != nil {
			content = em
		}
	}

	msg := Message{
		ID:      id,
		ChatJID: v.Info.Chat.String(),
		FromJID: v.Info.Sender.ToNonAD().String(),
		FromMe:  v.Info.IsFromMe,
		TS:      v.Info.Timestamp.Unix(),
		Edited:  edited,
	}
	extractText(content, &msg)
	if !hasContent(&msg) {
		// Protocol bookkeeping (key shares, history-sync notifications, peer
		// data requests, ...) arrives as a message too; there is nothing to
		// show, so it never becomes a bubble.
		return nil
	}
	return &Event{Kind: EventMessage, Message: &msg}
}

// translateCallMeta maps the part every call signal shares: the chat it
// belongs to (the group for a group call, else the peer), the caller, the
// call id and the signal's timestamp.
func translateCallMeta(m types.BasicCallMeta) *Call {
	chat := m.From
	if !m.GroupJID.IsEmpty() {
		chat = m.GroupJID
	}
	caller := m.CallCreator
	if caller.IsEmpty() {
		caller = m.From
	}
	var ts int64
	if !m.Timestamp.IsZero() {
		ts = m.Timestamp.Unix()
	}
	return &Call{
		ChatJID:   chat.String(),
		CallID:    m.CallID,
		CallerJID: caller.ToNonAD().String(),
		TS:        ts,
	}
}

// callOfferIsVideo tells a video call from a voice call by the offer's
// media description: a voice offer lists only <audio> codecs, a video one
// adds a <video> element.
func callOfferIsVideo(offer *waBinary.Node) bool {
	if offer == nil {
		return false
	}
	for _, child := range offer.GetChildren() {
		if child.Tag == "video" {
			return true
		}
	}
	return false
}

// callTerminateOutcome reads a terminate signal's reason: a call nobody
// picked up ends with "timeout" and is a missed call; any other reason (the
// caller hung up after it connected, a busy line, ...) settles nothing on
// its own.
func callTerminateOutcome(reason string) string {
	if reason == "timeout" {
		return CallMissed
	}
	return ""
}

// translateCallLog maps the phone's own record of a call (a CallLogMessage,
// which WhatsApp sends to its linked devices once a call ends) to a CallLog.
func translateCallLog(cl *waE2E.CallLogMessage) *CallLog {
	out := &CallLog{Video: cl.GetIsVideo(), DurationSecs: int(cl.GetDurationSecs())}
	switch cl.GetCallOutcome() {
	case waE2E.CallLogMessage_MISSED, waE2E.CallLogMessage_SILENCED_BY_DND, waE2E.CallLogMessage_SILENCED_UNKNOWN_CALLER:
		out.Outcome = CallMissed
	case waE2E.CallLogMessage_REJECTED:
		out.Outcome = CallDeclined
	case waE2E.CallLogMessage_FAILED:
		out.Outcome = CallFailed
	default:
		// CONNECTED, ACCEPTED_ELSEWHERE, ONGOING: the call was picked up.
		out.Outcome = CallAnswered
	}
	return out
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

// extractText fills in Text, ReplyTo, Attachment and the rich-kind fields
// (Location, Contact, Poll, Event) from the leaf proto message, covering
// plain/quoted text, the common media kinds, (live) locations, contacts,
// poll creation and scheduled events.
func extractText(m *waProto.Message, msg *Message) {
	m = unwrapContent(m)
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
		msg.Attachment = &Attachment{Kind: "image", MimeType: img.GetMimetype(), Caption: img.GetCaption(), ProtoBlob: marshalMedia(img), Thumbnail: img.GetJPEGThumbnail(), ViewOnce: img.GetViewOnce(), Size: int64(img.GetFileLength())}
		ctx = img.GetContextInfo()
	case m.GetVideoMessage() != nil || m.GetPtvMessage() != nil:
		// A PTV ("video note", the round one) is a VideoMessage in another
		// field; it downloads and plays exactly like a video.
		vid := m.GetVideoMessage()
		if vid == nil {
			vid = m.GetPtvMessage()
		}
		msg.Attachment = &Attachment{Kind: "video", MimeType: vid.GetMimetype(), Caption: vid.GetCaption(), ProtoBlob: marshalMedia(vid), Thumbnail: vid.GetJPEGThumbnail(), IsGIF: vid.GetGifPlayback(), ViewOnce: vid.GetViewOnce(), Size: int64(vid.GetFileLength()), DurationSecs: int(vid.GetSeconds())}
		ctx = vid.GetContextInfo()
	case m.GetAudioMessage() != nil:
		aud := m.GetAudioMessage()
		msg.Attachment = &Attachment{Kind: "audio", MimeType: aud.GetMimetype(), ProtoBlob: marshalMedia(aud), Size: int64(aud.GetFileLength()), DurationSecs: int(aud.GetSeconds())}
		ctx = aud.GetContextInfo()
	case m.GetDocumentMessage() != nil:
		doc := m.GetDocumentMessage()
		msg.Attachment = &Attachment{
			Kind: "document", MimeType: doc.GetMimetype(),
			Filename: doc.GetFileName(), Caption: doc.GetCaption(),
			ProtoBlob: marshalMedia(doc), Thumbnail: doc.GetJPEGThumbnail(),
			Size: int64(doc.GetFileLength()),
		}
		ctx = doc.GetContextInfo()
	case m.GetStickerMessage() != nil:
		sticker := m.GetStickerMessage()
		msg.Attachment = &Attachment{Kind: "sticker", MimeType: sticker.GetMimetype(), ProtoBlob: marshalMedia(sticker), Thumbnail: sticker.GetPngThumbnail(), Size: int64(sticker.GetFileLength())}
		ctx = sticker.GetContextInfo()
	case m.GetLocationMessage() != nil:
		loc := m.GetLocationMessage()
		msg.Location = &Location{
			Name: loc.GetName(), Address: loc.GetAddress(),
			Latitude: loc.GetDegreesLatitude(), Longitude: loc.GetDegreesLongitude(),
			Thumbnail: loc.GetJPEGThumbnail(),
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
	case m.GetContactMessage() != nil:
		c := m.GetContactMessage()
		msg.Contact = &Contact{
			DisplayName: c.GetDisplayName(),
			Phones:      parseVCardPhones(c.GetVcard()),
		}
		ctx = c.GetContextInfo()
	case m.GetContactsArrayMessage() != nil:
		// Several contacts shared at once: only the first person is modeled,
		// carrying their name and phone numbers.
		arr := m.GetContactsArrayMessage()
		if contacts := arr.GetContacts(); len(contacts) > 0 {
			first := contacts[0]
			name := first.GetDisplayName()
			if name == "" {
				name = arr.GetDisplayName()
			}
			msg.Contact = &Contact{
				DisplayName: name,
				Phones:      parseVCardPhones(first.GetVcard()),
			}
		}
		ctx = arr.GetContextInfo()
	case m.GetEventMessage() != nil:
		ev := m.GetEventMessage()
		msg.EventInvite = &EventInvite{
			Name: ev.GetName(), Description: ev.GetDescription(),
			Location: eventLocationText(ev.GetLocation()),
			StartTS:  ev.GetStartTime(), EndTS: ev.GetEndTime(),
			Canceled: ev.GetIsCanceled(),
		}
		ctx = ev.GetContextInfo()
	case m.GetCallLogMesssage() != nil:
		msg.CallLog = translateCallLog(m.GetCallLogMesssage())
	case pollCreation(m) != nil:
		poll := pollCreation(m)
		opts := poll.GetOptions()
		options := make([]PollOption, len(opts))
		for i, o := range opts {
			options[i] = PollOption{Name: o.GetOptionName()}
		}
		msg.Poll = &Poll{
			Name:            poll.GetName(),
			Options:         options,
			SelectableCount: int(poll.GetSelectableOptionsCount()),
		}
		ctx = poll.GetContextInfo()
	default:
		var rich bool
		ctx, rich = extractRichText(m, msg)
		if !rich && hasPayload(m) {
			msg.Text = unsupportedText
		}
	}
	if ctx == nil {
		return
	}
	if id := ctx.GetStanzaID(); id != "" {
		msg.ReplyTo = &MsgRef{ChatJID: msg.ChatJID, MsgID: id}
	}
	msg.Forwarded = ctx.GetIsForwarded()
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

// parseVCardPhones pulls phone numbers out of a vCard's TEL lines, e.g.
// "TEL;type=CELL;waid=1234567890:+1 234-567-890". WhatsApp's waid
// parameter is the account's number in canonical digits and wins over the
// display text after the last ':' (which may carry non-breaking spaces or
// typographic dashes); lines with neither are skipped.
func parseVCardPhones(vcard string) []string {
	var phones []string
	for _, line := range strings.Split(strings.ReplaceAll(vcard, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "TEL") {
			continue
		}
		idx := strings.LastIndex(line, ":")
		if idx < 0 {
			continue
		}
		if waid := vcardWAID(line[:idx]); waid != "" {
			phones = append(phones, "+"+waid)
			continue
		}
		if phone := strings.TrimSpace(line[idx+1:]); phone != "" {
			phones = append(phones, phone)
		}
	}
	return phones
}

// vcardWAID is the digits of a TEL line's waid= parameter, "" when absent.
func vcardWAID(params string) string {
	for _, p := range strings.Split(params, ";") {
		k, v, ok := strings.Cut(p, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "waid") {
			continue
		}
		v = strings.TrimSpace(v)
		for _, r := range v {
			if r < '0' || r > '9' {
				return ""
			}
		}
		return v
	}
	return ""
}

// eventLocationText renders an event's attached location as a single display
// string: the name, or "lat, long" when unnamed, or "" when absent entirely.
func eventLocationText(loc *waProto.LocationMessage) string {
	if loc == nil {
		return ""
	}
	if name := loc.GetName(); name != "" {
		return name
	}
	if loc.GetDegreesLatitude() == 0 && loc.GetDegreesLongitude() == 0 {
		return ""
	}
	return fmt.Sprintf("%v, %v", loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
}

// buildVCard renders contact as a minimal vCard 3.0 (FN plus one TEL line per
// phone) — the same shape parseVCardPhones reads back on the receiving end.
func buildVCard(contact Contact) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\n")
	b.WriteString("VERSION:3.0\r\n")
	fmt.Fprintf(&b, "FN:%s\r\n", contact.DisplayName)
	for _, phone := range contact.Phones {
		fmt.Fprintf(&b, "TEL;type=CELL:%s\r\n", phone)
	}
	b.WriteString("END:VCARD")
	return b.String()
}
