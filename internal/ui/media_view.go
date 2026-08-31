package ui

import (
	"context"
	"os"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// mediaView is the pure inline-vs-tap-to-load decision for one message's
// attachment, computed from a client.Message so it's unit-testable without
// a display.
type mediaView struct {
	IsMedia      bool
	Kind         string
	Chip         string
	HasLocal     bool
	LocalPath    string
	HasThumbnail bool
	Thumbnail    []byte
	IsGIF        bool
	ViewOnce     bool
	Viewed       bool
}

// mediaVM derives the media view-model for m. HasLocal requires both a
// stored local_path and the file actually existing on disk — a cache
// eviction (internal/media.Evict) NULLs local_path, but a stale in-memory
// copy or a manual deletion shouldn't be trusted either. HasThumbnail only
// applies once the full media isn't already cached: a downloaded attachment
// always renders in full, never the low-res preview.
func mediaVM(m client.Message) mediaView {
	if m.Attachment == nil {
		return mediaView{}
	}
	a := *m.Attachment
	v := mediaView{IsMedia: true, Kind: a.Kind, Chip: mediaChip(a), IsGIF: a.IsGIF, ViewOnce: a.ViewOnce, Viewed: a.Viewed}
	if a.LocalPath != "" {
		if info, err := os.Stat(a.LocalPath); err == nil && !info.IsDir() {
			v.HasLocal = true
			v.LocalPath = a.LocalPath
		}
	}
	if !v.HasLocal && len(a.Thumbnail) > 0 {
		v.HasThumbnail = true
		v.Thumbnail = a.Thumbnail
	}
	return v
}

// inlineable reports whether kind renders as an embedded widget once
// cached: image/video/sticker as a Picture/Video, audio as playback
// controls. Documents stay a chip.
func inlineable(kind string) bool {
	switch kind {
	case "image", "video", "sticker", "audio":
		return true
	default:
		return false
	}
}

// buildMediaContent renders a message's attachment: an inline widget
// (Picture/Video, or MediaControls for audio) if it's cached, otherwise a
// chip. An uncached chip downloads on click (goroutine -> glib.IdleAdd back
// to the main loop) and then either swaps itself for the inline widget or
// flips to an "open" affordance (document).
func buildMediaContent(msg client.Message, mv mediaView, c client.Client) gtk.Widgetter {
	slot := gtk.NewBox(gtk.OrientationVertical, 0)

	if mv.ViewOnce {
		slot.Append(buildViewOnceContent(msg, mv, c, slot))
		return slot
	}

	if mv.HasLocal && inlineable(mv.Kind) {
		widget := inlineMediaWidget(mv)
		if mv.IsGIF {
			widget = withGIFBadge(widget)
		}
		slot.Append(widget)
		return slot
	}

	if mv.HasThumbnail {
		if texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(mv.Thumbnail)); err == nil {
			slot.Append(buildThumbnailContent(texture, msg, mv, c, slot))
			return slot
		}
	}

	state := mv
	btn := gtk.NewButtonWithLabel(chipLabel(state))
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-bubble-media")
	btn.ConnectClicked(func() { onMediaChipClicked(&state, msg, c, slot, btn) })
	slot.Append(btn)
	return slot
}

// buildThumbnailContent renders the embedded low-res preview as a Picture
// with a small "tap to load" button overlaid, so the message reads
// instantly instead of showing a bare chip while the full media downloads.
func buildThumbnailContent(texture *gdk.Texture, msg client.Message, mv mediaView, c client.Client, slot *gtk.Box) gtk.Widgetter {
	size := 280
	height := 200
	if mv.Kind == "sticker" {
		size, height = stickerRenderSize, stickerRenderSize
	}
	pic := gtk.NewPictureForPaintable(texture)
	pic.SetCanShrink(true)
	pic.SetContentFit(gtk.ContentFitCover)
	pic.SetSizeRequest(size, height)

	overlay := gtk.NewOverlay()
	overlay.SetChild(pic)

	btn := gtk.NewButtonWithLabel("⬇ tap to load")
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-bubble-media")
	btn.SetHAlign(gtk.AlignCenter)
	btn.SetVAlign(gtk.AlignCenter)
	overlay.AddOverlay(btn)

	if mv.IsGIF {
		overlay.AddOverlay(gifBadge())
	}

	state := mv
	btn.ConnectClicked(func() { onThumbnailClicked(&state, msg, c, slot, overlay, btn) })
	return overlay
}

