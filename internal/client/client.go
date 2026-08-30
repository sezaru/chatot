// Package client defines the seam between the UI and the underlying
// WhatsApp connection, so the UI can be built and tested against Fake
// before the whatsmeow-backed implementation exists (F2).
package client

import "context"

// EventKind identifies the payload carried by an Event.
type EventKind int

const (
	EventMessage EventKind = iota
	EventReceipt
	EventPresence
	EventChatPresence
	EventCall
	EventConnection
	EventHistorySync
	EventQR
	EventPairSuccess
	EventLoggedOut
	EventReaction
)

// Event is a normalized notification pushed on Client.Events(). Only the
// field matching Kind is populated; the rest are zero.
type Event struct {
	Kind         EventKind
	Message      *Message
	Receipt      *Receipt
	Presence     *Presence
	ChatPresence *ChatPresence
	Call         *Call
	Connection   *Connection
	HistorySync  *HistorySync
	Reaction     *Reaction
}

// Receipt is a delivery/read acknowledgement for previously sent messages.
type Receipt struct {
	ChatJID string
	MsgIDs  []string
	Read    bool
}

// Presence is a contact's overall online/offline state.
type Presence struct {
	JID      string
	Online   bool
	LastSeen int64 // unix seconds, 0 if unknown
}

// ChatPresence is a per-chat composing/paused/recording indicator.
type ChatPresence struct {
	ChatJID string
	JID     string
	State   string // "composing", "paused", "recording"
}

// Call is an incoming/ongoing call notification.
type Call struct {
	ChatJID string
	CallID  string
	Video   bool
	Offer   bool // true = incoming offer, false = ended/rejected
}

// Connection reports transport-level state changes.
type Connection struct {
	Connected bool
	Err       error
}

// HistorySync signals that a batch of backfilled chats/messages has been
// ingested and reads should refresh.
type HistorySync struct {
	ChatJIDs []string
}

// Chat is a single conversation (1:1 or group) as shown in the chat list.
type Chat struct {
	JID           string
	Name          string
	Preview       string
	UnreadCount   int
	LastMessageTS int64
	Pinned        bool
	IsGroup       bool
}

// Message is a single chat message as shown in the conversation view.
type Message struct {
	ID         string
	ChatJID    string
	FromJID    string // sender within a group; equals ChatJID for 1:1
	FromMe     bool
	Text       string
	TS         int64
	ReplyTo    *MsgRef
	Reactions  map[string]string // emoji -> reactor JID (last wins)
	Attachment *Attachment
}

// Reaction is a reaction added to, or (Emoji == "") cleared from, a message.
type Reaction struct {
	ChatJID    string
	MsgID      string // the message being reacted to
	ReactorJID string
	Emoji      string // "" clears the reaction
	TS         int64
}

// MsgRef points at a message being replied to or reacted to.
type MsgRef struct {
	ChatJID string
	MsgID   string
}

// Attachment describes media attached to a message, inbound or outbound.
type Attachment struct {
	Kind      string // "image", "video", "audio", "document", "sticker"
	Filename  string
	MimeType  string
	LocalPath string // "" until downloaded
	Data      []byte // set on outbound sends before upload
	Caption   string
	// ProtoBlob is proto.Marshal of the inbound waE2E.*Message that carried
	// this attachment (set by extractText), stored so DownloadMedia can
	// reconstruct a whatsmeow.DownloadableMessage later. Never set outbound.
	ProtoBlob []byte
}

// SearchHit is a single fts5 match over the local store. MsgID is "" for a
// chat-name match (Snippet then holds the chat name, not a message excerpt).
type SearchHit struct {
	ChatJID  string
	MsgID    string
	ChatName string
	Snippet  string
	TS       int64
}

// Client is the seam the UI depends on. One real implementation
// (whatsmeow.go, added in F2) and one in-memory Fake (fake.go) satisfy it.
type Client interface {
	// lifecycle / auth
	Start(ctx context.Context) error
	QRCodes() <-chan string
	LoggedIn() bool
	Logout(ctx context.Context) error
	Events() <-chan Event

	// reads go through the store, but exposed here so the fake can serve them too
	Chats(limit int) ([]Chat, error)
	Messages(jid string, limit int) ([]Message, error)
	Search(query string, limit int) ([]SearchHit, error)

	// writes
	SendText(ctx context.Context, jid, text string, replyTo *MsgRef) (string, error)
	SendMedia(ctx context.Context, jid string, m Attachment, replyTo *MsgRef) (string, error)
	SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error)
	React(ctx context.Context, jid, msgID, emoji string) error // "" clears
	MarkRead(ctx context.Context, jid string, msgIDs []string) error
	SendPresence(available bool) error
	SendTyping(jid string, typing bool) error

	// media
	DownloadMedia(ctx context.Context, msgID string) (localPath string, err error)
}
