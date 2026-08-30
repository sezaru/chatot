// Package media caps the on-disk attachment cache: pure filesystem +
// callback logic, kept free of whatsmeow/store so it's unit-testable
// headless. Mirrors the DMS plugin's wa-prune shell script.
package media

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fileEntry struct {
	path  string
	size  int64
	mtime int64
}

// Evict caps dir at maxBytes, deleting the least-recently-downloaded files
// first (oldest mtime — re-viewing an evicted item re-downloads it with a
// fresh mtime) until the total is at or under the cap. For each deleted
// file, nullLocalPath is called with its path so the caller can clear the
// corresponding DB row (nil is fine if the caller doesn't need this). A
// non-existent dir or maxBytes <= 0 is a no-op.
func Evict(dir string, maxBytes int64, nullLocalPath func(path string) error) error {
	if maxBytes <= 0 {
		return nil
	}

	var files []fileEntry
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, fileEntry{path: path, size: info.Size(), mtime: info.ModTime().UnixNano()})
		total += info.Size()
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if total <= maxBytes {
		return nil
	}

	sort.Slice(files, func(i, j int) bool { return files[i].mtime < files[j].mtime })

	for _, f := range files {
		if total <= maxBytes {
			break
		}
		if err := os.Remove(f.path); err != nil {
			continue
		}
		total -= f.size
		if nullLocalPath != nil {
			if err := nullLocalPath(f.path); err != nil {
				return err
			}
		}
	}
	return nil
}

// RemoveWithinDir deletes path, but only if it resolves inside dir — guards
// a DB-sourced local_path (e.g. from a clear-chat "also delete media")
// against ever unlinking a file outside the attachment cache. A path that
// escapes dir, or a stat failure resolving either side, is silently
// skipped (not an error): the caller asked to clear a cache entry, and a
// path outside the cache was never that entry to begin with.
func RemoveWithinDir(dir, path string) error {
	if path == "" {
		return nil
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
