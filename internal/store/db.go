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
	if err := migrateAddColumn(db, "media", "proto_blob", "BLOB"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "messages", "kind", "TEXT NOT NULL DEFAULT ''"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "messages", "payload", "TEXT"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "messages", "edited", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "messages", "deleted", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "messages", "status", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "chats", "archived", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "messages", "starred", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "groups", "topic", "TEXT"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "media", "thumbnail", "BLOB"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "media", "is_gif", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "messages", "forwarded", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "media", "view_once", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := migrateAddColumn(db, "media", "viewed", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: migrate: %w", err)
	}
	if err := backfillFTS(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("chatot/store: fts backfill: %w", err)
	}
	return &Store{db: db}, nil
}

// backfillFTS indexes message rows written before messages_fts (or its
// sync triggers) existed. CREATE VIRTUAL TABLE/TRIGGER IF NOT EXISTS only
// take effect for writes made after they're created, so a dev database
// opened for the first time post-upgrade needs its past rows indexed once.
// Comparing row counts keeps repeat calls a cheap no-op pair of COUNT(*)s.
func backfillFTS(db *sql.DB) error {
	var msgCount, ftsCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgCount); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages_fts`).Scan(&ftsCount); err != nil {
		return err
	}
	if ftsCount >= msgCount {
		return nil
	}
	_, err := db.Exec(`INSERT INTO messages_fts(messages_fts) VALUES ('rebuild')`)
	return err
}

// migrateAddColumn adds column to table if it isn't already there. CREATE
// TABLE IF NOT EXISTS in schema.sql only covers fresh databases; existing
// dev databases need this to pick up new columns.
func migrateAddColumn(db *sql.DB, table, column, ddlType string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddlType))
	return err
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

// scanIDs collects a single TEXT column from rows into a slice of strings.
func scanIDs(rows *sql.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
