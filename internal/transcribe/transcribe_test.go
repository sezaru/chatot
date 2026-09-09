package transcribe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseOutputJoinsSegmentsAndDropsMarkers(t *testing.T) {
	out := " Olá,  tudo bem?\n[BLANK_AUDIO]\n Then  a second line. \n\n[MUSIC]\n"
	if got, want := parseOutput(out), "Olá, tudo bem? Then a second line."; got != want {
		t.Fatalf("parseOutput = %q, want %q", got, want)
	}
	if got := parseOutput("\n[BLANK_AUDIO]\n (chimes)\n*applause*\n"); got != "" {
		t.Fatalf("parseOutput of sound markers = %q, want empty", got)
	}
	if got, want := parseOutput(" Call me (after six) please.\n"), "Call me (after six) please."; got != want {
		t.Fatalf("parseOutput kept parenthesis = %q, want %q", got, want)
	}
}

func TestModelPathAndReady(t *testing.T) {
	dir := t.TempDir()
	if ModelReady(dir) {
		t.Fatal("ModelReady on an empty cache")
	}
	path := ModelPath(dir)
	if filepath.Base(path) != ModelName || filepath.Dir(filepath.Dir(path)) != dir {
		t.Fatalf("ModelPath = %q", path)
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path+".part", []byte("half"), 0o644)
	if ModelReady(dir) {
		t.Fatal("ModelReady with only a .part file")
	}
	os.WriteFile(path, []byte("model"), 0o644)
	if !ModelReady(dir) {
		t.Fatal("ModelReady false with the model in place")
	}
}

func TestDownloadModelWritesFileAndReportsProgress(t *testing.T) {
	body := make([]byte, 300*1024)
	for i := range body {
		body[i] = byte(i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "whisper", ModelName)
	var lastDone, lastTotal int64
	calls := 0
	err := downloadModel(context.Background(), srv.URL, dst, func(done, total int64) {
		calls++
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("downloadModel: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || len(got) != len(body) {
		t.Fatalf("model file: %v, %d bytes", err, len(got))
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left behind")
	}
	if calls == 0 || lastDone != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Fatalf("progress: %d calls, last %d/%d", calls, lastDone, lastTotal)
	}
}

func TestDownloadModelHTTPErrorLeavesNothing(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), ModelName)
	if err := downloadModel(context.Background(), srv.URL, dst, nil); err == nil {
		t.Fatal("no error for a 404")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("model file written after a 404")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left after a 404")
	}
}

func TestDownloadModelTruncatedBodyLeavesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Promise more than is sent, then drop the connection: the copy
		// fails partway through.
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		w.Write(make([]byte, 64*1024))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), ModelName)
	if err := downloadModel(context.Background(), srv.URL, dst, nil); err == nil {
		t.Fatal("no error for a truncated body")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("model file written after a truncated body")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left after a truncated body")
	}
}

func TestTranscribeWithoutEngine(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Transcribe(context.Background(), t.TempDir(), "/nonexistent.ogg"); !errors.Is(err, ErrNoEngine) {
		t.Fatalf("err = %v, want ErrNoEngine", err)
	}
}
