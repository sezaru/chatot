package store

import (
	"database/sql"
	"sort"
	"strings"
)

// UpsertChat inserts or updates a chat row. An empty Name leaves any
// existing name untouched (chat metadata often arrives piecemeal).
func (s *Store) UpsertChat(row ChatRow) error { return s.UpsertChatFlags(row, true, true) }

// UpsertChatFlags is UpsertChat where setPinned/setMuted say whether row's
// Pinned/Muted are known. A history-sync conversation often carries neither
// (both live in app state, which syncs separately), and overwriting them
// with false on every chunk would undo the mute the phone just reported.
// On insert an unknown flag starts false.
//
// The row's unread count and timestamp are a snapshot of the chat as of
// its last message: they replace the stored ones only when they are at
// least as new. A history chunk the phone assembled before a message
// arrived live says nothing about that message, and taking its count
// would clear the badge the message just raised.
func (s *Store) UpsertChatFlags(row ChatRow, setPinned, setMuted bool) error {
	_, err := s.db.Exec(`
		INSERT INTO chats(jid, is_group, name, pinned, muted, unread_count, last_message_ts)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			is_group = excluded.is_group,
			name = COALESCE(excluded.name, chats.name),
			pinned = CASE WHEN ? THEN excluded.pinned ELSE chats.pinned END,
			muted = CASE WHEN ? THEN excluded.muted ELSE chats.muted END,
			unread_count = CASE WHEN excluded.last_message_ts >= chats.last_message_ts
				THEN excluded.unread_count ELSE chats.unread_count END,
			last_message_ts = MAX(excluded.last_message_ts, chats.last_message_ts)
	`, row.JID, boolToInt(row.IsGroup), row.Name, boolToInt(row.Pinned), boolToInt(row.Muted), row.UnreadCount, row.LastMessageTS,
		boolToInt(setPinned), boolToInt(setMuted))
	return err
}

