package store

import "database/sql"

// StickerRow is one entry of the sticker library (see schema.sql).
type StickerRow struct {
	Key          string
	Path         string
	FromWhatsApp bool
	Hidden       bool
	AddedTS      int64
	UsedTS       int64
}

// UpsertSticker adds row to the library or refreshes an existing entry's
// path, origin and use time; a hidden entry stays hidden and keeps its
// added time.
func (s *Store) UpsertSticker(row StickerRow) error {
	_, err := s.db.Exec(`
		INSERT INTO stickers(key, path, from_whatsapp, hidden, added_ts, used_ts)
		VALUES (?, ?, ?, 0, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			path = excluded.path,
			from_whatsapp = excluded.from_whatsapp,
			used_ts = MAX(stickers.used_ts, excluded.used_ts)
	`, row.Key, row.Path, boolToInt(row.FromWhatsApp), row.AddedTS, row.UsedTS)
	return err
}

// Sticker looks up one library entry by key, hidden or not.
func (s *Store) Sticker(key string) (StickerRow, bool, error) {
	row := s.db.QueryRow(`SELECT key, path, from_whatsapp, hidden, added_ts, used_ts FROM stickers WHERE key = ?`, key)
	st, err := scanSticker(row)
	if err == sql.ErrNoRows {
		return StickerRow{}, false, nil
	}
	return st, err == nil, err
}

// StickerByPath looks up the visible library entry whose file is path.
func (s *Store) StickerByPath(path string) (StickerRow, bool, error) {
	row := s.db.QueryRow(`SELECT key, path, from_whatsapp, hidden, added_ts, used_ts FROM stickers WHERE path = ? AND hidden = 0`, path)
	st, err := scanSticker(row)
	if err == sql.ErrNoRows {
		return StickerRow{}, false, nil
	}
	return st, err == nil, err
}

// Stickers lists the visible library, most recently used first, then
// newest added.
func (s *Store) Stickers() ([]StickerRow, error) {
	rows, err := s.db.Query(`
		SELECT key, path, from_whatsapp, hidden, added_ts, used_ts FROM stickers
		WHERE hidden = 0 AND path != ''
		ORDER BY used_ts DESC, added_ts DESC, key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StickerRow
	for rows.Next() {
		st, err := scanSticker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// TouchSticker records a use of the sticker at path now (ts), so it moves
// to the front of the library. No-op when path is not in it.
func (s *Store) TouchSticker(path string, ts int64) error {
	_, err := s.db.Exec(`UPDATE stickers SET used_ts = MAX(used_ts, ?) WHERE path = ?`, ts, path)
	return err
}

// RemoveSticker takes key out of the library and returns the file it
// pointed at. A WhatsApp favourite is hidden rather than deleted (see
// schema.sql); a local file's row goes away.
func (s *Store) RemoveSticker(key string) (path string, err error) {
	st, ok, err := s.Sticker(key)
	if err != nil || !ok {
		return "", err
	}
	if st.FromWhatsApp {
		_, err = s.db.Exec(`UPDATE stickers SET hidden = 1, path = '' WHERE key = ?`, key)
	} else {
		_, err = s.db.Exec(`DELETE FROM stickers WHERE key = ?`, key)
	}
	return st.Path, err
}

type rowScanner interface{ Scan(dest ...any) error }

func scanSticker(r rowScanner) (StickerRow, error) {
	var st StickerRow
	var fromWA, hidden int
	if err := r.Scan(&st.Key, &st.Path, &fromWA, &hidden, &st.AddedTS, &st.UsedTS); err != nil {
		return StickerRow{}, err
	}
	st.FromWhatsApp = fromWA != 0
	st.Hidden = hidden != 0
	return st, nil
}
