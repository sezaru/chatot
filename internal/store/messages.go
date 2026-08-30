package store

import (
	"database/sql"
	"strings"
)

// UpsertMessage inserts or updates a message row. An empty ReplyToMsgID
// leaves any existing reply link untouched.
func (s *Store) UpsertMessage(row MessageRow) error {
	_, err := s.db.Exec(`
		INSERT INTO messages(chat_jid, msg_id, from_jid, from_me, text, ts, reply_to_msg_id, kind, payload, edited, deleted, forwarded)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?, ?)
		ON CONFLICT(chat_jid, msg_id) DO UPDATE SET
			from_jid = excluded.from_jid,
			from_me = excluded.from_me,
			text = excluded.text,
			ts = excluded.ts,
			reply_to_msg_id = COALESCE(excluded.reply_to_msg_id, messages.reply_to_msg_id),
			kind = excluded.kind,
			payload = excluded.payload,
			edited = messages.edited OR excluded.edited,
			deleted = messages.deleted OR excluded.deleted,
			forwarded = messages.forwarded OR excluded.forwarded
	`, row.ChatJID, row.MsgID, row.FromJID, boolToInt(row.FromMe), row.Text, row.TS, row.ReplyToMsgID, row.Kind, row.Payload, boolToInt(row.Edited), boolToInt(row.Deleted), boolToInt(row.Forwarded))
	return err
}

// SetMessagesStatus advances the delivery/read status of the given messages
// in chatJID to status, never downgrading it (a delivered receipt arriving
// after a read one leaves the message at read). No-op for ids not found.
func (s *Store) SetMessagesStatus(chatJID string, msgIDs []string, status int) error {
	if len(msgIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(msgIDs)), ",")
	args := make([]any, 0, len(msgIDs)+2)
	args = append(args, status, chatJID)
	for _, id := range msgIDs {
		args = append(args, id)
	}
	_, err := s.db.Exec(`
		UPDATE messages SET status = MAX(status, ?)
		WHERE chat_jid = ? AND msg_id IN (`+placeholders+`)
	`, args...)
	return err
}

// MarkMessageDeleted applies a "delete for everyone" (REVOKE) to msgID: it
// sets deleted=1 sticky, inserting a minimal stub row if the original message
// hasn't been seen yet (a revoke can arrive before, or without, the original).
func (s *Store) MarkMessageDeleted(chatJID, msgID string, ts int64) error {
	_, err := s.db.Exec(`
		INSERT INTO messages(chat_jid, msg_id, from_jid, ts, deleted)
		VALUES (?, ?, '', ?, 1)
		ON CONFLICT(chat_jid, msg_id) DO UPDATE SET deleted = 1
	`, chatJID, msgID, ts)
	return err
}

