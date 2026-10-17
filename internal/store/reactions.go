package store

import (
	"database/sql"
	"errors"
	"strings"
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

// AddUnseenReaction records r as not yet read. A reactor holds one reaction
// per message, so a changed one replaces the earlier row.
func (s *Store) AddUnseenReaction(r UnseenReaction) error {
	_, err := s.db.Exec(`
		INSERT INTO unseen_reactions(chat_jid, msg_id, reactor_jid, reaction_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(chat_jid, msg_id, reactor_jid) DO UPDATE SET
			reaction_id = excluded.reaction_id
	`, r.ChatJID, r.MsgID, r.ReactorJID, r.ReactionID)
	return err
}

// ClearUnseenReaction forgets reactorJID's unread reaction on msgID, for a
// reaction taken back before it was read.
func (s *Store) ClearUnseenReaction(chatJID, msgID, reactorJID string) error {
	_, err := s.db.Exec(`
		DELETE FROM unseen_reactions WHERE chat_jid = ? AND msg_id = ? AND reactor_jid = ?
	`, chatJID, msgID, reactorJID)
	return err
}

// UnseenReactions lists chatJID's unread reactions.
func (s *Store) UnseenReactions(chatJID string) ([]UnseenReaction, error) {
	rows, err := s.db.Query(`
		SELECT msg_id, reactor_jid, reaction_id FROM unseen_reactions WHERE chat_jid = ?
	`, chatJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnseenReaction
	for rows.Next() {
		r := UnseenReaction{ChatJID: chatJID}
		if err := rows.Scan(&r.MsgID, &r.ReactorJID, &r.ReactionID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DropUnseenReactions marks the reactions with the given reaction ids in
// chatJID as read. Ids that are not unread reactions are ignored, so a
// receipt's message ids can be passed as they are.
func (s *Store) DropUnseenReactions(chatJID string, reactionIDs []string) error {
	if len(reactionIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(reactionIDs)), ",")
	args := make([]any, 0, len(reactionIDs)+1)
	args = append(args, chatJID)
	for _, id := range reactionIDs {
		args = append(args, id)
	}
	_, err := s.db.Exec(`
		DELETE FROM unseen_reactions
		WHERE chat_jid = ? AND reaction_id IN (`+placeholders+`)
	`, args...)
	return err
}
