package media

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// transcodes serialises PlayableAudio per output path: two callers asking
// for the same MP3 at once (the tray rebuilds its stage on every selection
// change) must not both write the cached file.
var transcodes struct {
	sync.Mutex
	busy map[string]*sync.Mutex
}

func transcodeLock(out string) *sync.Mutex {
	transcodes.Lock()
	defer transcodes.Unlock()
	if transcodes.busy == nil {
		transcodes.busy = map[string]*sync.Mutex{}
	}
	m, ok := transcodes.busy[out]
	if !ok {
		m = &sync.Mutex{}
		transcodes.busy[out] = m
	}
	return m
}

// transcodeTimeout bounds one ffmpeg audio transcode.
const transcodeTimeout = 60 * time.Second

// NeedsTranscode reports whether GTK's media backend cannot be handed the
// audio file at path directly. MP3 through GtkMediaFile aborts the process
// on the GStreamer this app ships with (a decodebin3 assertion), so those
// are transcoded to FLAC before playback.
func NeedsTranscode(path, mime string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".mp2", ".mpga":
		return true
	}
	m := strings.ToLower(mime)
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	switch m {
	case "audio/mpeg", "audio/mp3", "audio/mpeg3", "audio/x-mpeg-3", "audio/x-mp3":
		return true
	}
	return false
}

// PlayableAudio returns a path GTK can play for the audio file at path: the
// file itself when it is safe, else a FLAC transcode cached under cacheDir
// (keyed by path, size and mtime, so an edited file is re-encoded).
func PlayableAudio(ctx context.Context, cacheDir, path, mime string) (string, error) {
	return SpeedAudio(ctx, cacheDir, path, mime, 1)
}

// SpeedAudio is PlayableAudio at a playback tempo: the file itself when it
// plays as is at 1×, else a FLAC render cached under cacheDir. GTK's media
// stream has no rate of its own, so a faster note is the note re-rendered
// through ffmpeg's atempo (which keeps the pitch), and the player maps the
// shorter file back onto the note's own timeline.
func SpeedAudio(ctx context.Context, cacheDir, path, mime string, speed float64) (string, error) {
	if speed <= 0 {
		speed = 1
	}
	if speed == 1 && !NeedsTranscode(path, mime) {
		return path, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", ErrNoRenderer
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(cacheDir, speedCacheName(path, info.Size(), info.ModTime().UnixNano(), speed))
	lock := transcodeLock(out)
	lock.Lock()
	defer lock.Unlock()
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, transcodeTimeout)
	defer cancel()
	tmp := out + ".part"
	args := []string{"-nostdin", "-loglevel", "error", "-y", "-i", path, "-vn", "-map_metadata", "-1"}
	if speed != 1 {
		args = append(args, "-filter:a", fmt.Sprintf("atempo=%g", speed))
	}
	args = append(args, "-c:a", "flac", "-f", "flac", tmp)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if msg, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("chatot/media: ffmpeg transcode: %w: %s", err, strings.TrimSpace(string(msg)))
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}

// speedCacheName is the cached render's file name: the source's identity
// (path, size, mtime) and, past 1×, the tempo, so each speed of a note is
// its own file and an edited source is re-rendered.
func speedCacheName(path string, size, mtime int64, speed float64) string {
	key := fmt.Sprintf("%s|%d|%d", path, size, mtime)
	sum := sha1.Sum([]byte(key))
	name := hex.EncodeToString(sum[:8])
	if speed != 1 {
		name += fmt.Sprintf("-x%g", speed)
	}
	return name + ".flac"
}