// SetMessageStarred sets a message's starred flag, leaving every other
// column untouched.
func (s *Store) SetMessageStarred(chatJID, msgID string, starred bool) error {
	_, err := s.db.Exec(`
		UPDATE messages SET starred = ? WHERE chat_jid = ? AND msg_id = ?
	`, boolToInt(starred), chatJID, msgID)
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

// statusBroadcastJID is the special chat every WhatsApp status ("story")
// message belongs to. It's excluded from the normal chat list (see Chats)
// and surfaced on its own via Statuses.
const statusBroadcastJID = "status@broadcast"

// Statuses returns recent status ("stories") updates — messages whose chat
// is statusBroadcastJID — newest first, with the same media/reaction
// enrichment as Messages. Each row's ChatJID is statusBroadcastJID and its
// FromJID is the poster.
func (s *Store) Statuses(limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(messageSelect+`
		WHERE m.chat_jid = ?
		ORDER BY m.ts DESC, m.rowid DESC
		LIMIT ?
	`, statusBroadcastJID, limit)
	if err != nil {
		return nil, err
	}
	// pageFromRows returns oldest-first; a status feed reads newest-first.
	msgs, err := s.pageFromRows(statusBroadcastJID, rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// messageSelect is the shared column list + joins for a page of messages;
// callers append their own WHERE/ORDER/LIMIT. Kept identical across Messages
// and MessagesBefore so both pages scan the same shape (see scanMessages).
const messageSelect = `
	SELECT
		m.msg_id, m.from_jid, m.from_me, COALESCE(m.text, ''), m.ts, COALESCE(m.reply_to_msg_id, ''),
		m.kind, COALESCE(m.payload, ''), m.edited, m.deleted, m.status, m.starred, m.forwarded,
		COALESCE(md.kind, ''), COALESCE(md.filename, ''), COALESCE(md.caption, ''), COALESCE(md.mime_type, ''), COALESCE(md.local_path, ''), md.thumbnail, COALESCE(md.is_gif, 0)
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
		var fromMe, edited, deleted, starred, forwarded int
		var mediaKind, mediaFilename, mediaCaption, mediaMime, mediaLocal string
		var mediaThumb []byte
		var mediaIsGif int
		if err := rows.Scan(
			&m.ID, &m.FromJID, &fromMe, &m.Text, &m.TS, &m.ReplyToMsgID,
			&m.Kind, &m.Payload, &edited, &deleted, &m.Status, &starred, &forwarded,
			&mediaKind, &mediaFilename, &mediaCaption, &mediaMime, &mediaLocal, &mediaThumb, &mediaIsGif,
		); err != nil {
			return nil, err
		}
		m.ChatJID = jid
		m.FromMe = fromMe != 0
		m.Edited = edited != 0
		m.Deleted = deleted != 0
		m.Starred = starred != 0
		m.Forwarded = forwarded != 0
		if mediaKind != "" {
			m.Attachment = &Attachment{
				Kind: mediaKind, Filename: mediaFilename, Caption: mediaCaption,
				MimeType: mediaMime, LocalPath: mediaLocal, Thumbnail: mediaThumb, IsGif: mediaIsGif != 0,
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

	var pollIDs []string
	for _, m := range out {
		if m.Kind == "poll" {
			pollIDs = append(pollIDs, m.ID)
		}
	}
	if len(pollIDs) > 0 {
		votes, err := s.pollVotesFor(jid, pollIDs)
		if err != nil {
			return nil, err
		}
		for i := range out {
			if v := votes[out[i].ID]; len(v) > 0 {
				out[i].PollVotes = v
			}
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

// StarredMessages returns starred messages across every chat, newest first,
// with the same reaction/media/poll enrichment as Messages/MessagesBefore.
// Unlike pageFromRows (scoped to one chat, so it can stamp ChatJID from its
// jid argument), each row here carries its own chat_jid; enrichment queries
// are grouped per chat since reactionsFor/pollVotesFor are chat-scoped.
func (s *Store) StarredMessages(limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT
			m.chat_jid, m.msg_id, m.from_jid, m.from_me, COALESCE(m.text, ''), m.ts, COALESCE(m.reply_to_msg_id, ''),
			m.kind, COALESCE(m.payload, ''), m.edited, m.deleted, m.status,
			COALESCE(md.kind, ''), COALESCE(md.filename, ''), COALESCE(md.caption, ''), COALESCE(md.mime_type, ''), COALESCE(md.local_path, ''), md.thumbnail, COALESCE(md.is_gif, 0)
		FROM messages m
		LEFT JOIN media md ON md.chat_jid = m.chat_jid AND md.msg_id = m.msg_id
		WHERE m.starred = 1
		ORDER BY m.ts DESC, m.rowid DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var fromMe, edited, deleted int
		var mediaKind, mediaFilename, mediaCaption, mediaMime, mediaLocal string
		var mediaThumb []byte
		var mediaIsGif int
		if err := rows.Scan(
			&m.ChatJID, &m.ID, &m.FromJID, &fromMe, &m.Text, &m.TS, &m.ReplyToMsgID,
			&m.Kind, &m.Payload, &edited, &deleted, &m.Status,
			&mediaKind, &mediaFilename, &mediaCaption, &mediaMime, &mediaLocal, &mediaThumb, &mediaIsGif,
		); err != nil {
			return nil, err
		}
		m.FromMe = fromMe != 0
		m.Edited = edited != 0
		m.Deleted = deleted != 0
		m.Starred = true
		if mediaKind != "" {
			m.Attachment = &Attachment{
				Kind: mediaKind, Filename: mediaFilename, Caption: mediaCaption,
				MimeType: mediaMime, LocalPath: mediaLocal, Thumbnail: mediaThumb, IsGif: mediaIsGif != 0,
			}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	byChat := make(map[string][]string)
	var pollIDsByChat = make(map[string][]string)
	for _, m := range out {
		byChat[m.ChatJID] = append(byChat[m.ChatJID], m.ID)
		if m.Kind == "poll" {
			pollIDsByChat[m.ChatJID] = append(pollIDsByChat[m.ChatJID], m.ID)
		}
	}
	for chatJID, ids := range byChat {
		reactions, err := s.reactionsFor(chatJID, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			if out[i].ChatJID != chatJID {
				continue
			}
			if r := reactions[out[i].ID]; len(r) > 0 {
				out[i].Reactions = r
			}
		}
	}
	for chatJID, ids := range pollIDsByChat {
		votes, err := s.pollVotesFor(chatJID, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			if out[i].ChatJID != chatJID {
				continue
			}
			if v := votes[out[i].ID]; len(v) > 0 {
				out[i].PollVotes = v
			}
		}
	}
	return out, nil
}
