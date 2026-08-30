package client

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// newsletterFetchCount is the default number of posts NewsletterMessages
// requests when the caller passes count <= 0.
const newsletterFetchCount = 20

// Newsletters lists the channels this account is subscribed to.
func (w *Whatsmeow) Newsletters(ctx context.Context) ([]Newsletter, error) {
	metas, err := w.wa.GetSubscribedNewsletters(ctx)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: get subscribed newsletters: %w", err)
	}
	out := make([]Newsletter, 0, len(metas))
	for _, m := range metas {
		if m == nil {
			continue
		}
		out = append(out, newsletterFromMeta(m))
	}
	return out, nil
}

// NewsletterMessages fetches up to count recent posts in channel jid.
func (w *Whatsmeow) NewsletterMessages(ctx context.Context, jid string, count int) ([]NewsletterMessage, error) {
	j, err := types.ParseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: parse newsletter jid: %w", err)
	}
	if count <= 0 {
		count = newsletterFetchCount
	}
	msgs, err := w.wa.GetNewsletterMessages(ctx, j, &whatsmeow.GetNewsletterMessagesParams{Count: count})
	if err != nil {
		return nil, fmt.Errorf("chatot/client: get newsletter messages: %w", err)
	}
	out := make([]NewsletterMessage, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		out = append(out, newsletterMessageFrom(m))
	}
	return out, nil
}

// FollowNewsletter subscribes to channel jid.
func (w *Whatsmeow) FollowNewsletter(ctx context.Context, jid string) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse newsletter jid: %w", err)
	}
	if err := w.wa.FollowNewsletter(ctx, j); err != nil {
		return fmt.Errorf("chatot/client: follow newsletter: %w", err)
	}
	return nil
}

// UnfollowNewsletter unsubscribes from channel jid.
func (w *Whatsmeow) UnfollowNewsletter(ctx context.Context, jid string) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse newsletter jid: %w", err)
	}
	if err := w.wa.UnfollowNewsletter(ctx, j); err != nil {
		return fmt.Errorf("chatot/client: unfollow newsletter: %w", err)
	}
	return nil
}

// NewsletterSetMuted mutes/unmutes channel jid.
func (w *Whatsmeow) NewsletterSetMuted(ctx context.Context, jid string, mute bool) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse newsletter jid: %w", err)
	}
	if err := w.wa.NewsletterToggleMute(ctx, j, mute); err != nil {
		return fmt.Errorf("chatot/client: toggle newsletter mute: %w", err)
	}
	return nil
}

// NewsletterReact reacts to a channel post.
func (w *Whatsmeow) NewsletterReact(ctx context.Context, jid, messageID string, serverID int64, emoji string) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse newsletter jid: %w", err)
	}
	if err := w.wa.NewsletterSendReaction(ctx, j, types.MessageServerID(serverID), emoji, types.MessageID(messageID)); err != nil {
		return fmt.Errorf("chatot/client: newsletter react: %w", err)
	}
	return nil
}

// FollowNewsletterByLink resolves link to a channel and follows it.
//
// Live-unverifiable risk: the exact whatsapp.com/channel/<key> URL scheme and
// what GetNewsletterInfoWithInvite expects as its key aren't confirmed against
// a live account here; parseChannelInvite best-effort strips the known prefix
// to the bare key.
func (w *Whatsmeow) FollowNewsletterByLink(ctx context.Context, link string) (string, error) {
	key := parseChannelInvite(link)
	if key == "" {
		return "", fmt.Errorf("chatot/client: empty channel invite key")
	}
	meta, err := w.wa.GetNewsletterInfoWithInvite(ctx, key)
	if err != nil {
		return "", fmt.Errorf("chatot/client: resolve channel invite: %w", err)
	}
	if err := w.wa.FollowNewsletter(ctx, meta.ID); err != nil {
		return "", fmt.Errorf("chatot/client: follow newsletter: %w", err)
	}
	return meta.ID.String(), nil
}

// newsletterFromMeta maps whatsmeow's NewsletterMetadata to our Newsletter,
// pulling the display name/description out of their wrapped-text fields and
// muted state out of the (optional) viewer metadata.
func newsletterFromMeta(m *types.NewsletterMetadata) Newsletter {
	muted := false
	if m.ViewerMeta != nil {
		muted = m.ViewerMeta.Mute == types.NewsletterMuteOn
	}
	return Newsletter{
		ID:          m.ID.String(),
		Name:        m.ThreadMeta.Name.Text,
		Description: m.ThreadMeta.Description.Text,
		Muted:       muted,
	}
}

// newsletterMessageFrom maps a whatsmeow NewsletterMessage, extracting the
// post text from its carried waE2E.Message via the same getters extractText
// uses and converting the int MessageServerID to int64.
func newsletterMessageFrom(m *types.NewsletterMessage) NewsletterMessage {
	return NewsletterMessage{
		ID:        string(m.MessageID),
		ServerID:  int64(m.MessageServerID),
		Text:      newsletterMessageText(m.Message),
		TS:        m.Timestamp.Unix(),
		Views:     m.ViewsCount,
		Reactions: m.ReactionCounts,
	}
}

// newsletterMessageText extracts the display text of a channel post, falling
// back to a placeholder for non-text bodies.
func newsletterMessageText(m *waE2E.Message) string {
	if m == nil {
		return ""
	}
	var msg Message
	extractText(m, &msg)
	switch {
	case msg.Text != "":
		return msg.Text
	case msg.Attachment != nil:
		return mediaChipKind(msg.Attachment.Kind)
	case msg.Location != nil:
		return "📍 Location"
	case msg.Contact != nil:
		return "👤 Contact"
	case msg.Poll != nil:
		return "📊 " + msg.Poll.Name
	default:
		return ""
	}
}

// mediaChipKind renders a bare media-kind placeholder for a channel post.
func mediaChipKind(kind string) string {
	switch kind {
	case "image":
		return "📷 Photo"
	case "video":
		return "🎥 Video"
	case "audio":
		return "🎤 Audio"
	case "document":
		return "📎 Document"
	case "sticker":
		return "🌟 Sticker"
	default:
		return "📎 " + kind
	}
}

// parseChannelInvite reduces a WhatsApp channel link to its bare invite key,
// accepting a full https/http URL, an optional "www.", a scheme-less
// "whatsapp.com/channel/<key>", or an already-bare key, trimming surrounding
// whitespace and slashes.
func parseChannelInvite(input string) string {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	s = strings.TrimPrefix(s, "whatsapp.com/channel/")
	return strings.Trim(s, "/ ")
}
