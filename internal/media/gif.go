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
	"time"
)

// gifVideoTimeout bounds one GIF transcode; a long, large GIF takes a few
// seconds, a minute means ffmpeg is stuck.
const gifVideoTimeout = 2 * time.Minute

// IsGIF reports whether the file at path, of the given MIME type (either may
// be empty), is a GIF picture.
func IsGIF(path, mime string) bool {
	m := strings.ToLower(mime)
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	if m != "" {
		return m == "image/gif"
	}
	return strings.EqualFold(filepath.Ext(path), ".gif")
}

// GIFVideo returns an H.264 mp4 of the GIF at path, cached under cacheDir
// (keyed by path, size and mtime). WhatsApp has no animated pictures: a GIF
// is an mp4 it plays looped and muted, and a GIF sent as a picture arrives
// as its first frame. GTK cannot play a GIF either, nor gdk-pixbuf decode a
// large one at a reduced size.
func GIFVideo(ctx context.Context, cacheDir, path string) (string, error) {
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
	out := filepath.Join(cacheDir, gifVideoName(path, info.Size(), info.ModTime().UnixNano()))
	lock := transcodeLock(out)
	lock.Lock()
	defer lock.Unlock()
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, gifVideoTimeout)
	defer cancel()
	tmp := out + ".part"
	// yuv420p and even sides are what phones' hardware decoders take;
	// faststart puts the index first so playback can begin mid-download.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-i", path,
		"-map_metadata", "-1", "-an",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-movflags", "+faststart", "-f", "mp4", tmp)
	if msg, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("chatot/media: gif to mp4: %w: %s", err, strings.TrimSpace(string(msg)))
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}

// gifVideoName is the cached mp4's file name, from the source's identity
// so an edited GIF is transcoded again.
func gifVideoName(path string, size, mtime int64) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", path, size, mtime)))
	return hex.EncodeToString(sum[:8]) + "-gif.mp4"
}
