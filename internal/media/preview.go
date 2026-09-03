package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// previewTimeout bounds one helper invocation; a preview is decoration and
// must never stall the attach tray.
const previewTimeout = 8 * time.Second

// PreviewMaxSide is the longest side of a generated preview, in pixels. The
// same JPEG doubles as the thumbnail WhatsApp embeds in the message, so it is
// kept small.
const PreviewMaxSide = 512

// VideoPoster grabs one early frame of the video at path as a JPEG. It
// shells out to ffmpeg, which chatot already depends on for voice notes.
func VideoPoster(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	// A frame half a second in skips the black first frame many clips
	// open on, but -ss on a clip shorter than that would yield nothing, so
	// fall back to the first frame.
	for _, seek := range []string{"0.5", "0"} {
		var out, errb bytes.Buffer
		cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-ss", seek, "-i", path,
			"-frames:v", "1", "-vf", scaleFilter(), "-f", "image2pipe", "-vcodec", "mjpeg", "-")
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err == nil && out.Len() > 0 {
			return out.Bytes(), nil
		} else if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("chatot/media: video poster: %w", ctx.Err())
		}
	}
	return nil, errors.New("chatot/media: video poster: ffmpeg produced no frame")
}

// scaleFilter is the ffmpeg -vf that fits a frame inside PreviewMaxSide on
// its longest side without upscaling (even dimensions, as JPEG prefers).
func scaleFilter() string {
	s := strconv.Itoa(PreviewMaxSide)
	return "scale='min(" + s + ",iw)':'min(" + s + ",ih)':force_original_aspect_ratio=decrease:force_divisible_by=2"
}

// VideoInfo is what ffprobe reports about a clip: its pixel size and
// duration in whole seconds. Zero fields mean ffprobe didn't say.
type VideoInfo struct {
	Width, Height int
	Seconds       int
}

// ProbeVideo reads a clip's dimensions and duration with ffprobe.
func ProbeVideo(ctx context.Context, path string) (VideoInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return VideoInfo{}, fmt.Errorf("chatot/media: ffprobe: %w", err)
	}
	return parseProbe(string(out)), nil
}

// parseProbe reads ffprobe's csv=p=0 output: a "width,height" line for the
// stream and a "duration" line for the format, in either order.
func parseProbe(out string) VideoInfo {
	var info VideoInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		switch len(fields) {
		case 2:
			info.Width, _ = strconv.Atoi(fields[0])
			info.Height, _ = strconv.Atoi(fields[1])
		case 1:
			if secs, err := strconv.ParseFloat(fields[0], 64); err == nil && secs > 0 {
				info.Seconds = int(secs + 0.5)
			}
		}
	}
	return info
}

// AudioSeconds reads an audio file's duration in whole seconds via ffprobe.
func AudioSeconds(ctx context.Context, path string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, fmt.Errorf("chatot/media: ffprobe: %w", err)
	}
	return parseProbe(string(out)).Seconds, nil
}

// PDFPage renders the first page of the PDF at path as a JPEG, using
// poppler's pdftoppm. ErrNoRenderer is returned when it isn't installed, so
// callers can fall back to a plain document card.
func PDFPage(ctx context.Context, path string) ([]byte, error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, ErrNoRenderer
	}
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "chatot-pdf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	cmd := exec.CommandContext(ctx, "pdftoppm", "-f", "1", "-l", "1", "-jpeg",
		"-scale-to", strconv.Itoa(PreviewMaxSide), path, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("chatot/media: pdftoppm: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// pdftoppm names the page with a zero-padded index whose width depends
	// on the page count, so glob rather than guess.
	pages, _ := filepath.Glob(prefix + "*.jpg")
	if len(pages) == 0 {
		return nil, errors.New("chatot/media: pdftoppm wrote no page")
	}
	return os.ReadFile(pages[0])
}

// ErrNoRenderer reports that the helper a preview needs isn't installed.
var ErrNoRenderer = errors.New("chatot/media: preview renderer not installed")

// PDFPageCount reads the page count with poppler's pdfinfo; ErrNoRenderer
// when it isn't installed, 0 pages on any other failure.
func PDFPageCount(ctx context.Context, path string) (int, error) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return 0, ErrNoRenderer
	}
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pdfinfo", path).Output()
	if err != nil {
		return 0, fmt.Errorf("chatot/media: pdfinfo: %w", err)
	}
	return parsePDFPages(string(out)), nil
}

// parsePDFPages pulls "Pages: N" out of pdfinfo's output.
func parsePDFPages(out string) int {
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "Pages:") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
		if err == nil {
			return n
		}
	}
	return 0
}

// PDFPageAt renders page (1-based) of the PDF at path as a JPEG whose
// longer side is maxSide px, for the attachment viewer's page stage.
func PDFPageAt(ctx context.Context, path string, page, maxSide int) ([]byte, error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, ErrNoRenderer
	}
	if page < 1 {
		page = 1
	}
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "chatot-pdf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	p := strconv.Itoa(page)
	cmd := exec.CommandContext(ctx, "pdftoppm", "-f", p, "-l", p, "-jpeg",
		"-scale-to", strconv.Itoa(maxSide), path, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("chatot/media: pdftoppm: %w: %s", err, strings.TrimSpace(string(out)))
	}
	pages, _ := filepath.Glob(prefix + "*.jpg")
	if len(pages) == 0 {
		return nil, errors.New("chatot/media: pdftoppm wrote no page")
	}
	return os.ReadFile(pages[0])
}