// gifBadge builds the small green "GIF" chip overlaid on a GIF attachment's
// thumbnail/inline widget — whether it loops on playback (deferred to a
// future feature) is a marker for now, not yet an actual player.
func gifBadge() *gtk.Label {
	badge := gtk.NewLabel("GIF")
	badge.AddCSSClass("chatot-gif-badge")
	badge.SetHAlign(gtk.AlignStart)
	badge.SetVAlign(gtk.AlignStart)
	badge.SetMarginStart(6)
	badge.SetMarginTop(6)
	return badge
}

// withGIFBadge wraps an already-built inline media widget in an overlay
// carrying the GIF badge, for the fully-downloaded render path.
func withGIFBadge(widget gtk.Widgetter) gtk.Widgetter {
	overlay := gtk.NewOverlay()
	overlay.SetChild(widget)
	overlay.AddOverlay(gifBadge())
	return overlay
}

// viewOnceRenderState is the pure placeholder-text/state selector for a
// view-once attachment: unopened shows the "click to open" invite, opened
// shows a spent tombstone that can never be reopened.
func viewOnceRenderState(isViewOnce, viewed bool) (title, subtitle string, spent bool) {
	if !isViewOnce {
		return "", "", false
	}
	if viewed {
		return "opened", "Opened", true
	}
	return "view once", "Click to open · closes after viewing", false
}

// viewOnceNoun/viewOnceIcon label a view-once bubble by attachment kind;
// only image and video ever carry WhatsApp's viewOnce flag.
func viewOnceNoun(kind string) string {
	if kind == "video" {
		return "Video"
	}
	return "Photo"
}

func viewOnceIcon(kind string) string {
	if kind == "video" {
		return "🎥"
	}
	return "📷"
}

// buildViewOnceContent renders a view-once attachment's placeholder bubble.
// Unopened, it's a clickable button that downloads the media, marks it
// viewed, and replaces itself with the spent tombstone; once viewed (either
// this run or restored from the store) it renders that tombstone directly
// and is never clickable again.
func buildViewOnceContent(msg client.Message, mv mediaView, c client.Client, slot *gtk.Box) gtk.Widgetter {
	state := mv
	if state.Viewed {
		return viewOnceTombstone(state)
	}

	title, subtitle, _ := viewOnceRenderState(state.ViewOnce, state.Viewed)
	body := gtk.NewBox(gtk.OrientationVertical, 2)
	body.AddCSSClass("chatot-viewonce")
	label := gtk.NewLabel(viewOnceIcon(state.Kind) + " " + viewOnceNoun(state.Kind) + " · " + title)
	label.AddCSSClass("chatot-viewonce-title")
	label.SetHAlign(gtk.AlignStart)
	sub := gtk.NewLabel(subtitle)
	sub.AddCSSClass("chatot-viewonce-subtitle")
	sub.SetHAlign(gtk.AlignStart)
	body.Append(label)
	body.Append(sub)

	btn := gtk.NewButton()
	btn.SetChild(body)
	btn.AddCSSClass("flat")
	btn.ConnectClicked(func() { onViewOnceClicked(&state, msg, c, slot, btn) })
	return btn
}

// viewOnceTombstone renders the permanent spent placeholder for an opened
// view-once attachment.
func viewOnceTombstone(mv mediaView) gtk.Widgetter {
	_, subtitle, _ := viewOnceRenderState(true, true)
	body := gtk.NewBox(gtk.OrientationVertical, 2)
	body.AddCSSClass("chatot-viewonce")
	body.AddCSSClass("chatot-viewonce-spent")
	label := gtk.NewLabel(viewOnceIcon(mv.Kind) + " " + viewOnceNoun(mv.Kind) + " · opened")
	label.AddCSSClass("chatot-viewonce-title")
	label.SetHAlign(gtk.AlignStart)
	sub := gtk.NewLabel(subtitle)
	sub.AddCSSClass("chatot-viewonce-subtitle")
	sub.SetHAlign(gtk.AlignStart)
	body.Append(label)
	body.Append(sub)
	return body
}

