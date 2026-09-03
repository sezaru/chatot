// Package audio captures voice notes from the default microphone via an
// ffmpeg subprocess, encoding straight to mono ogg/opus.
package audio

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// recordArgs builds the ffmpeg invocation for capturing the default
// PulseAudio source to a mono opus/ogg file at outPath. Pure so it's
// unit-testable without spawning ffmpeg.
func recordArgs(outPath string) []string {
	return []string{
		"-y",
		"-f", "pulse", "-i", "default",
		"-ac", "1",
		// Level metering for the composer's trace: an RMS reading every
		// 50 ms (2400 samples at 48 kHz), printed to stderr as
		// "lavfi.astats.Overall.RMS_level=-23.4" lines (see parseLevelLine).
		"-af", levelFilter,
		"-c:a", "libopus", "-application", "voip",
		outPath,
	}
}

// levelFilter is the ffmpeg audio filter chain that measures the input
// while it is being encoded; the audio itself passes through unchanged.
const levelFilter = "asetnsamples=n=2400,astats=metadata=1:reset=1:measure_perchannel=none:measure_overall=RMS_level,ametadata=mode=print:key=lavfi.astats.Overall.RMS_level"

// levelKey is the metadata line ametadata prints for the overall RMS level.
const levelKey = "lavfi.astats.Overall.RMS_level="

// levelFloorDB is the RMS level (dBFS) that reads as silence on the
// composer's trace; anything from there up to 0 dBFS maps onto 0..1.
const levelFloorDB = -50.0

// parseLevelLine reads one line of ffmpeg's stderr and, if it carries an
// RMS level, returns it mapped to a 0..1 meter value. Lines that are not
// level readings (ffmpeg's own progress chatter) report ok=false.
func parseLevelLine(line string) (level float64, ok bool) {
	i := strings.Index(line, levelKey)
	if i < 0 {
		return 0, false
	}
	v := strings.TrimSpace(line[i+len(levelKey):])
	db, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(db) {
		// "-inf" is digital silence.
		if v == "-inf" {
			return 0, true
		}
		return 0, false
	}
	if db <= levelFloorDB {
		return 0, true
	}
	if db >= 0 {
		return 1, true
	}
	return (db - levelFloorDB) / -levelFloorDB, true
}

// concatArgs builds the ffmpeg invocation that joins the paused-and-resumed
// segments listed in listPath into one ogg at outPath. Re-encoding rather
// than stream-copying: every segment is opus at the same settings, but the
// concat demuxer's copy path trips over ogg granule positions, whereas a
// decode/encode pass always yields a clean container.
func concatArgs(listPath, outPath string) []string {
	return []string{
		"-y",
		"-f", "concat", "-safe", "0", "-i", listPath,
		"-c:a", "libopus", "-application", "voip",
		outPath,
	}
}

// concatList renders the concat demuxer's list file for paths, one
// `file '<path>'` line each with single quotes escaped the way ffmpeg
// expects.
func concatList(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(p, "'", `'\''`))
		b.WriteString("'\n")
	}
	return b.String()
}

// durationSeconds truncates the wall-clock span between start and end to
// whole seconds, never negative (a clock going backwards or end<start
// shouldn't produce a nonsensical voice-note length).
func durationSeconds(start, end time.Time) int {
	d := end.Sub(start)
	if d <= 0 {
		return 0
	}
	return int(d.Seconds())
}

// Recorder captures one voice note at a time via ffmpeg. Not safe for
// concurrent Start calls; the composer guards against overlapping
// recordings.
//
// Pausing is modelled as segments: Pause finalizes the running ffmpeg's file
// and Resume starts a fresh one, so a paused note never contains the silence
// (or the ALSA buffer burst) a SIGSTOP'd capture would. Stop joins the
// segments.
type Recorder struct {
	cmd     *exec.Cmd
	outPath string // the segment being captured, "" while paused
	start   time.Time
	// segments are the finished files, oldest first; captured is their total
	// length, which Elapsed adds the live segment to.
	segments []string
	captured time.Duration
	// level is the latest meter reading (0..1, float64 bits) parsed off
	// ffmpeg's stderr; the composer polls it for the recording trace.
	level atomic.Uint64
}

// Level is the most recent microphone level, 0 (silence) to 1 (full
// scale). It holds its last value while paused and reads 0 before Start.
func (r *Recorder) Level() float64 {
	return math.Float64frombits(r.level.Load())
}

// readLevels feeds Level from ffmpeg's stderr until it closes.
func (r *Recorder) readLevels(stderr io.Reader) {
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		if lvl, ok := parseLevelLine(sc.Text()); ok {
			r.level.Store(math.Float64bits(lvl))
		}
	}
}

// Start launches ffmpeg capturing the default mic to a temp ogg file.
// Returns an error if ffmpeg isn't on PATH or fails to start (e.g. no
// PulseAudio source) rather than panicking.
func (r *Recorder) Start() error {
	return r.startSegment()
}

