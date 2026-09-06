// Package client defines the seam between the UI and the underlying
// WhatsApp connection, so the UI can be built and tested against Fake
// before the whatsmeow-backed implementation exists (F2).
package client

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported is returned by a backend for a call it has no transport
// for (whatsmeow has no channel directory, for one); the UI explains rather
// than fails.
var ErrUnsupported = errors.New("chatot/client: not supported by this backend")

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
	// EventNewsletterUpdate signals a channel changed: a new post arrived or
	// a live update moved its view/reaction counts. Newsletter names it.
	EventNewsletterUpdate
)

// Event is a normalized notification pushed on Client.Events(). Only the
// field matching Kind is populated; the rest are zero.
type Event struct {
	Kind EventKind
	// Synced marks a message that arrived as catch-up (the server replaying
	// what this device missed, or a stale-stamped backlog item) rather than
	// live news; the notifier leaves those alone.
	Synced       bool
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
	Newsletter   *NewsletterUpdate
}

// NewsletterUpdate names the channel an EventNewsletterUpdate is about.
type NewsletterUpdate struct {
	JID string
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
	// ReaderJID is who sent a read receipt for our message ("" when the
	// receipt isn't a read by someone else); TS is when. For a status view
	// ChatJID is status@broadcast and ReaderJID the viewer.
	ReaderJID string
	TS        int64
}

// StatusViewer is one contact who viewed an update of ours, from the read
// receipt they sent.
type StatusViewer struct {
	JID string
	TS  int64
}

// Outgoing message delivery/read states, mirrored 1:1 in the store's
// messages.status column. The negative states are the UI's own: a message
// shown the moment Send is pressed, before (or instead of) the server
// taking it. They never reach the store.
const (
	MessageStatusSent      = 0
	MessageStatusDelivered = 1
	MessageStatusRead      = 2
	// MessageStatusPending is an optimistic row whose send is in flight.
	MessageStatusPending = -1
	// MessageStatusFailed is an optimistic row whose send errored; the
	// bubble offers a retry.
	MessageStatusFailed = -2
)

// Presence is a contact's overall online/offline state.
type Presence struct {
	JID      string
	Online   bool
	LastSeen int64 // unix seconds, 0 if unknown
}

// ChatPresence is a per-chat composing/paused/recording indicator. Media
// distinguishes a voice-note recording ("audio") from plain typing ("text"
// or "") when State is "composing".
type ChatPresence struct {
	ChatJID string
	JID     string
	State   string // "composing", "paused"
	Media   string // "audio" for a recording composing state, "text"/"" otherwise
}

// Call is one step of a call's life: the incoming offer (Offer) and the
// accept/terminate/reject signals that settle it.
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
// ingested and reads should refresh. Type/Progress/Chats/Messages describe
// the chunk for the post-link sync screen: the phone streams the chat list
// first ("bootstrap"), then recent messages ("recent"), then the older
// history in "full" chunks that carry a 0-100 Progress.
type HistorySync struct {
	ChatJIDs []string
	// Type is the chunk kind: "bootstrap", "recent", "full", "status",
	// "pushname", "nonblocking", "ondemand" or "" (a synthetic refresh).
	Type string
	// Progress is the phone's percent-complete for this sync, -1 when the
	// chunk carries none.
	Progress int
	// Chats and Messages count what this chunk delivered.
	Chats    int
	Messages int
}