// onViewOnceClicked opens a view-once attachment exactly once: downloads it,
// marks it viewed in the store, then swaps the button for the spent
// tombstone regardless of whether the caller ever does anything with the
// downloaded path (opening it is left to the desktop default app).
func onViewOnceClicked(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, btn *gtk.Button) {
	btn.SetSensitive(false)
	go func() {
		path, err := c.DownloadMedia(context.Background(), msg.ID)
		if err == nil {
			err = c.MarkViewOnceOpened(context.Background(), msg.ChatJID, msg.ID)
		}
		glib.IdleAdd(func() {
			if err != nil {
				btn.SetSensitive(true)
				return
			}
			mv.HasLocal = true
			mv.LocalPath = path
			mv.Viewed = true
			slot.Remove(btn)
			slot.Append(viewOnceTombstone(*mv))
			openFile(path)
		})
	}()
}

// onThumbnailClicked downloads the full attachment and swaps the thumbnail
// overlay for the inline widget (or, for non-inlineable kinds like
// documents, a normal downloaded chip that opens the file on click).
func onThumbnailClicked(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, overlay *gtk.Overlay, btn *gtk.Button) {
	btn.SetSensitive(false)
	btn.SetLabel("Loading…")
	go func() {
		path, err := c.DownloadMedia(context.Background(), msg.ID)
		glib.IdleAdd(func() {
			if err != nil {
				btn.SetSensitive(true)
				btn.SetLabel("⬇ tap to load (failed)")
				return
			}
			mv.HasLocal = true
			mv.LocalPath = path
			slot.Remove(overlay)
			if inlineable(mv.Kind) {
				slot.Append(inlineMediaWidget(*mv))
				return
			}
			openBtn := gtk.NewButtonWithLabel(chipLabel(*mv))
			openBtn.AddCSSClass("flat")
			openBtn.AddCSSClass("chatot-bubble-media")
			openBtn.ConnectClicked(func() { openFile(mv.LocalPath) })
			slot.Append(openBtn)
		})
	}()
}

func chipLabel(mv mediaView) string {
	label := mv.Chip
	if !mv.HasLocal {
		label = "⬇ " + label
	}
	if mv.IsGIF {
		label += "  GIF"
	}
	return label
}

// stickerRenderSize is the mockup's bare-sticker footprint: no bubble, no
// media chip, just the image at roughly this size.
const stickerRenderSize = 120

func inlineMediaWidget(mv mediaView) gtk.Widgetter {
	switch mv.Kind {
	case "video":
		v := gtk.NewVideoForFilename(mv.LocalPath)
		v.SetSizeRequest(280, 200)
		return v
	case "audio":
		// gtk.MediaControls plays via its bound MediaStream but never
		// autoplays on its own, so no explicit Play()/Pause() call is needed.
		stream := gtk.NewMediaFileForFilename(mv.LocalPath)
		controls := gtk.NewMediaControls(stream)
		controls.SetSizeRequest(240, -1)
		return controls
	case "sticker":
		p := gtk.NewPictureForFilename(mv.LocalPath)
		p.SetCanShrink(true)
		p.SetContentFit(gtk.ContentFitContain)
		p.SetSizeRequest(stickerRenderSize, stickerRenderSize)
		p.AddCSSClass("chatot-sticker")
		return p
	}
	p := gtk.NewPictureForFilename(mv.LocalPath)
	p.SetCanShrink(true)
	p.SetContentFit(gtk.ContentFitContain)
	p.SetSizeRequest(280, 200)
	return p
}

// onMediaChipClicked handles a click on the media chip: if already
// downloaded it opens the file with the desktop default app; otherwise it
// downloads in the background and applies the result on the GTK main loop.
func onMediaChipClicked(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, btn *gtk.Button) {
	if mv.HasLocal {
		openFile(mv.LocalPath)
		return
	}
	btn.SetSensitive(false)
	go func() {
		path, err := c.DownloadMedia(context.Background(), msg.ID)
		glib.IdleAdd(func() {
			btn.SetSensitive(true)
			if err != nil {
				btn.SetLabel(mv.Chip + " (failed)")
				return
			}
			mv.HasLocal = true
			mv.LocalPath = path
			if inlineable(mv.Kind) {
				slot.Remove(btn)
				slot.Append(inlineMediaWidget(*mv))
				return
			}
			btn.SetLabel(chipLabel(*mv))
		})
	}()
}

// openFile launches path with the desktop's default application for it.
func openFile(path string) {
	launcher := gtk.NewFileLauncher(gio.NewFileForPath(path))
	launcher.Launch(context.Background(), nil, func(res gio.AsyncResulter) {
		_ = launcher.LaunchFinish(res)
	})
}