// RepairGroupSenders blanks the sender of group messages filed under the
// group itself: history sync once read the sender off the message key,
// which groups leave empty, and fell back to the chat as for a DM. The
// real sender is not recoverable from the store; a blank one at least
// stops the bubbles naming (and picturing) the group. Returns how many
// messages changed.
func (s *Store) RepairGroupSenders() (int64, error) {
	res, err := s.db.Exec(`
		UPDATE messages SET from_jid = ''
		WHERE from_me = 0 AND chat_jid LIKE '%@g.us' AND from_jid = chat_jid
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BumpChatActivity records a message's activity on a chat: it ensures the
// chat row exists, raises last_message_ts to the newer of the two values,
// and adds unreadDelta (typically +1 for an inbound message) to unread_count,
// clamped at 0. Existing pinned/muted/name state is left untouched.
func (s *Store) BumpChatActivity(jid string, isGroup bool, ts int64, unreadDelta int) error {
	initialUnread := unreadDelta
	if initialUnread < 0 {
		initialUnread = 0
	}
	_, err := s.db.Exec(`
		INSERT INTO chats(jid, is_group, last_message_ts, unread_count)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			is_group = excluded.is_group,
			last_message_ts = MAX(chats.last_message_ts, excluded.last_message_ts),
			unread_count = MAX(chats.unread_count + ?, 0)
	`, jid, boolToInt(isGroup), ts, initialUnread, unreadDelta)
	return err
}

// MarkChatRead clears a chat's unread count.
func (s *Store) MarkChatRead(jid string) error {
	_, err := s.db.Exec(`UPDATE chats SET unread_count = 0 WHERE jid = ?`, jid)
	return err
}

// SetChatPinned sets a chat's pinned flag.
func (s *Store) SetChatPinned(jid string, pinned bool) error {
	_, err := s.db.Exec(`UPDATE chats SET pinned = ? WHERE jid = ?`, boolToInt(pinned), jid)
	return err
}

// SetChatMuted sets a chat's muted flag.
func (s *Store) SetChatMuted(jid string, muted bool) error {
	_, err := s.db.Exec(`UPDATE chats SET muted = ? WHERE jid = ?`, boolToInt(muted), jid)
	return err
}

// SetChatArchived sets a chat's archived flag.
func (s *Store) SetChatArchived(jid string, archived bool) error {
	_, err := s.db.Exec(`UPDATE chats SET archived = ? WHERE jid = ?`, boolToInt(archived), jid)
	return err
}

// ChatLastMessageTS returns a chat's last_message_ts (0 if the chat has no
// row yet), for building app-state patches that require it.
func (s *Store) ChatLastMessageTS(jid string) (int64, error) {
	var ts int64
	err := s.db.QueryRow(`SELECT last_message_ts FROM chats WHERE jid = ?`, jid).Scan(&ts)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return ts, err
}

// SetChatUnread marks a chat unread (unread_count raised to at least 1,
// existing higher counts left untouched) or read (unread_count cleared to 0,
// same as MarkChatRead).
func (s *Store) SetChatUnread(jid string, unread bool) error {
	if !unread {
		return s.MarkChatRead(jid)
	}
	_, err := s.db.Exec(`UPDATE chats SET unread_count = MAX(unread_count, 1) WHERE jid = ?`, jid)
	return err
}

// Chats returns the chat list: name-resolved, ordered, preview-populated,
// and filtered to DMs plus groups other than communities themselves (a
// community's sub-groups, its announcement group included, are chats like
// any other, as in WhatsApp). See the design doc
// ("Store: name resolution / ordering / preview") for the reference rules.
func (s *Store) Chats(limit int) ([]Chat, error) {
	// limit <= 0 is every chat (as the fake reads it): the list and every
	// name lookup go through here, and a chat quiet since last year is
	// still a chat.
	lastReactions, err := s.latestOwnMessageReactions()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT
			c.jid, c.is_group, COALESCE(c.name, ''), c.pinned, c.muted, c.archived, c.unread_count, c.last_message_ts,
			COALESCE(g.name, ''), COALESCE(g.is_parent, 0), COALESCE(g.linked_parent_jid, ''),
			COALESCE(ct.business_name, ''), COALESCE(ct.full_name, ''), COALESCE(ct.push_name, ''), COALESCE(ct.system_name, ''), COALESCE(ct.pn_jid, ''),
			COALESCE(lm.from_me, 0), COALESCE(lm.text, ''), COALESCE(lm.kind, ''), COALESCE(lm.payload, ''), COALESCE(lm.ts, 0),
			COALESCE(md.kind, ''), COALESCE(md.caption, ''), COALESCE(md.filename, ''), COALESCE(md.duration_secs, 0), COALESCE(md.is_gif, 0)
		FROM chats c
		LEFT JOIN groups g ON g.jid = c.jid
		LEFT JOIN contacts ct ON ct.jid = c.jid
		LEFT JOIN (
			SELECT m.* FROM messages m
			WHERE m.rowid = (
				SELECT m2.rowid FROM messages m2
				WHERE m2.chat_jid = m.chat_jid
				ORDER BY m2.ts DESC, m2.rowid DESC
				LIMIT 1
			)
		) lm ON lm.chat_jid = c.jid
		LEFT JOIN media md ON md.chat_jid = lm.chat_jid AND md.msg_id = lm.msg_id
		WHERE COALESCE(g.is_parent, 0) = 0
			AND c.jid != 'status@broadcast'
			AND c.jid NOT LIKE '%@newsletter'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Chat
	for rows.Next() {
		var c Chat
		var isGroup, pinned, muted, archived, groupIsParent, fromMe int
		var chatName, groupName, groupLinkedParent string
		var business, full, push, system, pnJID string
		var lastText, lastKind, lastPayload, mediaKind, mediaCaption, mediaFilename string
		var mediaSecs, mediaGIF int
		var lastTS int64
		if err := rows.Scan(
			&c.JID, &isGroup, &chatName, &pinned, &muted, &archived, &c.UnreadCount, &c.LastMessageTS,
			&groupName, &groupIsParent, &groupLinkedParent,
			&business, &full, &push, &system, &pnJID,
			&fromMe, &lastText, &lastKind, &lastPayload, &lastTS,
			&mediaKind, &mediaCaption, &mediaFilename, &mediaSecs, &mediaGIF,
		); err != nil {
			return nil, err
		}
		c.IsGroup = isGroup != 0
		c.Pinned = pinned != 0
		c.Muted = muted != 0
		c.Archived = archived != 0
		c.Name = resolveChatName(chatName, groupName, business, full, push, system, pnJID, c.JID)
		if !c.IsGroup {
			if p, ok := phoneFromJID(pnJID); ok {
				c.Phone = p
			} else if p, ok := phoneFromJID(c.JID); ok {
				c.Phone = p
			}
		}
		c.Preview = buildPreview(previewInput{
			FromMe: fromMe != 0, Kind: lastKind, Text: lastText, Payload: lastPayload,
			MediaKind: mediaKind, MediaCaption: mediaCaption, MediaFilename: mediaFilename,
			MediaSeconds: mediaSecs, MediaIsGIF: mediaGIF != 0,
		})
		if lr, ok := lastReactions[c.JID]; ok && lr.TS > lastTS {
			r := lr
			c.LastReaction = &r
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if a.LastMessageTS != b.LastMessageTS {
			return a.LastMessageTS > b.LastMessageTS
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// latestOwnMessageReactions is, per chat, the newest reaction on a message
// of ours (the only reactions WhatsApp surfaces in the chat list), with the
// target's preview. One query for every chat, so Chats stays a fixed number
// of statements however long the list.
func (s *Store) latestOwnMessageReactions() (map[string]ChatReaction, error) {
	// SQLite's bare columns beside MAX() come from the row holding the max.
	rows, err := s.db.Query(`
		SELECT r.chat_jid, r.reactor_jid, r.emoji, MAX(r.ts),
			COALESCE(tm.text, ''), tm.kind, COALESCE(tm.payload, ''),
			COALESCE(md.kind, ''), COALESCE(md.caption, ''), COALESCE(md.filename, ''), COALESCE(md.duration_secs, 0), COALESCE(md.is_gif, 0)
		FROM reactions r
		JOIN messages tm ON tm.chat_jid = r.chat_jid AND tm.msg_id = r.msg_id AND tm.from_me = 1
		LEFT JOIN media md ON md.chat_jid = tm.chat_jid AND md.msg_id = tm.msg_id
		GROUP BY r.chat_jid
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ChatReaction{}
	for rows.Next() {
		var jid string
		var r ChatReaction
		var in previewInput
		var isGIF int
		if err := rows.Scan(&jid, &r.ReactorJID, &r.Emoji, &r.TS,
			&in.Text, &in.Kind, &in.Payload,
			&in.MediaKind, &in.MediaCaption, &in.MediaFilename, &in.MediaSeconds, &isGIF); err != nil {
			return nil, err
		}
		in.MediaIsGIF = isGIF != 0
		r.TargetPreview = buildPreview(in)
		out[jid] = r
	}
	return out, rows.Err()
}

// resolveChatName implements the reference name-resolution fix: prefer an
// explicit chat name, then the group name, then contact business/full/push/
// system name (in that order), then a "+<number>" fallback derived from a
// DM JID (for a LID-addressed chat, from its known phone-number JID pnJID
// rather than the opaque LID), and finally the raw JID as a last resort.
func resolveChatName(chatName, groupName, business, full, push, system, pnJID, jid string) string {
	for _, n := range []string{chatName, groupName, business, full, push, system} {
		if n != "" {
			return n
		}
	}
	if strings.HasSuffix(jid, "@lid") && pnJID != "" {
		if number, ok := phoneFromJID(pnJID); ok {
			return "+" + number
		}
	}
	if number, ok := phoneFromJID(jid); ok {
		return "+" + number
	}
	return jid
}

// phoneFromJID returns the all-digit user part of a phone-number JID. A LID
// ("123@lid") is numeric too but is not a phone number, so it is rejected.
func phoneFromJID(jid string) (string, bool) {
	at := strings.IndexByte(jid, '@')
	if at <= 0 || jid[at+1:] == "lid" {
		return "", false
	}
	user := jid[:at]
	for _, r := range user {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return user, true
}

// chatKeyedTables are the tables whose rows belong to a chat by chat_jid.
var chatKeyedTables = []string{"messages", "reactions", "poll_votes", "label_chats", "media", "read_receipts"}

// MergeChat files everything recorded under chat from under chat to and
// drops from: its messages, reactions, votes, labels, media and receipts
// move (a row to already has is kept), and the chat rows fold into one
// (newest activity, the unread count of the row active most recently — the
// other's is stale, its reads landed on the live row — pinned/muted/archived
// if either was, a name from either). It reports whether from had anything
// to merge.
func (s *Store) MergeChat(from, to string) (bool, error) {
	if from == "" || to == "" || from == to {
		return false, nil
	}
	// Every DM message creates its chat row, so no row means nothing to
	// move; this runs on the per-message path once a mapping is known.
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM chats WHERE jid = ?`, from).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var moved int64
	for _, table := range chatKeyedTables {
		res, err := tx.Exec(`UPDATE OR IGNORE `+table+` SET chat_jid = ? WHERE chat_jid = ?`, to, from)
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		moved += n
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE chat_jid = ?`, from); err != nil {
			return false, err
		}
	}
	// Renames from outright when to has no row; otherwise the rows fold.
	res, err := tx.Exec(`UPDATE OR IGNORE chats SET jid = ? WHERE jid = ?`, to, from)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	moved += n
	if _, err := tx.Exec(`
		UPDATE chats SET
			name = COALESCE(chats.name, src.name),
			pinned = MAX(chats.pinned, src.pinned),
			muted = MAX(chats.muted, src.muted),
			archived = MAX(chats.archived, src.archived),
			unread_count = CASE WHEN src.last_message_ts > chats.last_message_ts
				THEN src.unread_count ELSE chats.unread_count END,
			last_message_ts = MAX(chats.last_message_ts, src.last_message_ts)
		FROM (SELECT * FROM chats WHERE jid = ?) AS src
		WHERE chats.jid = ?
	`, from, to); err != nil {
		return false, err
	}
	res, err = tx.Exec(`DELETE FROM chats WHERE jid = ?`, from)
	if err != nil {
		return false, err
	}
	n, _ = res.RowsAffected()
	moved += n
	return moved > 0, tx.Commit()
}

