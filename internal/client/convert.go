package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"

	"chatot/internal/store"
)

// locationPayload is the JSON shape stored in a message row's opaque payload
// when Kind == "location". Only package client marshals/unmarshals it; the
// store persists the string verbatim.
type locationPayload struct {
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"long"`
	IsLive    bool    `json:"live,omitempty"`
}

// contactPayload is the JSON shape stored in a message row's opaque payload
// when Kind == "contact".
type contactPayload struct {
	DisplayName string   `json:"name,omitempty"`
	Phones      []string `json:"phones,omitempty"`
}

// pollPayload is the JSON shape stored when Kind == "poll": the immutable poll
// definition only. Vote counts are NOT stored here — they change with every
// vote and are tallied from poll_votes at read time (see messageFromStore).
type pollPayload struct {
	Name       string   `json:"name"`
	Options    []string `json:"options"`
	Selectable int      `json:"selectable,omitempty"`
}

// hashPollOption returns the SHA-256 of an option name, the form WhatsApp
// transmits votes in and poll_votes stores. Vote tallying matches these
// against the poll's option names.
func hashPollOption(name string) []byte {
	sum := sha256.Sum256([]byte(name))
	return sum[:]
}

// This file is the sole boundary between package client's value types and
// package store's: store defines its own plain Chat/Message/Attachment
// shapes (see internal/store/types.go) so it never needs to import client,
// keeping it a leaf, whatsmeow-agnostic, table-testable package.

func storeMessageRow(m *Message) store.MessageRow {
	row := store.MessageRow{
		ChatJID:   m.ChatJID,
		MsgID:     m.ID,
		FromJID:   m.FromJID,
		FromMe:    m.FromMe,
		Text:      m.Text,
		TS:        m.TS,
		Edited:    m.Edited,
		Deleted:   m.Deleted,
		Forwarded: m.Forwarded,
	}
	if m.ReplyTo != nil {
		row.ReplyToMsgID = m.ReplyTo.MsgID
	}
	// Rich kinds serialize their typed body into the opaque payload. Adding a
	// future kind (contact, poll) is another case here + in messageFromStore.
	if m.Location != nil {
		row.Kind = "location"
		if b, err := json.Marshal(locationPayload{
			Name: m.Location.Name, Address: m.Location.Address,
			Latitude: m.Location.Latitude, Longitude: m.Location.Longitude,
			IsLive: m.Location.IsLive,
		}); err == nil {
			row.Payload = string(b)
		}
	}
	if m.Contact != nil {
		row.Kind = "contact"
		if b, err := json.Marshal(contactPayload{
			DisplayName: m.Contact.DisplayName, Phones: m.Contact.Phones,
		}); err == nil {
			row.Payload = string(b)
		}
	}
	if m.Poll != nil {
		row.Kind = "poll"
		names := make([]string, len(m.Poll.Options))
		for i, o := range m.Poll.Options {
			names[i] = o.Name
		}
		if b, err := json.Marshal(pollPayload{
			Name: m.Poll.Name, Options: names, Selectable: m.Poll.SelectableCount,
		}); err == nil {
			row.Payload = string(b)
		}
	}
	return row
}

func storeMediaRow(chatJID, msgID string, a *Attachment) store.MediaRow {
	return store.MediaRow{
		ChatJID: chatJID, MsgID: msgID,
		Kind: a.Kind, Filename: a.Filename, Caption: a.Caption,
		MimeType: a.MimeType, LocalPath: a.LocalPath, ProtoBlob: a.ProtoBlob,
		Thumbnail: a.Thumbnail, IsGif: a.IsGIF,
	}
}

func storeReactionRow(r *Reaction) store.ReactionRow {
	return store.ReactionRow{
		ChatJID: r.ChatJID, MsgID: r.MsgID, ReactorJID: r.ReactorJID,
		Emoji: r.Emoji, TS: r.TS,
	}
}

func chatFromStore(c store.Chat) Chat {
	return Chat{
		JID: c.JID, Name: c.Name, Preview: c.Preview,
		UnreadCount: c.UnreadCount, LastMessageTS: c.LastMessageTS,
		Pinned: c.Pinned, Muted: c.Muted, Archived: c.Archived, IsGroup: c.IsGroup,
	}
}

// messageFromStore maps a store row to a client.Message. selfJID is this
// device's own JID (or "" for tests / when logged out), used to mark which
// poll options the local user voted for.
func messageFromStore(m store.Message, selfJID string) Message {
	out := Message{
		ID: m.ID, ChatJID: m.ChatJID, FromJID: m.FromJID, FromMe: m.FromMe,
		Text: m.Text, TS: m.TS, Reactions: m.Reactions, Edited: m.Edited, Deleted: m.Deleted,
		Status: m.Status, Starred: m.Starred, Forwarded: m.Forwarded,
	}
	if m.ReplyToMsgID != "" {
		out.ReplyTo = &MsgRef{ChatJID: m.ChatJID, MsgID: m.ReplyToMsgID}
	}
	if m.Attachment != nil {
		out.Attachment = &Attachment{
			Kind: m.Attachment.Kind, Filename: m.Attachment.Filename,
			MimeType: m.Attachment.MimeType, LocalPath: m.Attachment.LocalPath,
			Caption: m.Attachment.Caption, Thumbnail: m.Attachment.Thumbnail,
			IsGIF: m.Attachment.IsGif,
		}
	}
	switch m.Kind {
	case "location":
		var p locationPayload
		if err := json.Unmarshal([]byte(m.Payload), &p); err == nil {
			out.Location = &Location{
				Name: p.Name, Address: p.Address,
				Latitude: p.Latitude, Longitude: p.Longitude, IsLive: p.IsLive,
			}
		}
	case "contact":
		var p contactPayload
		if err := json.Unmarshal([]byte(m.Payload), &p); err == nil {
			out.Contact = &Contact{DisplayName: p.DisplayName, Phones: p.Phones}
		}
	case "poll":
		var p pollPayload
		if err := json.Unmarshal([]byte(m.Payload), &p); err == nil {
			out.Poll = pollFromStore(p, m.PollVotes, selfJID)
		}
	}
	return out
}

// pollFromStore rebuilds a Poll with live per-option tallies: for each option
// it hashes the name and counts the votes whose hash matches (Voted marks the
// options selfJID selected). The store stays hash-agnostic; only here, where
// the option names are known, do the hashes get resolved back to options.
func pollFromStore(p pollPayload, votes []store.PollVoteRow, selfJID string) *Poll {
	poll := &Poll{Name: p.Name, SelectableCount: p.Selectable, Options: make([]PollOption, len(p.Options))}
	for i, name := range p.Options {
		hash := hashPollOption(name)
		opt := PollOption{Name: name}
		for _, v := range votes {
			if !bytes.Equal(v.OptionHash, hash) {
				continue
			}
			opt.Count++
			if selfJID != "" && v.VoterJID == selfJID {
				opt.Voted = true
			}
		}
		poll.Options[i] = opt
	}
	return poll
}
