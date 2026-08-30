package store

import (
	"sort"
	"strings"
)

// UpsertChat inserts or updates a chat row. An empty Name leaves any
// existing name untouched (chat metadata often arrives piecemeal).
func (s *Store) UpsertChat(row ChatRow) error {
	_, err := s.db.Exec(`
		INSERT INTO chats(jid, is_group, name, pinned, muted, unread_count, last_message_ts)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			is_group = excluded.is_group,
			name = COALESCE(excluded.name, chats.name),
			pinned = excluded.pinned,
			muted = excluded.muted,
			unread_count = excluded.unread_count,
			last_message_ts = excluded.last_message_ts
	`, row.JID, boolToInt(row.IsGroup), row.Name, boolToInt(row.Pinned), boolToInt(row.Muted), row.UnreadCount, row.LastMessageTS)
	return err
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

// Chats returns the chat list: name-resolved, ordered, preview-populated,
// and filtered to DMs plus non-parent/non-linked groups. See the design doc
// ("Store: name resolution / ordering / preview") for the reference rules.
func (s *Store) Chats(limit int) ([]Chat, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT
			c.jid, c.is_group, COALESCE(c.name, ''), c.pinned, c.muted, c.unread_count, c.last_message_ts,
			COALESCE(g.name, ''), COALESCE(g.is_parent, 0), COALESCE(g.linked_parent_jid, ''),
			COALESCE(ct.business_name, ''), COALESCE(ct.full_name, ''), COALESCE(ct.push_name, ''), COALESCE(ct.system_name, ''),
			COALESCE(lm.from_me, 0), COALESCE(lm.text, ''),
			COALESCE(md.kind, ''), COALESCE(md.caption, ''), COALESCE(md.filename, '')
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
		WHERE COALESCE(g.is_parent, 0) = 0 AND COALESCE(g.linked_parent_jid, '') = ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Chat
	for rows.Next() {
		var c Chat
		var isGroup, pinned, muted, groupIsParent, fromMe int
		var chatName, groupName, groupLinkedParent string
		var business, full, push, system string
		var lastText, mediaKind, mediaCaption, mediaFilename string
		if err := rows.Scan(
			&c.JID, &isGroup, &chatName, &pinned, &muted, &c.UnreadCount, &c.LastMessageTS,
			&groupName, &groupIsParent, &groupLinkedParent,
			&business, &full, &push, &system,
			&fromMe, &lastText,
			&mediaKind, &mediaCaption, &mediaFilename,
		); err != nil {
			return nil, err
		}
		c.IsGroup = isGroup != 0
		c.Pinned = pinned != 0
		c.Muted = muted != 0
		c.Name = resolveChatName(chatName, groupName, business, full, push, system, c.JID)
		c.Preview = buildPreview(fromMe != 0, lastText, mediaKind, mediaCaption, mediaFilename)
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
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// resolveChatName implements the reference name-resolution fix: prefer an
// explicit chat name, then the group name, then contact business/full/push/
// system name (in that order), then a "+<number>" fallback derived from a
// DM JID, and finally the raw JID as a last resort.
func resolveChatName(chatName, groupName, business, full, push, system, jid string) string {
	for _, n := range []string{chatName, groupName, business, full, push, system} {
		if n != "" {
			return n
		}
	}
	if number, ok := phoneFromJID(jid); ok {
		return "+" + number
	}
	return jid
}

func phoneFromJID(jid string) (string, bool) {
	at := strings.IndexByte(jid, '@')
	if at <= 0 {
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

// buildPreview implements the reference preview fix: media messages show
// their caption, else filename, else a "[kind]" placeholder; text messages
// show their text; either way a from-me message is prefixed.
func buildPreview(fromMe bool, text, mediaKind, mediaCaption, mediaFilename string) string {
	body := text
	if mediaKind != "" {
		switch {
		case mediaCaption != "":
			body = mediaCaption
		case mediaFilename != "":
			body = mediaFilename
		default:
			body = "[" + mediaKind + "]"
		}
	}
	if fromMe && body != "" {
		return "You: " + body
	}
	return body
}