func (r *Recorder) startSegment() error {
	f, err := os.CreateTemp("", "chatot-voice-*.ogg")
	if err != nil {
		return fmt.Errorf("chatot/audio: create temp file: %w", err)
	}
	outPath := f.Name()
	_ = f.Close()

	cmd := exec.Command("ffmpeg", recordArgs(outPath)...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		os.Remove(outPath)
		return fmt.Errorf("chatot/audio: ffmpeg stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		os.Remove(outPath)
		return fmt.Errorf("chatot/audio: start ffmpeg: %w", err)
	}
	go r.readLevels(stderr)

	r.cmd = cmd
	r.outPath = outPath
	r.start = time.Now()
	return nil
}

// finishSegment signals ffmpeg to finalize the ogg container (SIGINT, which
// ffmpeg treats as a graceful stop), waits for it, and files the segment.
// ffmpeg exits non-zero when stopped by SIGINT even though it wrote a valid
// ogg, so the wait error alone can't gate success — a non-empty file is the
// real signal (see recordingOK).
func (r *Recorder) finishSegment() error {
	if r.cmd == nil {
		return nil
	}
	end := time.Now()
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("chatot/audio: signal ffmpeg: %w", err)
	}
	waitErr := r.cmd.Wait()
	r.cmd = nil

	data, _ := os.ReadFile(r.outPath)
	if err := recordingOK(data, waitErr); err != nil {
		os.Remove(r.outPath)
		r.outPath = ""
		return err
	}
	r.segments = append(r.segments, r.outPath)
	r.captured += end.Sub(r.start)
	r.outPath = ""
	return nil
}

// Pause finalizes the current segment; the timer stops until Resume. A no-op
// while already paused.
func (r *Recorder) Pause() error {
	return r.finishSegment()
}

// Resume starts capturing a new segment after Pause. A no-op while running.
func (r *Recorder) Resume() error {
	if r.cmd != nil {
		return nil
	}
	return r.startSegment()
}

// Paused reports whether a recording is between Pause and Resume.
func (r *Recorder) Paused() bool {
	return r.cmd == nil && len(r.segments) > 0
}

// Stop finalizes the recording and returns the ogg bytes plus the captured
// length in whole seconds (pauses excluded). Safe to call only after a
// successful Start; every temp file is removed on the way out.
func (r *Recorder) Stop() ([]byte, int, error) {
	if r.cmd == nil && len(r.segments) == 0 {
		return nil, 0, fmt.Errorf("chatot/audio: stop called without a running recording")
	}
	defer r.cleanup()

	if err := r.finishSegment(); err != nil {
		return nil, 0, err
	}
	secs := int(r.captured.Seconds())
	if len(r.segments) == 1 {
		data, err := os.ReadFile(r.segments[0])
		if err != nil {
			return nil, 0, fmt.Errorf("chatot/audio: read recording: %w", err)
		}
		return data, secs, nil
	}

	listPath := filepath.Join(os.TempDir(), fmt.Sprintf("chatot-voice-%d.txt", time.Now().UnixNano()))
	if err := os.WriteFile(listPath, []byte(concatList(r.segments)), 0o600); err != nil {
		return nil, 0, fmt.Errorf("chatot/audio: write concat list: %w", err)
	}
	defer os.Remove(listPath)
	outPath := strings.TrimSuffix(listPath, ".txt") + ".ogg"
	defer os.Remove(outPath)
	if out, err := exec.Command("ffmpeg", concatArgs(listPath, outPath)...).CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("chatot/audio: join segments: %w: %s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, 0, fmt.Errorf("chatot/audio: read joined recording: %w", err)
	}
	return data, secs, nil
}

// Cancel aborts an in-flight recording and discards whatever ffmpeg wrote,
// for the composer's 🗑 affordance. Unlike Stop it kills rather than
// interrupts, since the container never needs to be finalized. Safe to call
// on a Recorder that was never started, or twice.
func (r *Recorder) Cancel() {
	if r.cmd != nil {
		_ = r.cmd.Process.Kill()
		_ = r.cmd.Wait()
		r.cmd = nil
	}
	r.cleanup()
}

// cleanup removes every segment file and resets the segment state.
func (r *Recorder) cleanup() {
	if r.outPath != "" {
		os.Remove(r.outPath)
		r.outPath = ""
	}
	for _, p := range r.segments {
		os.Remove(p)
	}
	r.segments = nil
	r.captured = 0
}

// Elapsed is how much audio the recording holds so far, for the composer's
// timer: finished segments plus the live one. It does not advance while
// paused. Zero before Start.
func (r *Recorder) Elapsed() time.Duration {
	if r.cmd == nil {
		return r.captured
	}
	return r.captured + time.Since(r.start)
}

// recordingOK decides whether a stopped recording succeeded: a non-empty
// output file means ffmpeg finalized a valid ogg (the SIGINT exit code is
// irrelevant), while an empty file means ffmpeg never produced anything —
// only then does waitErr (or the emptiness itself) count as failure.
func recordingOK(fileBytes []byte, waitErr error) error {
	if len(fileBytes) > 0 {
		return nil
	}
	if waitErr != nil {
		return fmt.Errorf("chatot/audio: ffmpeg produced no recording: %w", waitErr)
	}
	return fmt.Errorf("chatot/audio: recording is empty")
}
