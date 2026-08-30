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
	EventPollVote
	EventRevoke
	EventAvatar
	// EventChatUpdate signals a chat's organization state (pin/mute/archive/
	// unread) changed, possibly from another device via app-state; the chat
	// list should refresh.
	EventChatUpdate
	// EventLabelUpdate signals the set of labels or a chat's label
	// associations changed (possibly from another device); the label filter
	// and chat list should refresh.
	EventLabelUpdate
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
	PollVote     *PollVote
	Revoke       *Revoke
	Avatar       *Avatar
	ChatUpdate   *ChatUpdate
}

// Receipt is a delivery/read acknowledgement for previously sent messages.
type Receipt struct {
	ChatJID string
	MsgIDs  []string
	Read    bool // true for a read-type receipt; still drives MarkChatRead
	// Status is the message status this receipt advances MsgIDs to: 1 for
	// delivered, 2 for read (see MessageStatus* constants). 0 means "don't
	// touch status" (kept only for older callers that never set it).
	Status int
}

// Outgoing message delivery/read states, mirrored 1:1 in the store's
// messages.status column.
const (
	MessageStatusSent      = 0
	MessageStatusDelivered = 1
	MessageStatusRead      = 2
)

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
	Muted         bool
	Archived      bool
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
	// Location is non-nil for a (live) location message. It's the first rich
	// non-text/media body carried through the store's kind/payload seam.
	Location *Location
	// Contact is non-nil for a shared vCard (or vCard array) message.
	Contact *Contact
	// Poll is non-nil for a poll-creation message; its options carry live vote
	// counts tallied from decrypted poll-update (vote) events.
	Poll *Poll
	// Edited is true once the sender edited this message's text (WhatsApp
	// allows editing a text message for ~15 min); drives the "edited" marker.
	Edited bool
	// Deleted is true once this message was revoked ("delete for everyone");
	// it renders as a tombstone regardless of its original content.
	Deleted bool
	// Status is the delivery/read tick state for a FromMe message: see the
	// MessageStatus* constants. Meaningless (always 0) for inbound messages.
	Status int
	// Starred is true once the message has been starred via app-state.
	Starred bool
}

// Poll is a poll-creation message with its immutable definition (Name,
// option names, SelectableCount) plus the per-option tally computed from
// stored votes. SelectableCount is how many options a voter may pick (1 for
// single-choice; >1 or 0 means multi-select).
type Poll struct {
	Name            string
	Options         []PollOption
	SelectableCount int
}

// PollOption is one poll choice. Count is how many distinct voters selected
// it; Voted is whether the local user selected it (drives the UI highlight).
type PollOption struct {
	Name  string
	Count int
	Voted bool
}

// PollVote signals that a poll's tally changed (a vote arrived or we cast
// one); the UI reloads the chat to refresh counts. It carries no per-vote
// detail — the recomputed tally is read back from the store.
type PollVote struct {
	ChatJID   string
	PollMsgID string
}

// Location is a shared or live-shared geographic point. Name/Address are
// often empty (a live location carries neither); IsLive marks a continuously
// updated share rather than a one-off pin.
type Location struct {
	Name      string
	Address   string
	Latitude  float64
	Longitude float64
	IsLive    bool
}

// Contact is a shared vCard. For a ContactsArrayMessage (several people
// shared at once) DisplayName is the first contact's name and Phones is
// just that first contact's numbers — multi-contact shares aren't modeled
// beyond that.
type Contact struct {
	DisplayName string
	Phones      []string
}

// Reaction is a reaction added to, or (Emoji == "") cleared from, a message.
type Reaction struct {
	ChatJID    string
	MsgID      string // the message being reacted to
	ReactorJID string
	Emoji      string // "" clears the reaction
	TS         int64
}

// Revoke is a "delete for everyone" applied to a message.
type Revoke struct {
	ChatJID string
	MsgID   string
	TS      int64
}

// Avatar signals that jid's profile picture changed (or was removed); any
// path previously returned by Client.Avatar for it should be re-resolved.
type Avatar struct {
	JID string
}

// ChatUpdate signals that JID's pin/mute/archive/unread state changed.
type ChatUpdate struct {
	JID string
}

