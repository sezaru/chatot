package store

import (
	"database/sql"
	"strings"
)

// UpsertContact inserts or updates a contact's name fields. Empty fields
// leave any existing value untouched, since contact info (push name from a
// message, business/full/system name from app-state sync) arrives piecemeal.
func (s *Store) UpsertContact(row ContactRow) error {
	_, err := s.db.Exec(`
		INSERT INTO contacts(jid, business_name, full_name, push_name, system_name, pn_jid)
		VALUES (?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))
		ON CONFLICT(jid) DO UPDATE SET
			business_name = COALESCE(excluded.business_name, contacts.business_name),
			full_name = COALESCE(excluded.full_name, contacts.full_name),
			push_name = COALESCE(excluded.push_name, contacts.push_name),
			system_name = COALESCE(excluded.system_name, contacts.system_name),
			pn_jid = COALESCE(excluded.pn_jid, contacts.pn_jid)
	`, row.JID, row.BusinessName, row.FullName, row.PushName, row.SystemName, row.PNJID)
	return err
}

// ContactName resolves jid's display name: the chat row's name, then the
// contacts table (business, full, push, system name). A LID and its
// phone-number twin are one person, so a name recorded on either row
// answers for both. Empty when nothing is known — never a "+number"
// stand-in, so a caller can tell a real name from a bare number.
func (s *Store) ContactName(jid string) (string, error) {
	jid = nonADJID(jid)
	name, twin, err := s.contactNameRow(jid)
	if err != nil || name != "" {
		return name, err
	}
	if twin == "" {
		// A phone-number JID's twin is the LID row that points back at it.
		err := s.db.QueryRow(`SELECT jid FROM contacts WHERE pn_jid = ? LIMIT 1`, jid).Scan(&twin)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
	}
	if twin == "" || twin == jid {
		return "", nil
	}
	name, _, err = s.contactNameRow(twin)
	return name, err
}

// contactNameRow is one row's best name ("" when it has none) and, for a
// LID row, the phone-number JID it maps to.
func (s *Store) contactNameRow(jid string) (name, pnJID string, err error) {
	var business, full, push, system, chatName string
	err = s.db.QueryRow(`
		SELECT COALESCE(ct.business_name, ''), COALESCE(ct.full_name, ''), COALESCE(ct.push_name, ''),
		       COALESCE(ct.system_name, ''), COALESCE(ct.pn_jid, ''), COALESCE(c.name, '')
		FROM (SELECT ? AS jid) q
		LEFT JOIN contacts ct ON ct.jid = q.jid
		LEFT JOIN chats c ON c.jid = q.jid`, jid).
		Scan(&business, &full, &push, &system, &pnJID, &chatName)
	if err != nil {
		return "", "", err
	}
	for _, n := range []string{chatName, business, full, push, system} {
		if n != "" && !isBareNumber(n) {
			return n, pnJID, nil
		}
	}
	return "", pnJID, nil
}

// isBareNumber reports a "+5548..." / "+55 48 9901-0873" stand-in name
// (a push name someone set to their own number, or a synced chat name
// that is just the number), which is no name at all.
func isBareNumber(s string) bool {
	if len(s) < 2 || s[0] != '+' {
		return false
	}
	digits := 0
	for _, r := range s[1:] {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return false
		}
	}
	return digits > 0
}

// nonADJID drops the device part of an addressed JID ("user:12@server"):
// a message names the exact device it came from, contact rows are keyed by
// the bare identity.
func nonADJID(jid string) string {
	user, server, ok := strings.Cut(jid, "@")
	if !ok {
		return jid
	}
	user, _, _ = strings.Cut(user, ":")
	return user + "@" + server
}

// ContactPNJID is the phone-number JID recorded for a LID contact ("" when
// the mapping isn't known).
func (s *Store) ContactPNJID(jid string) (string, error) {
	var pn sql.NullString
	err := s.db.QueryRow(`SELECT pn_jid FROM contacts WHERE jid = ?`, nonADJID(jid)).Scan(&pn)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return pn.String, err
}
