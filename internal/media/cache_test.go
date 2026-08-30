package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFixture(t *testing.T, dir, name string, size int, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	mtime := time.Now().Add(-age)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
	return path
}

func TestEvictOldestFirst(t *testing.T) {
	dir := t.TempDir()
	oldest := writeFixture(t, dir, "oldest.jpg", 100, 3*time.Hour)
	middle := writeFixture(t, dir, "middle.jpg", 100, 2*time.Hour)
	newest := writeFixture(t, dir, "newest.jpg", 100, 1*time.Hour)

	var nulled []string
	err := Evict(dir, 150, func(path string) error {
		nulled = append(nulled, path)
		return nil
	})
	if err != nil {
		t.Fatalf("Evict: %v", err)
	}

	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Errorf("oldest file should have been deleted, stat err = %v", err)
	}
	if _, err := os.Stat(middle); !os.IsNotExist(err) {
		t.Errorf("middle file should have been deleted, stat err = %v", err)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Errorf("newest file should survive, stat err = %v", err)
	}

	if len(nulled) != 2 || nulled[0] != oldest || nulled[1] != middle {
		t.Errorf("nulled = %v, want [%s %s]", nulled, oldest, middle)
	}
}

func TestEvictUnderCapNoop(t *testing.T) {
	dir := t.TempDir()
	kept := writeFixture(t, dir, "a.jpg", 100, time.Hour)

	called := false
	if err := Evict(dir, 1000, func(string) error { called = true; return nil }); err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if called {
		t.Error("nullLocalPath should not be called when under cap")
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("file should survive under-cap eviction, stat err = %v", err)
	}
}

func TestEvictMissingDirNoop(t *testing.T) {
	if err := Evict(filepath.Join(t.TempDir(), "does-not-exist"), 100, nil); err != nil {
		t.Fatalf("Evict on missing dir: %v", err)
	}
}

func TestEvictZeroCapNoop(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.jpg", 100, time.Hour)
	if err := Evict(dir, 0, func(string) error { t.Fatal("should not be called"); return nil }); err != nil {
		t.Fatalf("Evict: %v", err)
	}
}

func TestRemoveWithinDirDeletesFileInCacheDir(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "cached.jpg", 10, time.Minute)

	if err := RemoveWithinDir(dir, path); err != nil {
		t.Fatalf("RemoveWithinDir: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should have been deleted, stat err = %v", err)
	}
}

func TestRemoveWithinDirRefusesPathOutsideDir(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	path := writeFixture(t, outside, "secret.jpg", 10, time.Minute)

	if err := RemoveWithinDir(dir, path); err != nil {
		t.Fatalf("RemoveWithinDir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file outside dir should survive, stat err = %v", err)
	}
}

func TestRemoveWithinDirRefusesTraversalEscape(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sibling := writeFixture(t, dir, "sibling.jpg", 10, time.Minute)
	escaping := filepath.Join(cacheDir, "..", "sibling.jpg")

	if err := RemoveWithinDir(cacheDir, escaping); err != nil {
		t.Fatalf("RemoveWithinDir: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("file reached via traversal should survive, stat err = %v", err)
	}
}

func TestRemoveWithinDirEmptyPathNoop(t *testing.T) {
	if err := RemoveWithinDir(t.TempDir(), ""); err != nil {
		t.Fatalf("RemoveWithinDir with empty path: %v", err)
	}
}

func TestRemoveWithinDirMissingFileNoop(t *testing.T) {
	dir := t.TempDir()
	if err := RemoveWithinDir(dir, filepath.Join(dir, "gone.jpg")); err != nil {
		t.Fatalf("RemoveWithinDir on missing file: %v", err)
	}
}
