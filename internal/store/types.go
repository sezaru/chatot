// Package store is chatot's local sqlite message store — separate from
// whatsmeow's own sqlstore auth/session database. It holds chats, messages,
// contacts, groups, reactions and media, and owns the name-resolution,
// ordering, preview and chat-list filter logic. It has no dependency on
// internal/client or whatsmeow: callers (internal/client/whatsmeow.go) map
// between whatsmeow/client types and the plain rows/results defined here,
// which keeps this package pure and table-testable.
package store

// Chat is a resolved chat-list entry, ready for display.
type Chat struct {
	JID           string
	Name          string
	Preview       string
	UnreadCount   int
	LastMessageTS int64
	Pinned        bool
	Muted         bool
	IsGroup       bool
}

// Message is a resolved conversation message, ready for display.
type Message struct {
	ID           string
	ChatJID      string
	FromJID      string
	FromMe       bool
	Text         string
	TS           int64
	ReplyToMsgID string
	Reactions    map[string]string // emoji -> reactor JID (last wins)
	Attachment   *Attachment
	// Kind is "" for a plain text/media message or a rich-kind tag (e.g.
	// "location"); Payload is the opaque JSON body only package client
	// decodes. The store persists and returns them without interpreting them.
	Kind    string
	Payload string
	// Edited is true once a MESSAGE_EDIT updated this message's text.
	Edited bool
	// Deleted is true once a REVOKE ("delete for everyone") applied to this
	// message; the original content is kept but should never be rendered.
	Deleted bool
	// PollVotes holds the raw votes cast on a poll message (kind == "poll"):
	// one row per (voter, selected-option-hash). The store never hashes or
	// interprets these; package client matches the hashes against the poll's
	// option names to compute per-option counts.
	PollVotes []PollVoteRow
}

// PollVoteRow is a single voter's selection of one poll option, identified by
// the SHA-256 hash of the option name (WhatsApp transmits votes as hashes).
type PollVoteRow struct {
	VoterJID   string
	OptionHash []byte
}

// Attachment describes a message's media, as resolved from the media table.
type Attachment struct {
	Kind      string
	Filename  string
	MimeType  string
	LocalPath string
	Caption   string
}

// SearchHit is a single Search result: either a message match (MsgID set,
// Snippet an excerpt) or a chat-name match (MsgID "", Snippet == ChatName).
type SearchHit struct {
	ChatJID  string
	MsgID    string
	ChatName string
	Snippet  string
	TS       int64
}

// ChatRow is the upsert seam for the chats table.
type ChatRow struct {
	JID           string
	IsGroup       bool
	Name          string // "" leaves the existing name untouched
	Pinned        bool
	Muted         bool
	UnreadCount   int
	LastMessageTS int64
}

// MessageRow is the upsert seam for the messages table.
type MessageRow struct {
	ChatJID      string
	MsgID        string
	FromJID      string
	FromMe       bool
	Text         string
	TS           int64
	ReplyToMsgID string // "" leaves any existing reply link untouched
	Kind         string // "" plain message; "location" etc. for a rich kind
	Payload      string // opaque JSON body for a rich kind, "" otherwise
	Edited       bool   // true for a MESSAGE_EDIT upsert; sticky in the store
	Deleted      bool   // true for a REVOKE upsert; sticky in the store
}

// ContactRow is the upsert seam for the contacts table. Empty fields leave
// the existing value untouched, since contact info can arrive piecemeal.
type ContactRow struct {
	JID          string
	BusinessName string
	FullName     string
	PushName     string
	SystemName   string
}

// GroupRow is the upsert seam for the groups table.
type GroupRow struct {
	JID             string
	Name            string
	IsParent        bool   // community parent group
	LinkedParentJID string // set for a group linked to (e.g. an announcement
	// channel of) a community; non-empty excludes it from the chat list
}

// ReactionRow is the upsert seam for the reactions table. An empty Emoji
// clears (deletes) the reactor's reaction on the message.
type ReactionRow struct {
	ChatJID    string
	MsgID      string
	ReactorJID string
	Emoji      string
	TS         int64
}

// MediaRow is the upsert seam for the media table, one row per message that
// carries an attachment.
type MediaRow struct {
	ChatJID   string
	MsgID     string
	Kind      string // "image", "video", "audio", "document", "sticker"
	Filename  string
	Caption   string
	MimeType  string
	LocalPath string
	// ProtoBlob is proto.Marshal of the specific waE2E.*Message (nil until
	// the caller sets it); empty leaves any existing value untouched.
	ProtoBlob []byte
}
