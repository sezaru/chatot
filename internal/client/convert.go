package client

import (
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

// This file is the sole boundary between package client's value types and
// package store's: store defines its own plain Chat/Message/Attachment
// shapes (see internal/store/types.go) so it never needs to import client,
// keeping it a leaf, whatsmeow-agnostic, table-testable package.

func storeMessageRow(m *Message) store.MessageRow {
	row := store.MessageRow{
		ChatJID: m.ChatJID,
		MsgID:   m.ID,
		FromJID: m.FromJID,
		FromMe:  m.FromMe,
		Text:    m.Text,
		TS:      m.TS,
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
	return row
}

func storeMediaRow(chatJID, msgID string, a *Attachment) store.MediaRow {
	return store.MediaRow{
		ChatJID: chatJID, MsgID: msgID,
		Kind: a.Kind, Filename: a.Filename, Caption: a.Caption,
		MimeType: a.MimeType, LocalPath: a.LocalPath, ProtoBlob: a.ProtoBlob,
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
		Pinned: c.Pinned, Muted: c.Muted, IsGroup: c.IsGroup,
	}
}

func messageFromStore(m store.Message) Message {
	out := Message{
		ID: m.ID, ChatJID: m.ChatJID, FromJID: m.FromJID, FromMe: m.FromMe,
		Text: m.Text, TS: m.TS, Reactions: m.Reactions,
	}
	if m.ReplyToMsgID != "" {
		out.ReplyTo = &MsgRef{ChatJID: m.ChatJID, MsgID: m.ReplyToMsgID}
	}
	if m.Attachment != nil {
		out.Attachment = &Attachment{
			Kind: m.Attachment.Kind, Filename: m.Attachment.Filename,
			MimeType: m.Attachment.MimeType, LocalPath: m.Attachment.LocalPath,
			Caption: m.Attachment.Caption,
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
	}
	return out
}
