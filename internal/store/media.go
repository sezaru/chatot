package store

import "database/sql"

// UpsertMedia inserts or updates a message's attachment metadata. Empty
// string fields and a nil/empty ProtoBlob/Thumbnail leave any existing value
// untouched; Kind always overwrites.
func (s *Store) UpsertMedia(row MediaRow) error {
	var blob, thumb any
	if len(row.ProtoBlob) > 0 {
		blob = row.ProtoBlob
	}
	if len(row.Thumbnail) > 0 {
		thumb = row.Thumbnail
	}
	_, err := s.db.Exec(`
		INSERT INTO media(chat_jid, msg_id, kind, filename, caption, mime_type, local_path, proto_blob, thumbnail, is_gif, view_once)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)
		ON CONFLICT(chat_jid, msg_id) DO UPDATE SET
			kind = excluded.kind,
			filename = COALESCE(excluded.filename, media.filename),
			caption = COALESCE(excluded.caption, media.caption),
			mime_type = COALESCE(excluded.mime_type, media.mime_type),
			local_path = COALESCE(excluded.local_path, media.local_path),
			proto_blob = COALESCE(excluded.proto_blob, media.proto_blob),
			thumbnail = COALESCE(excluded.thumbnail, media.thumbnail),
			is_gif = excluded.is_gif,
			view_once = excluded.view_once
	`, row.ChatJID, row.MsgID, row.Kind, row.Filename, row.Caption, row.MimeType, row.LocalPath, blob, thumb, boolToInt(row.IsGif), boolToInt(row.ViewOnce))
	return err
}

// MediaByMsgID looks up a media row by message ID alone (whatsmeow message
// IDs are unique per sending device, not just per chat), for
// Client.DownloadMedia, which is only handed a msgID. ok is false if not
// found.
func (s *Store) MediaByMsgID(msgID string) (row MediaRow, ok bool, err error) {
	r := s.db.QueryRow(`
		SELECT chat_jid, msg_id, kind, COALESCE(filename, ''), COALESCE(caption, ''),
			COALESCE(mime_type, ''), COALESCE(local_path, ''), proto_blob, thumbnail, is_gif, view_once, viewed
		FROM media WHERE msg_id = ? LIMIT 1
	`, msgID)
	var blob, thumb []byte
	var isGif, viewOnce, viewed int
	if err := r.Scan(&row.ChatJID, &row.MsgID, &row.Kind, &row.Filename, &row.Caption,
		&row.MimeType, &row.LocalPath, &blob, &thumb, &isGif, &viewOnce, &viewed); err != nil {
		if err == sql.ErrNoRows {
			return MediaRow{}, false, nil
		}
		return MediaRow{}, false, err
	}
	row.ProtoBlob = blob
	row.Thumbnail = thumb
	row.IsGif = isGif != 0
	row.ViewOnce = viewOnce != 0
	row.Viewed = viewed != 0
	return row, true, nil
}

// SetMediaViewed marks a view-once attachment as opened; once set it's
// permanent — the UI never re-offers it for opening.
func (s *Store) SetMediaViewed(chatJID, msgID string) error {
	_, err := s.db.Exec(`UPDATE media SET viewed = 1 WHERE chat_jid = ? AND msg_id = ?`, chatJID, msgID)
	return err
}

// SetMediaProtoBlob overwrites a media row's proto descriptor, used after a
// successful media-retry decrypt refreshes the direct download path.
func (s *Store) SetMediaProtoBlob(chatJID, msgID string, blob []byte) error {
	_, err := s.db.Exec(`UPDATE media SET proto_blob = ? WHERE chat_jid = ? AND msg_id = ?`, blob, chatJID, msgID)
	return err
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
