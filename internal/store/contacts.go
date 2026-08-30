package store

// UpsertContact inserts or updates a contact's name fields. Empty fields
// leave any existing value untouched, since contact info (push name from a
// message, business/full/system name from app-state sync) arrives piecemeal.
func (s *Store) UpsertContact(row ContactRow) error {
	_, err := s.db.Exec(`
		INSERT INTO contacts(jid, business_name, full_name, push_name, system_name)
		VALUES (?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))
		ON CONFLICT(jid) DO UPDATE SET
			business_name = COALESCE(excluded.business_name, contacts.business_name),
			full_name = COALESCE(excluded.full_name, contacts.full_name),
			push_name = COALESCE(excluded.push_name, contacts.push_name),
			system_name = COALESCE(excluded.system_name, contacts.system_name)
	`, row.JID, row.BusinessName, row.FullName, row.PushName, row.SystemName)
	return err
}
