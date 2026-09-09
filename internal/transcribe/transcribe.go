// Package transcribe turns a voice note into text on this computer with
// whisper.cpp: the speech model is fetched once into the cache, ffmpeg
// (already a dependency) resamples the note to the 16 kHz mono WAV the
// engine wants, and whisper-cli does the rest. Nothing leaves the machine
// but the one-time model download.
package transcribe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// ModelName is the whisper.cpp model chatot uses: the multilingual
	// "small" model, 5-bit quantised. Small enough to fetch in a minute,
	// good enough for Portuguese and English voice notes; the tiny/base
	// models garble anything but clear English.
	ModelName = "ggml-small-q5_1.bin"
	// ModelURL is where ModelName is fetched from: the whisper.cpp
	// author's model mirror on Hugging Face.
	ModelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + ModelName
	// ModelSize is ModelName's byte length, for the download prompt; the
	// real length comes from the server once the fetch starts.
	ModelSize int64 = 190085487
	// Engine is the whisper.cpp command-line tool the transcription shells
	// out to.
	Engine = "whisper-cli"
)

var (
	// ErrNoEngine is Transcribe's answer when whisper-cli (or ffmpeg) is
	// not on PATH.
	ErrNoEngine = errors.New("chatot/transcribe: whisper-cli is not installed")
	// ErrNoModel is Transcribe's answer before the model was downloaded.
	ErrNoModel = errors.New("chatot/transcribe: speech model not downloaded")
	// ErrNoSpeech is Transcribe's answer for a note the engine found no
	// words in.
	ErrNoSpeech = errors.New("chatot/transcribe: no speech found")
)

// ModelPath is where the model lives under cacheDir.
func ModelPath(cacheDir string) string {
	return filepath.Join(cacheDir, "whisper", ModelName)
}

// ModelReady reports whether the model has been downloaded in full (a
// fetch in progress writes to a .part file beside it).
func ModelReady(cacheDir string) bool {
	info, err := os.Stat(ModelPath(cacheDir))
	return err == nil && !info.IsDir() && info.Size() > 0
}

// EngineAvailable reports whether the tools Transcribe shells out to are
// installed.
func EngineAvailable() bool {
	_, err := exec.LookPath(Engine)
	if err != nil {
		return false
	}
	_, err = exec.LookPath("ffmpeg")
	return err == nil
}

// DownloadModel fetches the model into cacheDir, reporting progress as
// (bytes so far, total; total is 0 when the server did not say). The file
// appears under ModelPath only once it is complete; a cancelled or failed
// fetch leaves nothing behind. ctx cancels it.
func DownloadModel(ctx context.Context, cacheDir string, progress func(done, total int64)) error {
	return downloadModel(ctx, ModelURL, ModelPath(cacheDir), progress)
}

// downloadModel is DownloadModel with the source pinned, for tests.
func downloadModel(ctx context.Context, src, dst string, progress func(done, total int64)) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: &http.Transport{Proxy: proxyFunc()}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chatot/transcribe: download model: %s", resp.Status)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := &progressWriter{w: f, total: max(resp.ContentLength, 0), report: progress}
	_, err = io.Copy(w, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	w.flush()
	return os.Rename(tmp, dst)
}

// proxyFunc honours CHATOT_PROXY (the app's saved proxy, exported by
// main.go the way the WhatsApp session reads it), else the environment's
// HTTPS_PROXY and friends.
func proxyFunc() func(*http.Request) (*url.URL, error) {
	if p := os.Getenv("CHATOT_PROXY"); p != "" {
		if u, err := url.Parse(p); err == nil {
			return http.ProxyURL(u)
		}
	}
	return http.ProxyFromEnvironment
}

// progressWriter counts bytes through to w and reports them in steps of
// half a percent (or every 512 KiB with no known total), so a callback
// that hops to the GTK main loop is not called per chunk.
type progressWriter struct {
	w        io.Writer
	done     int64
	total    int64
	reported int64
	report   func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	step := p.total / 200
	if step <= 0 {
		step = 512 * 1024
	}
	if p.done-p.reported >= step {
		p.flush()
	}
	return n, err
}

func (p *progressWriter) flush() {
	if p.report != nil && p.done != p.reported {
		p.report(p.done, p.total)
	}
	p.reported = p.done
}

// transcribeTimeout bounds one run: resampling plus the engine, which
// takes well under real time on a modern CPU but has no upper bound on an
// old one.
const transcribeTimeout = 15 * time.Minute

// engineMu runs one transcription at a time: whisper uses every core it is
// given, and a burst of automatic runs would only fight over them.
var engineMu sync.Mutex

// Transcribe returns the words spoken in the audio file at path, in the
// language it detects. It shells out to ffmpeg and whisper-cli, one run at
// a time process-wide, so call it off the main loop.
func Transcribe(ctx context.Context, cacheDir, path string) (string, error) {
	if !EngineAvailable() {
		return "", ErrNoEngine
	}
	model := ModelPath(cacheDir)
	if !ModelReady(cacheDir) {
		return "", ErrNoModel
	}
	engineMu.Lock()
	defer engineMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, transcribeTimeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "chatot-transcribe-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "note.wav")
	// whisper-cli reads WAV/FLAC/MP3/Vorbis itself, but a WhatsApp voice
	// note is Opus in Ogg, so everything goes through ffmpeg to the 16 kHz
	// mono PCM the model was trained on.
	ff := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-y",
		"-i", path, "-vn", "-map_metadata", "-1", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-f", "wav", wav)
	if msg, err := ff.CombinedOutput(); err != nil {
		return "", fmt.Errorf("chatot/transcribe: ffmpeg: %w: %s", err, strings.TrimSpace(string(msg)))
	}

	cmd := exec.CommandContext(ctx, Engine, "-m", model, "-f", wav,
		"-l", "auto", "-t", strconv.Itoa(threads()), "-np", "-nt")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("chatot/transcribe: %s: %w: %s", Engine, err, lastLine(stderr.String()))
	}
	text := parseOutput(stdout.String())
	if text == "" {
		return "", ErrNoSpeech
	}
	return text, nil
}

// threads is how many cores the engine gets: all of them up to eight,
// past which whisper.cpp gains nothing.
func threads() int {
	n := runtime.NumCPU()
	if n > 8 {
		n = 8
	}
	if n < 1 {
		n = 1
	}
	return n
}

// parseOutput joins whisper-cli's segment lines (one per line with -nt)
// into one paragraph, dropping its sound markers and collapsing the
// run-on spaces. A marker is a segment that is nothing but a bracketed or
// parenthesised note ("[BLANK_AUDIO]", "(chimes)", "(música)"); a
// parenthesis inside spoken text stays.
func parseOutput(out string) string {
	var words []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if isSoundMarker(line) {
			continue
		}
		for _, w := range strings.Fields(line) {
			if strings.HasPrefix(w, "[") && strings.HasSuffix(w, "]") {
				continue
			}
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

// isSoundMarker reports whether a whole segment is one of whisper's
// non-speech notes: "(music)", "[BLANK_AUDIO]", "*applause*".
func isSoundMarker(segment string) bool {
	for _, pair := range []struct{ open, close string }{{"(", ")"}, {"[", "]"}, {"*", "*"}} {
		if len(segment) > 1 && strings.HasPrefix(segment, pair.open) && strings.HasSuffix(segment, pair.close) &&
			!strings.Contains(segment[1:len(segment)-1], pair.close) {
			return true
		}
	}
	return false
}

// lastLine is the last non-empty line of s, for an error message.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
