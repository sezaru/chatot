package store

import (
	"database/sql"
	"strings"
)

// UpsertMessage inserts or updates a message row. An empty ReplyToMsgID
// leaves any existing reply link untouched.
func (s *Store) UpsertMessage(row MessageRow) error {
	_, err := s.db.Exec(`
		INSERT INTO messages(chat_jid, msg_id, from_jid, from_me, text, ts, reply_to_msg_id)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''))
		ON CONFLICT(chat_jid, msg_id) DO UPDATE SET
			from_jid = excluded.from_jid,
			from_me = excluded.from_me,
			text = excluded.text,
			ts = excluded.ts,
			reply_to_msg_id = COALESCE(excluded.reply_to_msg_id, messages.reply_to_msg_id)
	`, row.ChatJID, row.MsgID, row.FromJID, boolToInt(row.FromMe), row.Text, row.TS, row.ReplyToMsgID)
	return err
}

// MessageByID looks up a single message by chat+id, without reactions/media
// (callers needing those should use Messages). ok is false if not found.
func (s *Store) MessageByID(chatJID, msgID string) (m Message, ok bool, err error) {
	row := s.db.QueryRow(`
		SELECT msg_id, from_jid, from_me, COALESCE(text, ''), ts
		FROM messages WHERE chat_jid = ? AND msg_id = ?
	`, chatJID, msgID)
	var fromMe int
	if err := row.Scan(&m.ID, &m.FromJID, &fromMe, &m.Text, &m.TS); err != nil {
		if err == sql.ErrNoRows {
			return Message{}, false, nil
		}
		return Message{}, false, err
	}
	m.ChatJID = chatJID
	m.FromMe = fromMe != 0
	return m, true, nil
}

// messageSelect is the shared column list + joins for a page of messages;
// callers append their own WHERE/ORDER/LIMIT. Kept identical across Messages
// and MessagesBefore so both pages scan the same shape (see scanMessages).
const messageSelect = `
	SELECT
		m.msg_id, m.from_jid, m.from_me, COALESCE(m.text, ''), m.ts, COALESCE(m.reply_to_msg_id, ''),
		COALESCE(md.kind, ''), COALESCE(md.filename, ''), COALESCE(md.caption, ''), COALESCE(md.mime_type, ''), COALESCE(md.local_path, '')
	FROM messages m
	LEFT JOIN media md ON md.chat_jid = m.chat_jid AND md.msg_id = m.msg_id`

// Messages returns a chat's most recent messages (up to limit), oldest
// first, with reply context, reactions and media populated.
func (s *Store) Messages(jid string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(messageSelect+`
		WHERE m.chat_jid = ?
		ORDER BY m.ts DESC, m.rowid DESC
		LIMIT ?
	`, jid, limit)
	if err != nil {
		return nil, err
	}
	return s.pageFromRows(jid, rows)
}

// MessagesBefore returns up to limit messages strictly older than beforeMsgID
// in jid (oldest first, same enrichment as Messages) — the older-page fetch
// the conversation view issues as the reader scrolls up. Returns nil (no
// error) if beforeMsgID isn't in the store. The (ts, rowid) cursor breaks
// same-second ties so no message is skipped or repeated across pages.
func (s *Store) MessagesBefore(jid, beforeMsgID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	var ts, rowid int64
	err := s.db.QueryRow(`SELECT ts, rowid FROM messages WHERE chat_jid = ? AND msg_id = ?`, jid, beforeMsgID).Scan(&ts, &rowid)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(messageSelect+`
		WHERE m.chat_jid = ? AND (m.ts < ? OR (m.ts = ? AND m.rowid < ?))
		ORDER BY m.ts DESC, m.rowid DESC
		LIMIT ?
	`, jid, ts, ts, rowid, limit)
	if err != nil {
		return nil, err
	}
	return s.pageFromRows(jid, rows)
}

// pageFromRows scans a newest-first messageSelect result, reverses it to
// chronological order, and populates reactions. It closes rows.
func (s *Store) pageFromRows(jid string, rows *sql.Rows) ([]Message, error) {
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var fromMe int
		var mediaKind, mediaFilename, mediaCaption, mediaMime, mediaLocal string
		if err := rows.Scan(
			&m.ID, &m.FromJID, &fromMe, &m.Text, &m.TS, &m.ReplyToMsgID,
			&mediaKind, &mediaFilename, &mediaCaption, &mediaMime, &mediaLocal,
		); err != nil {
			return nil, err
		}
		m.ChatJID = jid
		m.FromMe = fromMe != 0
		if mediaKind != "" {
			m.Attachment = &Attachment{
				Kind: mediaKind, Filename: mediaFilename, Caption: mediaCaption,
				MimeType: mediaMime, LocalPath: mediaLocal,
			}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The query orders newest-first for LIMIT; return chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	ids := make([]string, len(out))
	for i, m := range out {
		ids[i] = m.ID
	}
	reactions, err := s.reactionsFor(jid, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if r := reactions[out[i].ID]; len(r) > 0 {
			out[i].Reactions = r
		}
	}
	return out, nil
}

func (s *Store) reactionsFor(chatJID string, msgIDs []string) (map[string]map[string]string, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(msgIDs)), ",")
	args := make([]any, 0, len(msgIDs)+1)
	args = append(args, chatJID)
	for _, id := range msgIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(`
		SELECT msg_id, reactor_jid, emoji FROM reactions
		WHERE chat_jid = ? AND msg_id IN (`+placeholders+`)
		ORDER BY ts ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]string)
	for rows.Next() {
		var msgID, reactor, emoji string
		if err := rows.Scan(&msgID, &reactor, &emoji); err != nil {
			return nil, err
		}
		if out[msgID] == nil {
			out[msgID] = make(map[string]string)
		}
		out[msgID][emoji] = reactor
	}
	return out, rows.Err()
}
