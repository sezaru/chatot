package store

// UpsertLabel inserts or updates a label's name/color/deleted flag.
func (s *Store) UpsertLabel(id, name string, color int, deleted, predefined bool) error {
	_, err := s.db.Exec(`
		INSERT INTO labels(label_id, name, color, deleted, predefined)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(label_id) DO UPDATE SET
			name = excluded.name,
			color = excluded.color,
			deleted = excluded.deleted,
			predefined = excluded.predefined
	`, id, name, color, boolToInt(deleted), boolToInt(predefined))
	return err
}

// Labels returns the user's own non-deleted labels, ordered by numeric id
// then id. WhatsApp's predefined lists (Unread, Favorites, Groups,
// Communities) are synced as labels too but are not the user's to filter
// by here; the chip row has its own fixed filters for them.
func (s *Store) Labels() ([]Label, error) {
	rows, err := s.db.Query(`
		SELECT label_id, name, color FROM labels
		WHERE deleted = 0 AND predefined = 0
		ORDER BY CAST(label_id AS INTEGER), label_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Label
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetChatLabel associates (labeled) or disassociates a chat with a label.
func (s *Store) SetChatLabel(labelID, chatJID string, labeled bool) error {
	if labeled {
		_, err := s.db.Exec(`
			INSERT OR IGNORE INTO label_chats(label_id, chat_jid) VALUES (?, ?)
		`, labelID, chatJID)
		return err
	}
	_, err := s.db.Exec(`
		DELETE FROM label_chats WHERE label_id = ? AND chat_jid = ?
	`, labelID, chatJID)
	return err
}

// LabelsForChat returns the ids of labels currently associated with chatJID.
func (s *Store) LabelsForChat(chatJID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT label_id FROM label_chats WHERE chat_jid = ?
	`, chatJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

// ChatsForLabel returns the JIDs of chats currently associated with labelID.
func (s *Store) ChatsForLabel(labelID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT chat_jid FROM label_chats WHERE label_id = ?
	`, labelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

// LabelIDs returns every label id ever stored, deleted and predefined ones
// included: a new label must not reuse any of them.
func (s *Store) LabelIDs() ([]string, error) {
	rows, err := s.db.Query(`SELECT label_id FROM labels`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}
