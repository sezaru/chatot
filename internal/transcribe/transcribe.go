// Package transcribe turns a voice note into text on this computer with
// whisper.cpp: the chosen speech model is fetched once into the cache, ffmpeg
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

// Engine is the whisper.cpp command-line tool the transcription shells
// out to.
const Engine = "whisper-cli"

// Model is one of the whisper.cpp speech models chatot can fetch and run.
// The figures were measured on 40 Brazilian Portuguese voice notes (481 s,
// 939 words) with 8 threads and no GPU: word error rates of 6.2% for Small
// and 4.2% for Large v3 Turbo on clean audio, peak memory of 463 MB and
// 792 MB, and Turbo taking about four times as long. Seconds depend on the
// machine, so the picker says "slower", not how much slower in time. The
// base model was measured too, at 11.4%: nearly double Small's errors, a
// downgrade rather than a lighter trade, so it is not offered.
type Model struct {
	// Key names the model in the settings file: "small" or "turbo".
	Key string
	// Name is what the picker calls it.
	Name string
	// Recommended marks the model a fresh install starts with.
	Recommended bool
	// File is the model file's name, in the cache and on the mirror.
	File string
	// Size is File's byte length, for the download prompt; the real
	// length comes from the server once the fetch starts.
	Size int64
	// PeakMemory is the most a transcription with it takes, in bytes. It
	// is a spike while the engine runs (one note at a time, see engineMu),
	// not something held between notes, and it barely moves with the
	// note's length or the thread count.
	PeakMemory int64
	// Speed and Quality are how it compares with the other models, as the
	// phrases the picker shows; "" says nothing on that count. The spaces
	// are no-break ones so a phrase never wraps in the middle.
	Speed, Quality string
}

// DefaultModel is the key of the model a fresh install uses, and the one
// an install from before there was a choice keeps: the file it already
// downloaded.
const DefaultModel = "small"

// Models lists the models on offer, in the picker's order.
var Models = []Model{
	{Key: "small", Name: "Small", Recommended: true, File: "ggml-small-q5_1.bin",
		Size: 190085487, PeakMemory: 463 << 20, Speed: "fastest"},
	{Key: "turbo", Name: "Large v3 Turbo", File: "ggml-large-v3-turbo-q5_0.bin",
		Size: 574041195, PeakMemory: 792 << 20, Speed: "~4×\u00a0slower", Quality: "~⅓\u00a0fewer\u00a0mistakes"},
}

// modelMirror is where the models are fetched from: the whisper.cpp
// author's mirror on Hugging Face.
const modelMirror = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

// URL is where m is fetched from.
func (m Model) URL() string { return modelMirror + m.File }

// ModelByKey is the model called key, or the default one for a key it
// does not know.
func ModelByKey(key string) Model {
	for _, m := range Models {
		if m.Key == key {
			return m
		}
	}
	return ModelByKey(DefaultModel)
}

// KnownModel reports whether key names one of Models.
func KnownModel(key string) bool {
	for _, m := range Models {
		if m.Key == key {
			return true
		}
	}
	return false
}

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

// ModelPath is where the model called key lives under cacheDir. Each
// model has its own file, so switching between them never throws the
// other one away.
func ModelPath(cacheDir, key string) string {
	return filepath.Join(cacheDir, "whisper", ModelByKey(key).File)
}

// ModelReady reports whether the model called key has been downloaded in
// full (a fetch in progress writes to a .part file beside it).
func ModelReady(cacheDir, key string) bool {
	info, err := os.Stat(ModelPath(cacheDir, key))
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

// DownloadModel fetches the model called key into cacheDir, reporting
// progress as (bytes so far, total; total is 0 when the server did not
// say). The file appears under ModelPath only once it is complete; a
// cancelled or failed fetch leaves nothing behind. ctx cancels it.
func DownloadModel(ctx context.Context, cacheDir, key string, progress func(done, total int64)) error {
	return downloadModel(ctx, ModelByKey(key).URL(), ModelPath(cacheDir, key), progress)
}

// downloadAttempts is how many times a fetch is tried before its error is
// reported, and downloadRetryDelay the wait before the second try, doubled
// for each after it. Hugging Face answers 503 now and then for a URL that
// works a moment later, and a first-run download should not fail on that.
const downloadAttempts = 4

var downloadRetryDelay = time.Second

// downloadModel is DownloadModel with the source pinned, for tests. A
// fetch that fails on the server's or the network's side is tried again
// after a short wait; one the server refused outright (a 404) is not.
func downloadModel(ctx context.Context, src, dst string, progress func(done, total int64)) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	delay := downloadRetryDelay
	for attempt := 1; ; attempt++ {
		again, err := fetchModel(ctx, src, dst, progress)
		if err == nil || !again || attempt == downloadAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// fetchModel is one attempt at downloadModel: the file lands at dst, or
// nothing is left behind. again says whether the failure was the passing
// kind (the connection, a 5xx, a body cut short) that another try may get
// past.
func fetchModel(ctx context.Context, src, dst string, progress func(done, total int64)) (again bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{Transport: &http.Transport{Proxy: proxyFunc()}}
	resp, err := client.Do(req)
	if err != nil {
		return ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		again := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return again, fmt.Errorf("chatot/transcribe: download model: %s", resp.Status)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return false, err
	}
	w := &progressWriter{w: f, total: max(resp.ContentLength, 0), report: progress}
	_, err = io.Copy(w, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return ctx.Err() == nil, err
	}
	w.flush()
	return false, os.Rename(tmp, dst)
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
// language it detects, using the model called modelKey from cacheDir. It
// shells out to ffmpeg and whisper-cli, one run at a time process-wide, so
// call it off the main loop.
func Transcribe(ctx context.Context, cacheDir, modelKey, path string) (string, error) {
	if !EngineAvailable() {
		return "", ErrNoEngine
	}
	model := ModelPath(cacheDir, modelKey)
	if !ModelReady(cacheDir, modelKey) {
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
