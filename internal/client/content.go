package client

import (
	"fmt"
	"strings"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// unsupportedText is the bubble shown for a message whose payload WhatsApp
// sent but chatot cannot render (payments, interactive templates it does
// not model, ...). Better a labelled bubble than an empty one.
const unsupportedText = "⚠ Unsupported message"

// placeholderText is what WhatsApp shows while a message's content is still
// withheld from a linked device.
const placeholderText = "Waiting for this message. This may take a while."

// unwrapContent peels the transport wrappers WhatsApp nests a real message
// inside (a message sent from another own device, a disappearing or
// view-once envelope, a document-with-caption, an edit, a bot reply, a
// comment) until it reaches the leaf that carries the content. whatsmeow
// unwraps most of these on the live path but the history-sync path hands
// over the raw WebMessageInfo, so this runs on both.
func unwrapContent(m *waProto.Message) *waProto.Message {
	for i := 0; m != nil && i < 8; i++ {
		var inner *waProto.Message
		switch {
		case m.GetDeviceSentMessage().GetMessage() != nil:
			inner = m.GetDeviceSentMessage().GetMessage()
		case m.GetBotInvokeMessage().GetMessage() != nil:
			inner = m.GetBotInvokeMessage().GetMessage()
		case m.GetEphemeralMessage().GetMessage() != nil:
			inner = m.GetEphemeralMessage().GetMessage()
		case m.GetViewOnceMessage().GetMessage() != nil:
			inner = m.GetViewOnceMessage().GetMessage()
		case m.GetViewOnceMessageV2().GetMessage() != nil:
			inner = m.GetViewOnceMessageV2().GetMessage()
		case m.GetViewOnceMessageV2Extension().GetMessage() != nil:
			inner = m.GetViewOnceMessageV2Extension().GetMessage()
		case m.GetLottieStickerMessage().GetMessage() != nil:
			inner = m.GetLottieStickerMessage().GetMessage()
		case m.GetDocumentWithCaptionMessage().GetMessage() != nil:
			inner = m.GetDocumentWithCaptionMessage().GetMessage()
		case m.GetEditedMessage().GetMessage() != nil:
			inner = m.GetEditedMessage().GetMessage()
		case m.GetCommentMessage().GetMessage() != nil:
			inner = m.GetCommentMessage().GetMessage()
		case m.GetGroupMentionedMessage().GetMessage() != nil:
			inner = m.GetGroupMentionedMessage().GetMessage()
		case m.GetPollCreationMessageV4().GetMessage() != nil:
			inner = m.GetPollCreationMessageV4().GetMessage()
		case m.GetPollCreationOptionImageMessage().GetMessage() != nil:
			inner = m.GetPollCreationOptionImageMessage().GetMessage()
		default:
			return m
		}
		m = inner
	}
	return m
}

// pollCreation returns whichever poll-creation variant m carries (WhatsApp
// has bumped the field several times; the payload shape is the same).
func pollCreation(m *waProto.Message) *waProto.PollCreationMessage {
	for _, p := range []*waProto.PollCreationMessage{
		m.GetPollCreationMessage(), m.GetPollCreationMessageV2(), m.GetPollCreationMessageV3(),
		m.GetPollCreationMessageV5(), m.GetPollCreationMessageV6(),
	} {
		if p != nil {
			return p
		}
	}
	return nil
}

// extractRichText covers the message kinds that render as plain text in
// chatot even though WhatsApp models them as structured payloads: group
// invites, business templates/buttons/lists and their replies, products,
// orders, invoices, scheduled calls, sticker packs and withheld
// placeholders. It reports whether m was one of those.
func extractRichText(m *waProto.Message, msg *Message) (ctx *waProto.ContextInfo, ok bool) {
	switch {
	case m.GetGroupInviteMessage() != nil:
		gi := m.GetGroupInviteMessage()
		msg.Text = joinNonEmpty("\n", "👥 Group invite: "+gi.GetGroupName(), gi.GetCaption())
		return gi.GetContextInfo(), true
	case m.GetTemplateMessage() != nil:
		t := m.GetTemplateMessage()
		body := t.GetHydratedTemplate().GetHydratedContentText()
		if body == "" {
			body = t.GetHydratedFourRowTemplate().GetHydratedContentText()
		}
		msg.Text = orUnsupported(body)
		return t.GetContextInfo(), true
	case m.GetButtonsMessage() != nil:
		b := m.GetButtonsMessage()
		msg.Text = orUnsupported(joinNonEmpty("\n", b.GetText(), b.GetContentText()))
		return b.GetContextInfo(), true
	case m.GetListMessage() != nil:
		l := m.GetListMessage()
		msg.Text = orUnsupported(joinNonEmpty("\n", l.GetTitle(), l.GetDescription()))
		return l.GetContextInfo(), true
	case m.GetInteractiveMessage() != nil:
		im := m.GetInteractiveMessage()
		msg.Text = orUnsupported(joinNonEmpty("\n", im.GetHeader().GetTitle(), im.GetBody().GetText()))
		return im.GetContextInfo(), true
	case m.GetButtonsResponseMessage() != nil:
		msg.Text = orUnsupported(m.GetButtonsResponseMessage().GetSelectedDisplayText())
		return m.GetButtonsResponseMessage().GetContextInfo(), true
	case m.GetListResponseMessage() != nil:
		msg.Text = orUnsupported(m.GetListResponseMessage().GetTitle())
		return m.GetListResponseMessage().GetContextInfo(), true
	case m.GetTemplateButtonReplyMessage() != nil:
		msg.Text = orUnsupported(m.GetTemplateButtonReplyMessage().GetSelectedDisplayText())
		return m.GetTemplateButtonReplyMessage().GetContextInfo(), true
	case m.GetInteractiveResponseMessage() != nil:
		msg.Text = orUnsupported(m.GetInteractiveResponseMessage().GetBody().GetText())
		return m.GetInteractiveResponseMessage().GetContextInfo(), true
	case m.GetProductMessage() != nil:
		p := m.GetProductMessage()
		msg.Text = "🛍 " + orUnsupported(p.GetProduct().GetTitle())
		return p.GetContextInfo(), true
	case m.GetOrderMessage() != nil:
		o := m.GetOrderMessage()
		title := o.GetOrderTitle()
		if title == "" {
			title = fmt.Sprintf("%d item(s)", o.GetItemCount())
		}
		msg.Text = "🧾 Order: " + title
		return o.GetContextInfo(), true
	case m.GetInvoiceMessage() != nil:
		msg.Text = joinNonEmpty("\n", "🧾 Invoice", m.GetInvoiceMessage().GetNote())
		return nil, true
	case m.GetScheduledCallCreationMessage() != nil:
		msg.Text = "📞 Scheduled call: " + orUnsupported(m.GetScheduledCallCreationMessage().GetTitle())
		return nil, true
	case m.GetStickerPackMessage() != nil:
		msg.Text = "🎨 Sticker pack: " + orUnsupported(m.GetStickerPackMessage().GetName())
		return m.GetStickerPackMessage().GetContextInfo(), true
	case m.GetPlaceholderMessage() != nil:
		msg.Text = placeholderText
		return nil, true
	case m.GetSendPaymentMessage() != nil, m.GetRequestPaymentMessage() != nil:
		msg.Text = "💸 Payment"
		return nil, true
	}
	return nil, false
}

// noiseFields are the payload fields that are protocol bookkeeping, not a
// message a person would read: key shares, reactions (handled separately),
// poll votes, pins, album headers, and so on. A message carrying only these
// is dropped rather than shown as an empty bubble.
var noiseFields = map[string]bool{
	"messageContextInfo":                         true,
	"senderKeyDistributionMessage":               true,
	"fastRatchetKeySenderKeyDistributionMessage": true,
	"protocolMessage":                            true,
	"reactionMessage":                            true,
	"encReactionMessage":                         true,
	"pollUpdateMessage":                          true,
	"keepInChatMessage":                          true,
	"pinInChatMessage":                           true,
	"albumMessage":                               true,
	"stickerSyncRmrMessage":                      true,
	"encCommentMessage":                          true,
	"encEventResponseMessage":                    true,
	"requestPhoneNumberMessage":                  true,
	"statusMentionMessage":                       true,
	"groupStatusMentionMessage":                  true,
	"secretEncryptedMessage":                     true,
	"rootSecretDistributeMessage":                true,
	"pollResultSnapshotMessage":                  true,
	"groupStatusMessage":                         true,
	"statusNotificationMessage":                  true,
	"limitSharingMessage":                        true,
	"associatedChildMessage":                     true,
	"bcallMessage":                               true,
	"declinePaymentRequestMessage":               true,
	"cancelPaymentRequestMessage":                true,
	"paymentInviteMessage":                       true,
	"scheduledCallEditMessage":                   true,
	"newsletterAdminInviteMessage":               false,
}

// hasPayload reports whether m carries anything beyond protocol noise, i.e.
// something a person would expect to see a bubble for. Used after the
// typed extraction failed to decide between "drop silently" and "show an
// Unsupported placeholder".
func hasPayload(m *waProto.Message) bool {
	if m == nil {
		return false
	}
	found := false
	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if noiseFields[string(fd.Name())] {
			return true
		}
		found = true
		return false
	})
	return found
}

// hasContent reports whether msg carries something renderable: text, an
// attachment or one of the rich kinds. Messages without it (a bare key
// share, a poll vote, a history-sync notification) never reach the store.
func hasContent(msg *Message) bool {
	return msg.Text != "" || msg.Attachment != nil || msg.Location != nil ||
		msg.Contact != nil || msg.Poll != nil || msg.EventInvite != nil
}

func orUnsupported(s string) string {
	if strings.TrimSpace(s) == "" {
		return unsupportedText
	}
	return s
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}
