package ui

import (
	"context"
	"log"
	"math"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/media"
)

// trayKind sorts a queued file into the preview it gets: a picture, a
// video player with a poster, an audio player, a rendered PDF page, or the
// plain "No preview available" card.
type trayKind int

const (
	trayOther trayKind = iota
	trayImage
	trayVideo
	trayAudio
	trayPDF
)

// trayKindOf classifies path by extension, the same way the mockup picks
// its tray icon.
func trayKindOf(path string) trayKind {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".avif", ".svg", ".svgz":
		return trayImage
	case ".mp4", ".mkv", ".webm", ".mov", ".m4v", ".3gp":
		return trayVideo
	case ".mp3", ".ogg", ".opus", ".oga", ".wav", ".flac", ".m4a", ".aac":
		return trayAudio
	case ".pdf":
		return trayPDF
	}
	return trayOther
}

// trayPreview is what the background helpers found out about a queued
// file: a JPEG poster/page image for videos, PDFs and (as the thumbnail
// sent with the message) pictures, plus a duration for anything timed.
type trayPreview struct {
	Image   []byte
	Seconds int
	Width   int
	Height  int
	Done    bool // the helpers have run, whether or not they produced anything
}

// buildTrayPreview runs the helpers for path off the main loop. It never
// fails outright: a missing renderer just leaves Image empty, and the tray
// shows the plain card.
func buildTrayPreview(path string) trayPreview {
	ctx := context.Background()
	p := trayPreview{Done: true}
	switch trayKindOf(path) {
	case trayImage:
		// The wire thumbnail: the picture itself scaled down, through
		// gdk-pixbuf (which also covers SVG), not a video frame grab.
		if img, err := imageThumbnail(path); err == nil {
			p.Image = img
		} else {
			log.Printf("chatot: thumbnail for %s: %v", filepath.Base(path), err)
		}
	case trayVideo:
		if img, err := media.VideoPoster(ctx, path); err == nil {
			p.Image = img
		} else {
			log.Printf("chatot: video poster for %s: %v", filepath.Base(path), err)
		}
		if info, err := media.ProbeVideo(ctx, path); err == nil {
			p.Seconds, p.Width, p.Height = info.Seconds, info.Width, info.Height
		}
	case trayAudio:
		if secs, err := media.AudioSeconds(ctx, path); err == nil {
			p.Seconds = secs
		}
	case trayPDF:
		if img, err := media.PDFPage(ctx, path); err == nil {
			p.Image = img
		} else if err != media.ErrNoRenderer {
			log.Printf("chatot: pdf page for %s: %v", filepath.Base(path), err)
		}
	}
	return p
}

// imageThumbnail scales the picture at path to fit media.PreviewMaxSide and
// encodes it as JPEG, for the thumbnail embedded in the message. gdk-pixbuf
// is safe to use off the main loop, and its loaders cover SVG as well.
func imageThumbnail(path string) ([]byte, error) {
	pb, err := gdkpixbuf.NewPixbufFromFileAtScale(path, media.PreviewMaxSide, media.PreviewMaxSide, true)
	if err != nil {
		return nil, err
	}
	// JPEG has no alpha: composite onto white first so a transparent PNG
	// or SVG doesn't come out black.
	if pb.HasAlpha() {
		pb = pb.CompositeColorSimple(pb.Width(), pb.Height(), gdkpixbuf.InterpBilinear, 255, 16, 0xffffff, 0xffffff)
	}
	return pb.SaveToBufferv("jpeg", []string{"quality"}, []string{"82"})
}

// pixbufFromBytes decodes an encoded image (JPEG/PNG) into a pixbuf.
func pixbufFromBytes(data []byte) (*gdkpixbuf.Pixbuf, error) {
	loader := gdkpixbuf.NewPixbufLoader()
	if err := loader.Write(data); err != nil {
		loader.Close()
		return nil, err
	}
	if err := loader.Close(); err != nil {
		return nil, err
	}
	return loader.Pixbuf(), nil
}

// coverThumb is a w×h DrawingArea that paints pixbuf scaled to cover the
// box and clipped to radius-px rounded corners. Unlike a GtkPicture, whose
// natural size is the image's own pixel size, it is exactly the size asked
// for; that is what keeps the tray strip's tiles 54px. alignTop anchors the
// crop to the image's top edge (a document page) instead of its centre.
func coverThumb(pixbuf *gdkpixbuf.Pixbuf, w, h int, radius float64, alignTop bool) *gtk.DrawingArea {
	area := gtk.NewDrawingArea()
	area.SetContentWidth(w)
	area.SetContentHeight(h)
	area.SetSizeRequest(w, h)
	pw, ph := float64(pixbuf.Width()), float64(pixbuf.Height())
	area.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		if pw <= 0 || ph <= 0 {
			return
		}
		fw, fh := float64(width), float64(height)
		roundedRectPath(cr, 0, 0, fw, fh, radius)
		cr.Clip()
		scale := math.Max(fw/pw, fh/ph)
		dx := (fw - pw*scale) / 2
		dy := (fh - ph*scale) / 2
		if alignTop {
			dy = 0
		}
		cr.Translate(dx, dy)
		cr.Scale(scale, scale)
		gdk.CairoSetSourcePixbuf(cr, pixbuf, 0, 0)
		cr.Paint()
	})
	return area
}

// roundedRectPath traces a rounded rectangle for a subsequent clip or fill.
func roundedRectPath(cr *cairo.Context, x, y, w, h, r float64) {
	if r <= 0 {
		cr.Rectangle(x, y, w, h)
		return
	}
	cr.NewSubPath()
	cr.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	cr.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	cr.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	cr.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	cr.ClosePath()
}

// thumbRadius is the strip tile's corner radius, per the mockup.
const thumbRadius = 8

// newTrayThumb builds the strip tile's picture for path: the file itself
// for an image, the poster/page for a video or PDF once the helpers have
// produced one. nil means "use the glyph".
func (t *AttachTray) newTrayThumb(path string, size int) gtk.Widgetter {
	var pixbuf *gdkpixbuf.Pixbuf
	switch trayKindOf(path) {
	case trayImage:
		// Two device pixels per logical pixel keeps the tile crisp on HiDPI.
		pb, err := gdkpixbuf.NewPixbufFromFileAtScale(path, size*2, size*2, true)
		if err != nil {
			return nil
		}
		pixbuf = pb
	default:
		p, ok := t.previews[path]
		if !ok || len(p.Image) == 0 {
			return nil
		}
		pb, err := pixbufFromBytes(p.Image)
		if err != nil {
			return nil
		}
		pixbuf = pb
	}
	thumb := coverThumb(pixbuf, size, size, thumbRadius, trayKindOf(path) == trayPDF)
	thumb.AddCSSClass("chatot-tray-thumb")
	return thumb
}

// gdkpixbufAtScale loads path scaled to fit a side×side box, keeping the
// aspect ratio.
func gdkpixbufAtScale(path string, side int) (*gdkpixbuf.Pixbuf, error) {
	return gdkpixbuf.NewPixbufFromFileAtScale(path, side, side, true)
}
