package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBackupWritesAnOpenableCopy(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "chatot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Size() <= 0 {
		t.Errorf("Size = %d, want the file's bytes", s.Size())
	}
	out := filepath.Join(dir, "copy.db")
	if err := s.Backup(out); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	copy, err := Open(out)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	copy.Close()
}
