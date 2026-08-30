package store

// UpsertMedia inserts or updates a message's attachment metadata. Empty
// string fields leave any existing value untouched; Kind always overwrites.
func (s *Store) UpsertMedia(row MediaRow) error {
	_, err := s.db.Exec(`
		INSERT INTO media(chat_jid, msg_id, kind, filename, caption, mime_type, local_path)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))
		ON CONFLICT(chat_jid, msg_id) DO UPDATE SET
			kind = excluded.kind,
			filename = COALESCE(excluded.filename, media.filename),
			caption = COALESCE(excluded.caption, media.caption),
			mime_type = COALESCE(excluded.mime_type, media.mime_type),
			local_path = COALESCE(excluded.local_path, media.local_path)
	`, row.ChatJID, row.MsgID, row.Kind, row.Filename, row.Caption, row.MimeType, row.LocalPath)
	return err
}