// LIDChatJIDs lists the chats filed under a LID rather than a phone number.
func (s *Store) LIDChatJIDs() ([]string, error) {
	rows, err := s.db.Query(`SELECT jid FROM chats WHERE jid LIKE '%@lid'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var jid string
		if err := rows.Scan(&jid); err != nil {
			return nil, err
		}
		out = append(out, jid)
	}
	return out, rows.Err()
}

// RepairUnreadCounts caps every unread count at what the stored thread can
// support: the messages from the other side newer than the account's own
// last message (a reply means everything before it was read). History sync
// wrote counts as of linking that reads addressed to the chat's other JID
// never cleared. Chats with no stored messages are left alone, and so is a
// count of exactly 1: that is what "mark as unread" sets, with no message
// behind it. Returns how many chats changed.
func (s *Store) RepairUnreadCounts() (int64, error) {
	res, err := s.db.Exec(`
		WITH caps AS (
			SELECT c.jid, (
				SELECT COUNT(*) FROM messages m
				WHERE m.chat_jid = c.jid AND m.from_me = 0 AND m.ts > COALESCE(
					(SELECT MAX(o.ts) FROM messages o WHERE o.chat_jid = c.jid AND o.from_me = 1), 0)
			) AS cap
			FROM chats c
			WHERE c.unread_count > 1 AND EXISTS (SELECT 1 FROM messages WHERE chat_jid = c.jid)
		)
		UPDATE chats SET unread_count = caps.cap
		FROM caps WHERE chats.jid = caps.jid AND chats.unread_count > caps.cap
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