// Chat is a single conversation (1:1 or group) as shown in the chat list.
type Chat struct {
	JID  string
	Name string
	// Phone is the contact's number without the plus, "" when unknown (a
	// group, or a LID chat whose number hasn't been learned yet).
	Phone         string
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
	ID      string
	ChatJID string
	FromJID string // sender within a group; equals ChatJID for 1:1
	FromMe  bool
	Text    string
	TS      int64
	ReplyTo *MsgRef
	// Reactions maps an emoji to the JIDs that reacted with it, oldest
	// first. WhatsApp allows one reaction per person, so a JID appears under
	// at most one emoji.
	Reactions  map[string][]string
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
	// Forwarded is true once WhatsApp's forwarded flag (ContextInfo.IsForwarded)
	// was set on this message; drives the "↩ Forwarded" marker.
	Forwarded bool
	// EventInvite is non-nil for a scheduled-event message (a calendar-style
	// invite posted in a chat, typically a group).
	EventInvite *EventInvite
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
// updated share rather than a one-off pin. LiveUntil is the unix timestamp
// the share was started for (0 if unknown, e.g. a live location received
// from another WhatsApp client — the wire protocol carries no absolute
// expiry, only a stream of position updates).
type Location struct {
	Name      string
	Address   string
	Latitude  float64
	Longitude float64
	IsLive    bool
	LiveUntil int64
	// Thumbnail is the small JPEG map preview the sender's WhatsApp embeds in
	// a location message, so the bubble can show the map without any tile
	// fetch of its own. Empty when the sender didn't attach one.
	Thumbnail []byte
}

// EventInvite is a scheduled event posted in a chat (WhatsApp's
// calendar-style invite): a name, an optional description, an optional
// attached location, and a start time. EndTS is 0 when the event carries
// none.
type EventInvite struct {
	Name        string
	Description string
	Location    string
	StartTS     int64
	EndTS       int64
	Canceled    bool
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

// Newsletter is a subscribed WhatsApp channel ("newsletter"), a one-to-many
// broadcast feed. Channels are fetched live from whatsmeow, never persisted
// to the local store.
type Newsletter struct {
	ID          string
	Name        string
	Description string
	Muted       bool
	// Verified is WhatsApp's verified-channel mark.
	Verified bool
	// Subscribers is the follower count the server reports (0 if unknown).
	Subscribers int
	// InviteCode is the bare key of the channel's whatsapp.com/channel/<key>
	// link ("" if unknown); see NewsletterLink.
	InviteCode string
	// Created is the channel's creation time (Unix seconds, 0 if unknown).
	Created int64
	// Following is true for a channel this account subscribes to. Always
	// true from Newsletters; DiscoverNewsletters sets it per result.
	Following bool
	// Category is the directory category a discovered channel is filed
	// under ("" for a subscribed channel).
	Category string
}

// NewsletterLink is the shareable link for a channel, or "" without a key.
func NewsletterLink(n Newsletter) string {
	if n.InviteCode == "" {
		return ""
	}
	return "https://whatsapp.com/channel/" + n.InviteCode
}

// Community is a WhatsApp community: a parent group whose members share an
// announcement group and a set of linked sub-groups. Groups lists every
// linked group, joined or not.
type Community struct {
	JID         string
	Name        string
	Description string
	CreatorJID  string
	Created     int64 // Unix seconds, 0 if unknown
	// Muted reflects the announcement group's mute state.
	Muted bool
	// IsAdmin is true when this account administers the community.
	IsAdmin     bool
	MemberCount int
	Members     []GroupParticipant
	Groups      []CommunityGroup
}

// CommunityGroup is one group linked to a community. Preview and
// UnreadCount are only meaningful for a joined group.
type CommunityGroup struct {
	JID          string
	Name         string
	Announcement bool
	Joined       bool
	MemberCount  int
	Preview      string
	UnreadCount  int
}

// NewsletterMessage is a single post in a channel. ServerID (whatsmeow's
// MessageServerID, an int) is carried as int64; it plus ID identify the post
// when reacting.
type NewsletterMessage struct {
	ID        string
	ServerID  int64
	Text      string
	TS        int64
	Views     int
	Reactions map[string]int // emoji -> count
	// MyReaction is the emoji we reacted with ("" for none). The server only
	// reports counts, so the backend remembers what it sent.
	MyReaction string
	// Attachment is the post's media, if any; Text then carries the caption.
	// It downloads through DownloadMedia like a chat attachment.
	Attachment *Attachment
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
	// Thumbnail is the small JPEG/PNG preview WhatsApp embeds directly in
	// the message proto, shown instantly while the full media downloads.
	// Outbound, the composer fills it from the attach tray's preview (poster
	// frame, rendered PDF page, scaled photo) so recipients get one too.
	Thumbnail []byte
	// Width and Height are the pixel size of an outbound picture or video,
	// sent in the message so the other side can size the bubble before the
	// download. Zero when unknown; never set inbound.
	Width, Height int
	// IsGIF marks a "video" attachment WhatsApp flags gifPlayback: it plays
	// looped and muted. Inline looping playback is deferred (F37 ships only
	// the badge); this just tags the kind.
	IsGIF bool
	// ViewOnce marks media WhatsApp flags viewOnce: it may be opened exactly
	// once before it's considered spent. Never set outbound.
	ViewOnce bool
	// Viewed is true once a ViewOnce attachment has been opened locally; a
	// spent attachment renders a tombstone-style placeholder and can't be
	// re-downloaded through the UI.
	Viewed bool
	// Size is the byte length WhatsApp reported for the attachment, used for
	// the "· 1.2 MB" subline on document and voice rows. 0 when the sender
	// omitted it (and on every row stored before this field existed), in
	// which case the subline simply drops that segment.
	Size int64
	// DurationSecs is the playback length of an audio or video attachment,
	// shown as "0:12". 0 when unknown.
	DurationSecs int
}

// MediaItem is one image/video attachment in a chat, for the media/links/docs
// page's Media tab.
type MediaItem struct {
	MsgID     string
	Kind      string // "image" or "video"
	MimeType  string
	LocalPath string
	Thumbnail []byte
	TS        int64
}

// DocItem is one document attachment in a chat, for the Docs tab.
type DocItem struct {
	MsgID     string
	Filename  string
	MimeType  string
	LocalPath string
	TS        int64
}

// LinkItem is a message in a chat whose text contains a URL, for the Links
// tab: URL is the first URL found in the message, Title its full text.
type LinkItem struct {
	MsgID string
	URL   string
	Host  string
	Title string
	TS    int64
}

// GroupParticipant is one member of a group, as returned by GroupInfo.
type GroupParticipant struct {
	JID          string
	IsAdmin      bool
	IsSuperAdmin bool
}

// GroupInfo is a group's metadata and membership.
type GroupInfo struct {
	JID               string
	Name              string
	Topic             string
	OwnerJID          string
	Announce          bool   // only admins may send
	Locked            bool   // only admins may edit group info
	DisappearingTimer uint32 // seconds; 0 = off
	Participants      []GroupParticipant
	// IsParent marks a community (the parent group); LinkedParentJID is the
	// community a sub-group (announcement group included) belongs to.
	IsParent        bool
	LinkedParentJID string
}

// JoinRequest is a pending request to join an approval-required group; the
// requester's display name is resolved by the UI, not carried here.
type JoinRequest struct {
	JID         string
	RequestedAt time.Time
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
	// Paired reports whether a device is linked at all, connected or not:
	// the startup screen shows the loading mark (not the QR page) while a
	// paired account is still bringing its socket up.
	Paired() bool
	Logout(ctx context.Context) error
	Events() <-chan Event
	// PairPhone requests a phone-number pairing code as an alternative to
	// scanning the QR; call while connected-but-unpaired, same as QR pairing.
	PairPhone(ctx context.Context, phone string) (code string, err error)

	// reads go through the store, but exposed here so the fake can serve them too
	Chats(limit int) ([]Chat, error)
	Messages(jid string, limit int) ([]Message, error)
	// MessagesBefore returns up to limit messages older than beforeMsgID
	// (oldest first), for the conversation view's scroll-up paging.
	MessagesBefore(jid, beforeMsgID string, limit int) ([]Message, error)
	// RequestMoreHistory asks the phone for up to count messages older than
	// oldestMsgID in chatJID, for when MessagesBefore has run out of locally
	// stored history. The reply arrives asynchronously as an EventHistorySync
	// naming chatJID once ingested; this call only sends the request.
	RequestMoreHistory(ctx context.Context, chatJID, oldestMsgID string, count int) error
	Search(query string, limit int) ([]SearchHit, error)
	// SearchInChat runs query scoped to a single chat, oldest-first, for the
	// in-chat search bar's hit navigation.
	SearchInChat(chatJID, query string, limit int) ([]SearchHit, error)

	// writes
	SendText(ctx context.Context, jid, text string, replyTo *MsgRef) (string, error)
	SendMedia(ctx context.Context, jid string, m Attachment, replyTo *MsgRef) (string, error)
	SendLocation(ctx context.Context, jid string, loc Location, replyTo *MsgRef) (string, error)
	// SendLiveLocation shares a live location for durationSecs. This sends a
	// single initial update, not a continuous stream — real live-location
	// sharing periodically re-sends the position for the whole duration,
	// which is a follow-up (needs a background ticker + a way to cancel it).
	SendLiveLocation(ctx context.Context, jid string, lat, lon float64, durationSecs int) (string, error)
	// StopLiveLocation ends an own live share early: the bubble flips to
	// "Live location ended" locally and the open chat reloads.
	StopLiveLocation(ctx context.Context, chatJID, msgID string) error
	// SendContact shares a vCard built from contact's name/phone(s).
	SendContact(ctx context.Context, jid string, contact Contact, replyTo *MsgRef) (string, error)
	// ForwardMessage re-sends msg's content to toJID, marked as forwarded.
	ForwardMessage(ctx context.Context, msg Message, toJID string) (string, error)
	// ClearChat deletes jid's messages from local storage only — the phone
	// and the other party's device are never touched. If alsoMedia,
	// downloaded attachment files are also removed from the local cache.
	ClearChat(ctx context.Context, jid string, alsoMedia bool) error
	SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error)
	// SendSticker uploads the file at path and sends it as a sticker message.
	// Non-webp files are sent best-effort (no image->webp conversion here) and
	// may not render as a sticker on other clients.
	SendSticker(ctx context.Context, jid, path string) (string, error)
	// Stickers lists the sticker picker's library, most recently used
	// first: files added here plus the account's WhatsApp favourites.
	Stickers() ([]Sticker, error)
	// AddSticker files the picture at path in the library and marks it
	// used now; the same picture added twice is one entry.
	AddSticker(path string) (Sticker, error)
	// RemoveSticker takes a library entry out. A WhatsApp favourite is only
	// hidden on this device.
	RemoveSticker(ctx context.Context, key string) error
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
	// DeleteMessageForMe removes any message from this account only (the
	// other side keeps it): synced to the phone through app state and
	// dropped from the store, no tombstone.
	DeleteMessageForMe(ctx context.Context, chatJID, msgID string) error
	React(ctx context.Context, jid, msgID, emoji string) error // "" clears
	// MarkRead reports msgIDs in jid as read: to the account's other
	// devices always, so their badges clear, and to the senders too when
	// notifySender is set. It clears the local badge as well.
	MarkRead(ctx context.Context, jid string, msgIDs []string, notifySender bool) error
	// ClearUnread zeroes jid's local unread badge without telling anyone.
	ClearUnread(jid string) error
	// CheckOnWhatsApp looks up an E.164 phone number and reports its canonical
	// JID and whether it's registered on WhatsApp. onWhatsApp is false (with a
	// nil error) for a well-formed but unregistered number; err is reserved
	// for transport failures.
	CheckOnWhatsApp(ctx context.Context, phone string) (jid string, onWhatsApp bool, err error)
	SendPresence(available bool) error
	SendTyping(jid string, typing bool) error
	// SendRecording sends the "recording a voice note" chat-presence
	// (composing + audio media) when recording is true, else the plain
	// paused state.
	SendRecording(jid string, recording bool) error
	// PinChat, MuteChat, ArchiveChat and MarkChatUnread write chat-organization
	// app-state; reflected optimistically in the store and via an
	// EventChatUpdate.
	PinChat(ctx context.Context, jid string, pin bool) error
	MuteChat(ctx context.Context, jid string, mute bool) error
	// MuteChatFor mutes jid for d (WhatsApp's 8-hour and 1-week options);
	// MuteChat(true) is the "always" form.
	MuteChatFor(ctx context.Context, jid string, d time.Duration) error
	ArchiveChat(ctx context.Context, jid string, archive bool) error
	MarkChatUnread(ctx context.Context, jid string, unread bool) error
	// StarMessage stars/unstars msgID via app-state; reflected optimistically
	// in the store and via a refresh event for the open thread.
	StarMessage(ctx context.Context, chatJID, msgID string, starred bool) error
	// PinMessage pins (or unpins) msgID at the top of chatJID for everyone in
	// it, the way the ⋮ menu's "Pin in chat" row reads. WhatsApp keeps a pin
	// for a bounded time (chatot asks for 7 days).
	PinMessage(ctx context.Context, chatJID, msgID string, pin bool) error
	// StarredMessages returns starred messages across every chat, newest
	// first, for the starred-messages sidebar view.
	StarredMessages(limit int) ([]Message, error)
	// Statuses returns recent status ("stories") updates, newest first; each
	// message's FromJID is the poster. Backed by the status@broadcast chat.
	Statuses(limit int) ([]Message, error)
	// ChatMedia returns jid's image/video attachments, newest first, for the
	// media/links/docs page's Media tab.
	ChatMedia(jid string) ([]MediaItem, error)
	// ChatDocs returns jid's document attachments, newest first.
	ChatDocs(jid string) ([]DocItem, error)
	// ChatLinks returns jid's messages containing a URL, newest first, one
	// link per message (the first URL found).
	ChatLinks(jid string) ([]LinkItem, error)
	// PostStatus posts a text status update to the status broadcast.
	PostStatus(ctx context.Context, text string) error
	// RejectCall declines an incoming call offer identified by callID from
	// callJID. chatot never places or answers calls (whatsmeow can't); this
	// is the only call action supported.
	RejectCall(ctx context.Context, callJID, callID string) error

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
	// OwnJID returns this device's own user JID ("" if not logged in), so the
	// UI can decide whether the current user is a group admin/owner.
	OwnJID() string
	// ContactName resolves a participant or contact JID (phone-number or LID
	// form) to its display name: the address book / push name, else a
	// "+number" for a phone-number JID, else "". Used for group senders and
	// @mentions, which point at people who need not be chats of their own.
	ContactName(jid string) string
	// OwnName returns this account's own profile (push) name, "" if unknown,
	// for the account header and switcher.
	OwnName() string
	// CreateGroup creates a group named name with the given participant JIDs
	// (self is added implicitly); the new group is persisted as a chat and its
	// JID returned.
	CreateGroup(ctx context.Context, name string, participantJIDs []string) (jid string, err error)
	// LeaveGroup leaves jid; a refresh event is pushed so the UI updates.
	LeaveGroup(ctx context.Context, jid string) error
	// UpdateGroupParticipants adds/removes/promotes/demotes participants.
	// action is one of "add", "remove", "promote", "demote"; anything else is
	// an error. The group is re-fetched and a refresh event pushed on success.
	UpdateGroupParticipants(ctx context.Context, jid string, participantJIDs []string, action string) error
	// SetGroupName renames jid.
	SetGroupName(ctx context.Context, jid, name string) error
	// SetGroupTopic sets jid's topic/description.
	SetGroupTopic(ctx context.Context, jid, topic string) error
	// SetGroupAnnounce toggles announce mode (true = only admins can send).
	SetGroupAnnounce(ctx context.Context, jid string, announce bool) error
	// SetGroupLocked toggles locked mode (true = only admins can edit info).
	SetGroupLocked(ctx context.Context, jid string, locked bool) error
	// SetGroupPhoto replaces jid's group picture with jpeg (a JPEG, ideally
	// square; WhatsApp rejects other formats).
	SetGroupPhoto(ctx context.Context, jid string, jpeg []byte) error
	// SetGroupDisappearingTimer sets jid's disappearing-message timer in
	// seconds (0 disables it).
	SetGroupDisappearingTimer(ctx context.Context, jid string, seconds int64) error
	// GroupInviteLink returns jid's invite link, resetting it first if reset.
	GroupInviteLink(ctx context.Context, jid string, reset bool) (string, error)
	// JoinGroupWithLink joins via an invite link or bare code and returns the
	// joined group's JID, persisting it as a chat.
	JoinGroupWithLink(ctx context.Context, code string) (jid string, err error)
	// CreateCommunity creates a WhatsApp community (a parent group whose
	// linked announcement group is created automatically by the server) and
	// returns its JID, persisting it as a chat like a regular group.
	CreateCommunity(ctx context.Context, name, description string) (jid string, err error)
	// LinkGroupToCommunity links an existing group as a sub-group of
	// community.
	LinkGroupToCommunity(ctx context.Context, community, group string) error
	// GroupJoinRequests returns jid's pending join requests (approval-required
	// groups only); empty for a group with none.
	GroupJoinRequests(ctx context.Context, jid string) ([]JoinRequest, error)
	// ResolveGroupJoinRequest approves or rejects participantJID's pending
	// request to join groupJID.
	ResolveGroupJoinRequest(ctx context.Context, groupJID, participantJID string, approve bool) error

	// Newsletters returns the channels ("newsletters") this account is
	// subscribed to, fetched live from WhatsApp (never persisted).
	Newsletters(ctx context.Context) ([]Newsletter, error)
	// NewsletterMessages returns up to count recent posts in channel jid,
	// as returned by WhatsApp (newest first).
	NewsletterMessages(ctx context.Context, jid string, count int) ([]NewsletterMessage, error)
	// FollowNewsletter subscribes to channel jid.
	FollowNewsletter(ctx context.Context, jid string) error
	// UnfollowNewsletter unsubscribes from channel jid.
	UnfollowNewsletter(ctx context.Context, jid string) error
	// NewsletterSetMuted mutes/unmutes channel jid.
	NewsletterSetMuted(ctx context.Context, jid string, mute bool) error
	// NewsletterReact reacts to a channel post identified by messageID +
	// serverID with emoji ("" clears the reaction).
	NewsletterReact(ctx context.Context, jid, messageID string, serverID int64, emoji string) error
	// NewsletterMarkViewed reports the posts with serverIDs as viewed, which
	// is what feeds a channel's view counts.
	NewsletterMarkViewed(ctx context.Context, jid string, serverIDs []int64) error
	// NewsletterSubscribeLive asks for live view/reaction updates on jid for
	// a while; they arrive as EventNewsletterUpdate.
	NewsletterSubscribeLive(ctx context.Context, jid string) error
	// FollowNewsletterByLink resolves a whatsapp.com/channel/<key> link to a
	// channel and follows it, returning the followed channel's JID.
	FollowNewsletterByLink(ctx context.Context, link string) (jid string, err error)
	// DiscoverNewsletters searches WhatsApp's channel directory; query ""
	// lists the most followed channels. Backends without a directory return
	// ErrUnsupported.
	DiscoverNewsletters(ctx context.Context, query string) ([]Newsletter, error)

	// Communities lists the communities this account belongs to, each with
	// its linked groups.
	Communities(ctx context.Context) ([]Community, error)
	// JoinCommunityGroup joins a linked group of a community this account is
	// already in. Backends without the call return ErrUnsupported.
	JoinCommunityGroup(ctx context.Context, community, group string) error
	// ReactToStatus reacts to poster's status update msgID ("" clears).
	ReactToStatus(ctx context.Context, poster, msgID, emoji string) error
	// MarkStatusViewed records poster's updates msgIDs as viewed (they show
	// as read in Statuses) and, when notify is set, sends the poster the
	// read receipt WhatsApp counts as a view.
	MarkStatusViewed(ctx context.Context, poster string, msgIDs []string, notify bool) error
	// StatusViewers lists who viewed our update msgID, earliest first, as
	// assembled from their read receipts.
	StatusViewers(msgID string) ([]StatusViewer, error)
	// MuteStatus hides (or restores) poster's updates from the top of the
	// feed, synced through app-state where the backend supports it.
	MuteStatus(ctx context.Context, poster string, mute bool) error
	// MutedStatusPosters lists the posters muted with MuteStatus.
	MutedStatusPosters() ([]string, error)
	// HideStatusFrom stops jid from seeing our status updates, by adding
	// them to the status privacy exclusion list.
	HideStatusFrom(ctx context.Context, jid string) error
	// SetPrivacySetting changes one account privacy setting; name is a key
	// of PrivacySettings and value one of PrivacySettingOptions(name).
	SetPrivacySetting(ctx context.Context, name, value string) error

	// media
	DownloadMedia(ctx context.Context, msgID string) (localPath string, err error)
	// MarkViewOnceOpened marks msgID's view-once attachment as viewed, a
	// local-only tombstone: once set, the UI never re-offers it for opening.
	MarkViewOnceOpened(ctx context.Context, chatJID, msgID string) error
	// Avatar resolves jid's profile picture to a local cached file path,
	// fetching it on first use. Returns ("", nil) if there's no picture (or
	// it's not visible to us) — that's normal, not an error.
	Avatar(ctx context.Context, jid string) (localPath string, err error)
}
