package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirSizeAndClearDir(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "tiles", "z"), 0o700)
	os.WriteFile(filepath.Join(root, "tiles", "z", "a.png"), make([]byte, 300), 0o600)
	os.WriteFile(filepath.Join(root, "b.flac"), make([]byte, 700), 0o600)
	if got := dirSize(root); got != 1000 {
		t.Errorf("dirSize = %d, want 1000", got)
	}
	if got := dirSize(filepath.Join(root, "missing")); got != 0 {
		t.Errorf("dirSize(missing) = %d, want 0", got)
	}
	if err := clearDir(root); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("clearDir left %d entries", len(entries))
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("clearDir removed the root itself: %v", err)
	}
	if err := clearDir(filepath.Join(root, "missing")); err != nil {
		t.Errorf("clearDir(missing) = %v, want nil", err)
	}
}
