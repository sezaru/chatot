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
	if !NeedsTranscode(path, mime) {
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
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())))
	out := filepath.Join(cacheDir, hex.EncodeToString(sum[:8])+".flac")
	lock := transcodeLock(out)
	lock.Lock()
	defer lock.Unlock()
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, transcodeTimeout)
	defer cancel()
	tmp := out + ".part"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-y",
		"-i", path, "-vn", "-map_metadata", "-1", "-c:a", "flac", "-f", "flac", tmp)
	if msg, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("chatot/media: ffmpeg transcode: %w: %s", err, strings.TrimSpace(string(msg)))
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}
