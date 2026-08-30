package store

// UpsertGroup inserts or updates a group's metadata. IsParent and
// LinkedParentJID are always overwritten (they're membership-state facts,
// not free-text that arrives piecemeal); Name follows the same
// leave-untouched-if-empty rule as UpsertChat/UpsertContact.
func (s *Store) UpsertGroup(row GroupRow) error {
	_, err := s.db.Exec(`
		INSERT INTO groups(jid, name, is_parent, linked_parent_jid)
		VALUES (?, NULLIF(?, ''), ?, NULLIF(?, ''))
		ON CONFLICT(jid) DO UPDATE SET
			name = COALESCE(excluded.name, groups.name),
			is_parent = excluded.is_parent,
			linked_parent_jid = excluded.linked_parent_jid
	`, row.JID, row.Name, boolToInt(row.IsParent), row.LinkedParentJID)
	return err
}
