// Package audio captures voice notes from the default microphone via an
// ffmpeg subprocess, encoding straight to mono ogg/opus.
package audio

import (
	"fmt"
	"os"
	"os/exec"
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
		"-c:a", "libopus", "-application", "voip",
		outPath,
	}
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
type Recorder struct {
	cmd     *exec.Cmd
	outPath string
	start   time.Time
}

// Start launches ffmpeg capturing the default mic to a temp ogg file.
// Returns an error if ffmpeg isn't on PATH or fails to start (e.g. no
// PulseAudio source) rather than panicking.
func (r *Recorder) Start() error {
	f, err := os.CreateTemp("", "chatot-voice-*.ogg")
	if err != nil {
		return fmt.Errorf("chatot/audio: create temp file: %w", err)
	}
	outPath := f.Name()
	_ = f.Close()

	cmd := exec.Command("ffmpeg", recordArgs(outPath)...)
	if err := cmd.Start(); err != nil {
		os.Remove(outPath)
		return fmt.Errorf("chatot/audio: start ffmpeg: %w", err)
	}

	r.cmd = cmd
	r.outPath = outPath
	r.start = time.Now()
	return nil
}

// Stop signals ffmpeg to finalize the ogg container (SIGINT, which ffmpeg
// treats as a graceful stop), waits for it to exit, and returns the
// recorded bytes plus elapsed duration. Safe to call only after a
// successful Start.
func (r *Recorder) Stop() ([]byte, int, error) {
	if r.cmd == nil {
		return nil, 0, fmt.Errorf("chatot/audio: stop called without a running recording")
	}
	defer os.Remove(r.outPath)

	end := time.Now()
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		return nil, 0, fmt.Errorf("chatot/audio: signal ffmpeg: %w", err)
	}
	waitErr := r.cmd.Wait()
	r.cmd = nil

	// ffmpeg exits non-zero when stopped by SIGINT even though it wrote a
	// valid ogg, so waitErr alone can't gate success — a non-empty file is
	// the real signal (see recordingOK).
	data, _ := os.ReadFile(r.outPath)
	if err := recordingOK(data, waitErr); err != nil {
		return nil, 0, err
	}

	return data, durationSeconds(r.start, end), nil
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
