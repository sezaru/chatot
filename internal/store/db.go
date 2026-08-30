package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

// Store wraps chatot's local sqlite message/chat database.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the store at path and applies the schema.
// path may be ":memory:" for an ephemeral, single-connection database (used
// by tests).
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = fmt.Sprintf("file:%s?_foreign_keys=on", path)
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("chatot/store: open: %w", err)
	}
	if path == ":memory:" {
		// A bare ":memory:" DSN gets a fresh empty database per connection;
		// pin the pool to one connection so the schema/data survive.
		db.SetMaxOpenConns(1)
	}
	pragmas := "PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"
	if path == ":memory:" {
		pragmas = "PRAGMA foreign_keys=ON;" // WAL is a no-op (and noisy) on :memory:
	}
	if _, err := db.Exec(pragmas); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: pragmas: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
