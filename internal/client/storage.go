package client

import "context"

// StorageInfo is the optional interface the Preferences window's Storage
// group uses: where downloaded media lives, how big the message database
// is, and a consistent copy of it. The account manager forwards to the
// active account; the fake has none of it.
type StorageInfo interface {
	// MediaDir is the directory downloaded attachments are saved to.
	MediaDir() string
	// DatabaseSize is the message database's bytes on disk.
	DatabaseSize() int64
	// BackupDatabase writes a consistent copy of the message database to
	// path, which must not exist yet.
	BackupDatabase(ctx context.Context, path string) error
}

func (w *Whatsmeow) MediaDir() string    { return w.mediaDir }
func (w *Whatsmeow) DatabaseSize() int64 { return w.store.Size() }
func (w *Whatsmeow) BackupDatabase(ctx context.Context, path string) error {
	return w.store.Backup(path)
}

// storage returns the active account's StorageInfo, or nil when the active
// client has none.
func (m *AccountManager) storage() StorageInfo {
	s, _ := m.active().(StorageInfo)
	return s
}

func (m *AccountManager) MediaDir() string {
	if s := m.storage(); s != nil {
		return s.MediaDir()
	}
	return ""
}

func (m *AccountManager) DatabaseSize() int64 {
	if s := m.storage(); s != nil {
		return s.DatabaseSize()
	}
	return 0
}

func (m *AccountManager) BackupDatabase(ctx context.Context, path string) error {
	if s := m.storage(); s != nil {
		return s.BackupDatabase(ctx, path)
	}
	return errNoStorage
}

var errNoStorage = errorString("this account keeps no database")

type errorString string

func (e errorString) Error() string { return string(e) }
