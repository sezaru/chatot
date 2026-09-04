package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterRestartsPastTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log", "chatot.log")
	w, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := 0; i < 5; i++ {
		if _, err := w.Write([]byte("0123456789\n")); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// 11 bytes per line, cap 20: two lines fit, the third restarts the file.
	if got := strings.Count(string(data), "\n"); got != 1 {
		t.Errorf("file holds %d lines (%q), want 1 after restart", got, data)
	}
}

func TestOpenAppendsToAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatot.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := Open(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("new\n"))
	w.Close()
	data, _ := os.ReadFile(path)
	if string(data) != "old\nnew\n" {
		t.Errorf("file = %q, want old then new", data)
	}
}
