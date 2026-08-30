package store

import "database/sql"

// UpsertMedia inserts or updates a message's attachment metadata. Empty
// string fields and a nil/empty ProtoBlob leave any existing value
// untouched; Kind always overwrites.
func (s *Store) UpsertMedia(row MediaRow) error {
	var blob any
	if len(row.ProtoBlob) > 0 {
		blob = row.ProtoBlob
	}
	_, err := s.db.Exec(`
		INSERT INTO media(chat_jid, msg_id, kind, filename, caption, mime_type, local_path, proto_blob)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?)
		ON CONFLICT(chat_jid, msg_id) DO UPDATE SET
			kind = excluded.kind,
			filename = COALESCE(excluded.filename, media.filename),
			caption = COALESCE(excluded.caption, media.caption),
			mime_type = COALESCE(excluded.mime_type, media.mime_type),
			local_path = COALESCE(excluded.local_path, media.local_path),
			proto_blob = COALESCE(excluded.proto_blob, media.proto_blob)
	`, row.ChatJID, row.MsgID, row.Kind, row.Filename, row.Caption, row.MimeType, row.LocalPath, blob)
	return err
}

// MediaByMsgID looks up a media row by message ID alone (whatsmeow message
// IDs are unique per sending device, not just per chat), for
// Client.DownloadMedia, which is only handed a msgID. ok is false if not
// found.
func (s *Store) MediaByMsgID(msgID string) (row MediaRow, ok bool, err error) {
	r := s.db.QueryRow(`
		SELECT chat_jid, msg_id, kind, COALESCE(filename, ''), COALESCE(caption, ''),
			COALESCE(mime_type, ''), COALESCE(local_path, ''), proto_blob
		FROM media WHERE msg_id = ? LIMIT 1
	`, msgID)
	var blob []byte
	if err := r.Scan(&row.ChatJID, &row.MsgID, &row.Kind, &row.Filename, &row.Caption,
		&row.MimeType, &row.LocalPath, &blob); err != nil {
		if err == sql.ErrNoRows {
			return MediaRow{}, false, nil
		}
		return MediaRow{}, false, err
	}
	row.ProtoBlob = blob
	return row, true, nil
}

// SetMediaLocalPath records where a downloaded attachment was cached to disk.
func (s *Store) SetMediaLocalPath(chatJID, msgID, localPath string) error {
	_, err := s.db.Exec(`UPDATE media SET local_path = ? WHERE chat_jid = ? AND msg_id = ?`, localPath, chatJID, msgID)
	return err
}

// NullMediaLocalPathByPath clears local_path wherever it points at path, so
// the UI re-offers tap-to-load after that cached file is evicted from disk.
func (s *Store) NullMediaLocalPathByPath(localPath string) error {
	_, err := s.db.Exec(`UPDATE media SET local_path = NULL WHERE local_path = ?`, localPath)
	return err
}
