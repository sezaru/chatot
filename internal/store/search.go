package store

import "strings"

// The caller's limit caps the message sub-query; searchChatLimit caps the
// chat-name sub-query. Search blends the two and trims to the caller's limit.
const searchChatLimit = 10

// buildFTSQuery turns raw user input into a safe fts5 MATCH expression.
// Every whitespace-delimited token is individually double-quoted (so
// punctuation, and fts5 operators like AND/OR/NOT/NEAR typed as plain
// words, can never be parsed as query syntax) and joined with a space,
// which fts5 treats as an implicit AND across phrases. The final token
// additionally gets a trailing "*" for prefix-matching ("hel" finds
// "hello") to make as-you-type search feel responsive. Empty input maps to
// "" so callers can skip the query without needing to special-case it.
func buildFTSQuery(user string) string {
	tokens := strings.Fields(user)
	if len(tokens) == 0 {
		return ""
	}
	quoted := make([]string, len(tokens))
	for i, tok := range tokens {
		q := `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
		if i == len(tokens)-1 {
			q += "*"
		}
		quoted[i] = q
	}
	return strings.Join(quoted, " ")
}

// Search looks up query across message text (fts5) and chat display names,
// returning up to limit hits ordered message-relevance-then-recency first,
// chat-name matches last. An empty/whitespace-only query returns no hits.
func (s *Store) Search(query string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 50
	}
	if query == "" {
		return nil, nil
	}

	hits, err := s.searchMessages(query, limit)
	if err != nil {
		return nil, err
	}
	chatHits, err := s.searchChatNames(query, searchChatLimit)
	if err != nil {
		return nil, err
	}
	hits = append(hits, chatHits...)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// searchMessages runs the fts5 MATCH query, joining back to messages for
// chat/ts and to chats/groups/contacts for display-name resolution.
// Ranking is bm25 (best match first) with recency breaking ties.
func (s *Store) searchMessages(query string, limit int) ([]SearchHit, error) {
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT
			m.chat_jid, m.msg_id, m.ts,
			snippet(messages_fts, 0, '[', ']', '…', 10),
			COALESCE(c.name, ''), COALESCE(g.name, ''),
			COALESCE(ct.business_name, ''), COALESCE(ct.full_name, ''), COALESCE(ct.push_name, ''), COALESCE(ct.system_name, '')
		FROM messages_fts
		JOIN messages m ON m.rowid = messages_fts.rowid
		LEFT JOIN chats c ON c.jid = m.chat_jid
		LEFT JOIN groups g ON g.jid = m.chat_jid
		LEFT JOIN contacts ct ON ct.jid = m.chat_jid
		WHERE messages_fts MATCH ?
		ORDER BY bm25(messages_fts), m.ts DESC
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		var chatName, groupName, business, full, push, system string
		if err := rows.Scan(&h.ChatJID, &h.MsgID, &h.TS, &h.Snippet, &chatName, &groupName, &business, &full, &push, &system); err != nil {
			return nil, err
		}
		h.ChatName = resolveChatName(chatName, groupName, business, full, push, system, h.ChatJID)
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchInChat runs the fts5 query scoped to a single chat, ordered oldest
// first (unlike Search's relevance ordering) since the in-chat search bar
// steps through hits in reading order. An empty/whitespace-only query
// returns no hits.
func (s *Store) SearchInChat(chatJID, query string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 200
	}
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT
			m.chat_jid, m.msg_id, m.ts,
			snippet(messages_fts, 0, '[', ']', '…', 10)
		FROM messages_fts
		JOIN messages m ON m.rowid = messages_fts.rowid
		WHERE messages_fts MATCH ? AND m.chat_jid = ?
		ORDER BY m.ts ASC
		LIMIT ?
	`, ftsQuery, chatJID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ChatJID, &h.MsgID, &h.TS, &h.Snippet); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// searchChatNames matches query as a case-insensitive substring against
// each chat's resolved display name. Filtering happens in Go (chat counts
// are small) so resolveChatName's precedence rules don't need reimplementing
// in SQL.
func (s *Store) searchChatNames(query string, limit int) ([]SearchHit, error) {
	rows, err := s.db.Query(`
		SELECT c.jid, c.last_message_ts,
			COALESCE(c.name, ''), COALESCE(g.name, ''),
			COALESCE(ct.business_name, ''), COALESCE(ct.full_name, ''), COALESCE(ct.push_name, ''), COALESCE(ct.system_name, '')
		FROM chats c
		LEFT JOIN groups g ON g.jid = c.jid
		LEFT JOIN contacts ct ON ct.jid = c.jid
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SearchHit
	for rows.Next() {
		var jid string
		var ts int64
		var chatName, groupName, business, full, push, system string
		if err := rows.Scan(&jid, &ts, &chatName, &groupName, &business, &full, &push, &system); err != nil {
			return nil, err
		}
		name := resolveChatName(chatName, groupName, business, full, push, system, jid)
		if !likeMatch(name, query) {
			continue
		}
		out = append(out, SearchHit{ChatJID: jid, ChatName: name, Snippet: name, TS: ts})
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

// likeMatch reports whether name case-insensitively contains query.
func likeMatch(name, query string) bool {
	return strings.Contains(strings.ToLower(name), strings.ToLower(query))
}
