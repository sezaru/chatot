package store

import (
	"database/sql"
	"errors"
)

// UpsertReaction sets (or, if Emoji is "", clears) a reactor's reaction on a
// message — mirroring WhatsApp's "tap the same emoji again to remove it".
func (s *Store) UpsertReaction(row ReactionRow) error {
	if row.Emoji == "" {
		_, err := s.db.Exec(`
			DELETE FROM reactions WHERE chat_jid = ? AND msg_id = ? AND reactor_jid = ?
		`, row.ChatJID, row.MsgID, row.ReactorJID)
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO reactions(chat_jid, msg_id, reactor_jid, emoji, ts)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(chat_jid, msg_id, reactor_jid) DO UPDATE SET
			emoji = excluded.emoji,
			ts = excluded.ts
	`, row.ChatJID, row.MsgID, row.ReactorJID, row.Emoji, row.TS)
	return err
}

// ReactionBy returns reactorJID's current reaction on a message, or "" when
// there is none. Used to remember what to put back if a reaction shown
// optimistically turns out not to have been sent.
func (s *Store) ReactionBy(chatJID, msgID, reactorJID string) (string, error) {
	var emoji string
	err := s.db.QueryRow(`
		SELECT emoji FROM reactions
		WHERE chat_jid = ? AND msg_id = ? AND reactor_jid = ?
	`, chatJID, msgID, reactorJID).Scan(&emoji)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return emoji, err
}
