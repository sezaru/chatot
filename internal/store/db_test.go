package store

import (
	"context"
	"path/filepath"
	"testing"
)

// Every connection the pool opens gets the store's settings, not just the
// first one.
func TestOpenSettingsReachEveryConnection(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "chatot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	for i := range 2 {
		// Held open, so the second Conn is a second connection.
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		var mode string
		var sync, busy, fk int
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&sync); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" || sync != 1 || busy != 10000 || fk != 1 {
			t.Errorf("connection %d: journal_mode=%s synchronous=%d busy_timeout=%d foreign_keys=%d, want wal 1 10000 1", i, mode, sync, busy, fk)
		}
	}
}
