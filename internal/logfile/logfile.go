// Package logfile is the capped log file the app mirrors its log output to,
// so a user can hand over "the log" from Preferences without the file ever
// growing without bound (an earlier uncapped log filled a disk).
package logfile

import (
	"os"
	"path/filepath"
	"sync"
)

// Writer appends to a file and starts it over once it passes Limit bytes.
// Cheap and lossy by design: the tail of a session is what a bug report
// needs, and a rotation that keeps the previous file would double the cap.
type Writer struct {
	mu    sync.Mutex
	path  string
	limit int64
	size  int64
	f     *os.File
}

// Open creates dir/name (truncating nothing) and returns a Writer capped at
// limit bytes.
func Open(path string, limit int64) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Writer{path: path, limit: limit, size: st.Size(), f: f}, nil
}

// Path is the file being written.
func (w *Writer) Path() string { return w.path }

// Write appends p, restarting the file first if that would pass the cap.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.limit && w.size > 0 {
		if err := w.f.Truncate(0); err != nil {
			return 0, err
		}
		if _, err := w.f.Seek(0, 0); err != nil {
			return 0, err
		}
		w.size = 0
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// Close closes the file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}
