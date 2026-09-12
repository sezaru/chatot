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
	"time"
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
	if ModelReady(dir, DefaultModel) {
		t.Fatal("ModelReady on an empty cache")
	}
	path := ModelPath(dir, DefaultModel)
	if filepath.Base(path) != "ggml-small-q5_1.bin" || filepath.Dir(filepath.Dir(path)) != dir {
		t.Fatalf("ModelPath = %q", path)
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path+".part", []byte("half"), 0o644)
	if ModelReady(dir, DefaultModel) {
		t.Fatal("ModelReady with only a .part file")
	}
	os.WriteFile(path, []byte("model"), 0o644)
	if !ModelReady(dir, DefaultModel) {
		t.Fatal("ModelReady false with the model in place")
	}
	// The other model has its own file beside it, so neither download
	// replaces the other and a switch back costs nothing.
	if ModelReady(dir, "turbo") {
		t.Fatal("ModelReady for turbo with only small in place")
	}
	turbo := ModelPath(dir, "turbo")
	if turbo == path || filepath.Dir(turbo) != filepath.Dir(path) || filepath.Base(turbo) != "ggml-large-v3-turbo-q5_0.bin" {
		t.Fatalf("ModelPath(turbo) = %q beside %q", turbo, path)
	}
	os.WriteFile(turbo, []byte("model"), 0o644)
	if !ModelReady(dir, "turbo") || !ModelReady(dir, DefaultModel) {
		t.Fatal("both models in place, but not both ready")
	}
}

func TestModelByKeyFallsBackToTheDefault(t *testing.T) {
	if m := ModelByKey("turbo"); m.Key != "turbo" || m.URL() != modelMirror+m.File {
		t.Fatalf("ModelByKey(turbo) = %+v", m)
	}
	if m := ModelByKey("base"); m.Key != DefaultModel || !m.Recommended {
		t.Fatalf("ModelByKey(base) = %+v, want the default", m)
	}
	if KnownModel("base") || !KnownModel("small") || !KnownModel("turbo") {
		t.Fatal("KnownModel")
	}
	if ModelPath("c", "base") != ModelPath("c", DefaultModel) {
		t.Fatal("ModelPath of an unknown key is not the default model")
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

	dst := filepath.Join(t.TempDir(), "whisper", ModelByKey(DefaultModel).File)
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
	dst := filepath.Join(t.TempDir(), ModelByKey(DefaultModel).File)
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
	dst := filepath.Join(t.TempDir(), ModelByKey(DefaultModel).File)
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
	if _, err := Transcribe(context.Background(), t.TempDir(), DefaultModel, "/nonexistent.ogg"); !errors.Is(err, ErrNoEngine) {
		t.Fatalf("err = %v, want ErrNoEngine", err)
	}
}

func TestDownloadModelRetriesAPassingServerError(t *testing.T) {
	downloadRetryDelay = time.Millisecond
	defer func() { downloadRetryDelay = time.Second }()
	body := []byte("model bytes")
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			http.Error(w, "overloaded", http.StatusServiceUnavailable)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), ModelByKey(DefaultModel).File)
	if err := downloadModel(context.Background(), srv.URL, dst, nil); err != nil {
		t.Fatalf("downloadModel after two 503s: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(body) {
		t.Fatalf("model file = %q", got)
	}
	if hits != 3 {
		t.Fatalf("server hit %d times, want 3", hits)
	}
}

func TestDownloadModelGivesUpAfterTheAttempts(t *testing.T) {
	downloadRetryDelay = time.Millisecond
	defer func() { downloadRetryDelay = time.Second }()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "overloaded", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), ModelByKey(DefaultModel).File)
	if err := downloadModel(context.Background(), srv.URL, dst, nil); err == nil {
		t.Fatal("no error when every attempt got a 503")
	}
	if hits != downloadAttempts {
		t.Fatalf("server hit %d times, want %d", hits, downloadAttempts)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("model file written after 503s")
	}
}

func TestDownloadModelDoesNotRetryARefusal(t *testing.T) {
	downloadRetryDelay = time.Millisecond
	defer func() { downloadRetryDelay = time.Second }()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), ModelByKey(DefaultModel).File)
	if err := downloadModel(context.Background(), srv.URL, dst, nil); err == nil {
		t.Fatal("no error for a 404")
	}
	if hits != 1 {
		t.Fatalf("server hit %d times for a 404, want 1", hits)
	}
}

func TestDownloadModelCancelledDuringTheRetryWait(t *testing.T) {
	downloadRetryDelay = time.Minute
	defer func() { downloadRetryDelay = time.Second }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "overloaded", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	dst := filepath.Join(t.TempDir(), ModelByKey(DefaultModel).File)
	start := time.Now()
	err := downloadModel(ctx, srv.URL, dst, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("cancel did not cut the retry wait short")
	}
}
