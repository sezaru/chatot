package store

// SetNewsletterReaction records our reaction on a channel post; an empty
// emoji withdraws it.
func (s *Store) SetNewsletterReaction(newsletterJID, msgID, emoji string) error {
	if emoji == "" {
		_, err := s.db.Exec(`DELETE FROM newsletter_reactions WHERE newsletter_jid = ? AND msg_id = ?`, newsletterJID, msgID)
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO newsletter_reactions(newsletter_jid, msg_id, emoji) VALUES (?, ?, ?)
		ON CONFLICT(newsletter_jid, msg_id) DO UPDATE SET emoji = excluded.emoji
	`, newsletterJID, msgID, emoji)
	return err
}

// NewsletterReactions returns our reactions in a channel, keyed by post ID.
func (s *Store) NewsletterReactions(newsletterJID string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT msg_id, emoji FROM newsletter_reactions WHERE newsletter_jid = ?`, newsletterJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, emoji string
		if err := rows.Scan(&id, &emoji); err != nil {
			return nil, err
		}
		out[id] = emoji
	}
	return out, rows.Err()
}

// ReadReceipt is one reader's read receipt for a message.
type ReadReceipt struct {
	ReaderJID string
	TS        int64
}

// UpsertReadReceipt records that readerJID read msgID in chatJID. A repeat
// keeps the earliest timestamp: the first view is the one that counts.
func (s *Store) UpsertReadReceipt(chatJID, msgID, readerJID string, ts int64) error {
	_, err := s.db.Exec(`
		INSERT INTO read_receipts(chat_jid, msg_id, reader_jid, ts) VALUES (?, ?, ?, ?)
		ON CONFLICT(chat_jid, msg_id, reader_jid) DO UPDATE SET
			ts = CASE WHEN read_receipts.ts = 0 THEN excluded.ts ELSE MIN(read_receipts.ts, excluded.ts) END
	`, chatJID, msgID, readerJID, ts)
	return err
}

// ReadReceipts lists who read msgID in chatJID, earliest reader first.
func (s *Store) ReadReceipts(chatJID, msgID string) ([]ReadReceipt, error) {
	rows, err := s.db.Query(`
		SELECT reader_jid, ts FROM read_receipts WHERE chat_jid = ? AND msg_id = ?
		ORDER BY ts ASC, reader_jid ASC
	`, chatJID, msgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReadReceipt
	for rows.Next() {
		var r ReadReceipt
		if err := rows.Scan(&r.ReaderJID, &r.TS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetStatusMuted files (or unfiles) jid's status updates as muted.
func (s *Store) SetStatusMuted(jid string, muted bool) error {
	if !muted {
		_, err := s.db.Exec(`DELETE FROM status_mutes WHERE jid = ?`, jid)
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO status_mutes(jid) VALUES (?)`, jid)
	return err
}

// MutedStatusPosters lists the posters whose status updates are muted.
func (s *Store) MutedStatusPosters() ([]string, error) {
	rows, err := s.db.Query(`SELECT jid FROM status_mutes ORDER BY jid`)
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

// ReadReceiptsByMsgID lists who read msgID whatever chat the receipt was
// filed under, earliest first. A status view receipt arrives addressed
// from the viewer rather than from status@broadcast, and message IDs are
// unique per sending device, so the ID alone identifies our update.
func (s *Store) ReadReceiptsByMsgID(msgID string) ([]ReadReceipt, error) {
	rows, err := s.db.Query(`
		SELECT reader_jid, MIN(ts) FROM read_receipts WHERE msg_id = ?
		GROUP BY reader_jid ORDER BY MIN(ts) ASC, reader_jid ASC
	`, msgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReadReceipt
	for rows.Next() {
		var r ReadReceipt
		if err := rows.Scan(&r.ReaderJID, &r.TS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReadReceiptMsgIDs lists the messages in chatJID that anyone has read.
func (s *Store) ReadReceiptMsgIDs(chatJID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT msg_id FROM read_receipts WHERE chat_jid = ?`, chatJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}
