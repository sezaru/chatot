package store

import "database/sql"

// UpsertGroup inserts or updates a group's metadata. IsParent and
// LinkedParentJID are always overwritten (they're membership-state facts,
// not free-text that arrives piecemeal); Name and Topic follow the same
// leave-untouched-if-empty rule as UpsertChat/UpsertContact.
func (s *Store) UpsertGroup(row GroupRow) error {
	_, err := s.db.Exec(`
		INSERT INTO groups(jid, name, topic, is_parent, linked_parent_jid)
		VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''))
		ON CONFLICT(jid) DO UPDATE SET
			name = COALESCE(excluded.name, groups.name),
			topic = COALESCE(excluded.topic, groups.topic),
			is_parent = excluded.is_parent,
			linked_parent_jid = excluded.linked_parent_jid
	`, row.JID, row.Name, row.Topic, boolToInt(row.IsParent), row.LinkedParentJID)
	return err
}

// GroupMeta returns a group's name and topic.
func (s *Store) GroupMeta(jid string) (name, topic string, err error) {
	var n, t sql.NullString
	err = s.db.QueryRow(`SELECT name, topic FROM groups WHERE jid = ?`, jid).Scan(&n, &t)
	if err != nil {
		return "", "", err
	}
	return n.String, t.String, nil
}

// SetGroupParticipants replaces groupJID's participant set with parts, in a
// single transaction (existing rows for the group are deleted first, so a
// re-fetch after a membership change never leaves stale rows behind).
func (s *Store) SetGroupParticipants(groupJID string, parts []GroupParticipant) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM group_participants WHERE group_jid = ?`, groupJID); err != nil {
		return err
	}
	for _, p := range parts {
		if _, err := tx.Exec(`
			INSERT INTO group_participants(group_jid, participant_jid, is_admin, is_super_admin)
			VALUES (?, ?, ?, ?)
		`, groupJID, p.JID, boolToInt(p.IsAdmin), boolToInt(p.IsSuperAdmin)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GroupParticipants returns groupJID's current membership.
func (s *Store) GroupParticipants(groupJID string) ([]GroupParticipant, error) {
	rows, err := s.db.Query(`
		SELECT participant_jid, is_admin, is_super_admin
		FROM group_participants WHERE group_jid = ?
	`, groupJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupParticipant
	for rows.Next() {
		var p GroupParticipant
		var isAdmin, isSuperAdmin int
		if err := rows.Scan(&p.JID, &isAdmin, &isSuperAdmin); err != nil {
			return nil, err
		}
		p.IsAdmin = isAdmin != 0
		p.IsSuperAdmin = isSuperAdmin != 0
		out = append(out, p)
	}
	return out, rows.Err()
}