// Label is a WhatsApp Business label used to organize chats.
type Label struct {
	ID    string
	Name  string
	Color int
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

// GroupParticipant is one member of a group, as returned by GroupInfo.
type GroupParticipant struct {
	JID          string
	IsAdmin      bool
	IsSuperAdmin bool
}

// GroupInfo is a group's metadata and membership, read-only (mutating a
// group is out of scope here).
type GroupInfo struct {
	JID          string
	Name         string
	Topic        string
	OwnerJID     string
	Participants []GroupParticipant
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
	// MessagesBefore returns up to limit messages older than beforeMsgID
	// (oldest first), for the conversation view's scroll-up paging.
	MessagesBefore(jid, beforeMsgID string, limit int) ([]Message, error)
	Search(query string, limit int) ([]SearchHit, error)

	// writes
	SendText(ctx context.Context, jid, text string, replyTo *MsgRef) (string, error)
	SendMedia(ctx context.Context, jid string, m Attachment, replyTo *MsgRef) (string, error)
	SendLocation(ctx context.Context, jid string, loc Location, replyTo *MsgRef) (string, error)
	SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error)
	// CreatePoll sends a poll with the given question and options; selectable
	// is how many options a voter may pick (1 = single-choice).
	CreatePoll(ctx context.Context, jid, name string, options []string, selectable int) (string, error)
	// VotePoll casts (or replaces) the local user's vote on pollMsgID with the
	// named options.
	VotePoll(ctx context.Context, chatJID, pollMsgID string, options []string) error
	// EditMessage replaces an own text message's content (WhatsApp's ~15-min
	// edit window); reflected optimistically in the store + open chat.
	EditMessage(ctx context.Context, chatJID, msgID, newText string) error
	// DeleteMessage revokes ("delete for everyone") an own message; reflected
	// optimistically in the store + open chat.
	DeleteMessage(ctx context.Context, chatJID, msgID string) error
	React(ctx context.Context, jid, msgID, emoji string) error // "" clears
	MarkRead(ctx context.Context, jid string, msgIDs []string) error
	// CheckOnWhatsApp looks up an E.164 phone number and reports its canonical
	// JID and whether it's registered on WhatsApp. onWhatsApp is false (with a
	// nil error) for a well-formed but unregistered number; err is reserved
	// for transport failures.
	CheckOnWhatsApp(ctx context.Context, phone string) (jid string, onWhatsApp bool, err error)
	SendPresence(available bool) error
	SendTyping(jid string, typing bool) error
	// PinChat, MuteChat, ArchiveChat and MarkChatUnread write chat-organization
	// app-state; reflected optimistically in the store and via an
	// EventChatUpdate.
	PinChat(ctx context.Context, jid string, pin bool) error
	MuteChat(ctx context.Context, jid string, mute bool) error
	ArchiveChat(ctx context.Context, jid string, archive bool) error
	MarkChatUnread(ctx context.Context, jid string, unread bool) error
	// StarMessage stars/unstars msgID via app-state; reflected optimistically
	// in the store and via a refresh event for the open thread.
	StarMessage(ctx context.Context, chatJID, msgID string, starred bool) error
	// StarredMessages returns starred messages across every chat, newest
	// first, for the starred-messages sidebar view.
	StarredMessages(limit int) ([]Message, error)

	// Blocklist returns the JIDs currently blocked.
	Blocklist(ctx context.Context) ([]string, error)
	// SetBlocked blocks or unblocks jid; the result is reflected in the
	// cached set IsBlocked reads and pushed as an EventChatUpdate.
	SetBlocked(ctx context.Context, jid string, blocked bool) error
	// IsBlocked is a cheap synchronous check against the locally cached
	// blocked set (no network call).
	IsBlocked(jid string) bool
	// PrivacySettings returns the account's privacy settings as a
	// display-friendly name -> value map. Read-only.
	PrivacySettings(ctx context.Context) (map[string]string, error)

	// Labels returns the non-deleted WhatsApp Business labels.
	Labels() ([]Label, error)
	// CreateLabel creates a new label with the given name/color, allocating
	// the next unused numeric id; returns the new id.
	CreateLabel(ctx context.Context, name string, color int) (id string, err error)
	// EditLabel renames/recolors an existing label.
	EditLabel(ctx context.Context, id, name string, color int) error
	// DeleteLabel marks a label deleted (removing it from Labels).
	DeleteLabel(ctx context.Context, id string) error
	// SetChatLabeled associates/disassociates chatJID with labelID via
	// app-state; reflected optimistically and via an EventLabelUpdate.
	SetChatLabeled(ctx context.Context, labelID, chatJID string, labeled bool) error
	// LabelsForChat returns the ids of labels currently on chatJID.
	LabelsForChat(chatJID string) ([]string, error)

	// GroupInfo fetches a group's current name/topic/membership. Read-only.
	GroupInfo(ctx context.Context, jid string) (*GroupInfo, error)

	// media
	DownloadMedia(ctx context.Context, msgID string) (localPath string, err error)
	// Avatar resolves jid's profile picture to a local cached file path,
	// fetching it on first use. Returns ("", nil) if there's no picture (or
	// it's not visible to us) — that's normal, not an error.
	Avatar(ctx context.Context, jid string) (localPath string, err error)
}
