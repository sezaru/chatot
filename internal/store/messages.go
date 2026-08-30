package store

import "strings"

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

// Messages returns a chat's most recent messages (up to limit), oldest
// first, with reply context, reactions and media populated.
func (s *Store) Messages(jid string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT
			m.msg_id, m.from_jid, m.from_me, COALESCE(m.text, ''), m.ts, COALESCE(m.reply_to_msg_id, ''),
			COALESCE(md.kind, ''), COALESCE(md.filename, ''), COALESCE(md.caption, ''), COALESCE(md.mime_type, ''), COALESCE(md.local_path, '')
		FROM messages m
		LEFT JOIN media md ON md.chat_jid = m.chat_jid AND md.msg_id = m.msg_id
		WHERE m.chat_jid = ?
		ORDER BY m.ts DESC, m.rowid DESC
		LIMIT ?
	`, jid, limit)
	if err != nil {
		return nil, err
	}
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
