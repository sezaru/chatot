package store

import "database/sql"

// Meta reads a bookkeeping value; "" when unset.
func (s *Store) Meta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetMeta writes a bookkeeping value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// MessagePayload reads a rich message's kind and JSON body ("" when the
// row is a plain text message or doesn't exist).
func (s *Store) MessagePayload(chatJID, msgID string) (kind, payload string, err error) {
	err = s.db.QueryRow(`SELECT COALESCE(kind, ''), COALESCE(payload, '') FROM messages WHERE chat_jid = ? AND msg_id = ?`,
		chatJID, msgID).Scan(&kind, &payload)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return kind, payload, err
}

// SetMessagePayload replaces a rich message's JSON body in place (the kind
// is untouched), for state that changes after the send, such as a live
// location ending.
func (s *Store) SetMessagePayload(chatJID, msgID, payload string) error {
	_, err := s.db.Exec(`UPDATE messages SET payload = ? WHERE chat_jid = ? AND msg_id = ?`, payload, chatJID, msgID)
	return err
}
