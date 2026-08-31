package ui

import (
	"context"
	"os"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// mediaView is the pure inline-vs-tap-to-load decision for one message's
// attachment, computed from a client.Message so it's unit-testable without
// a display.
type mediaView struct {
	IsMedia      bool
	Kind         string
	Chip         string
	Caption      string // caption text or filename, without the "[kind]" prefix
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
	caption := a.Caption
	if caption == "" {
		caption = a.Filename
	}
	v := mediaView{IsMedia: true, Kind: a.Kind, Chip: mediaChip(a), Caption: caption, IsGIF: a.IsGIF, ViewOnce: a.ViewOnce, Viewed: a.Viewed}
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
	if mv.HasLocal { // downloaded document → "Open" row
		slot.Append(buildDocumentOpenRow(mv))
		return slot
	}

	// Not downloaded. Per the mockup's attachment-load states: image/video are a
	// hatched 280×115 tile with a centred green ⬇ circle (or the embedded
	// thumbnail with that circle overlaid); audio/document are a compact row
	// with a small ⬇ circle and a two-line label.
	state := mv
	if isVisualKind(mv.Kind) {
		slot.Append(buildAttachmentTile(&state, msg, c, slot))
		return slot
	}
	slot.Append(buildMediaRow(&state, msg, c, slot))
	return slot
}

// isVisualKind reports whether the attachment renders as a picture-shaped tile
// (image/video/sticker) rather than a document/voice row.
func isVisualKind(kind string) bool {
	return kind == "image" || kind == "video" || kind == "sticker"
}

// buildMediaTile is the image/video not-downloaded state: the embedded
// thumbnail if we have one, else a diagonal-hatch placeholder, with a green
// download circle centred on it. Clicking anywhere downloads and swaps to the
// inline widget.
func buildAttachmentTile(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box) gtk.Widgetter {
	w, h := 280, 158
	if mv.Kind == "sticker" {
		w, h = stickerRenderSize, stickerRenderSize
	}

	overlay := gtk.NewOverlay()
	if mv.HasThumbnail {
		if texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(mv.Thumbnail)); err == nil {
			pic := gtk.NewPictureForPaintable(texture)
			pic.SetCanShrink(true)
			pic.SetContentFit(gtk.ContentFitCover)
			pic.SetSizeRequest(w, h)
			pic.AddCSSClass("chatot-media-tile")
			overlay.SetChild(pic)
		}
	}
	if overlay.Child() == nil {
		hatch := gtk.NewBox(gtk.OrientationVertical, 7)
		hatch.AddCSSClass("chatot-media-hatch")
		hatch.SetSizeRequest(w, h)
		caption := gtk.NewLabel(mediaIcon(mv.Kind) + "  " + mediaNoun(mv.Kind))
		caption.AddCSSClass("chatot-media-caption")
		caption.SetVAlign(gtk.AlignEnd)
		caption.SetHAlign(gtk.AlignCenter)
		caption.SetMarginBottom(10)
		hatch.Append(caption)
		overlay.SetChild(hatch)
	}

	circle := mediaDownloadCircle(true)
	circle.SetHAlign(gtk.AlignCenter)
	circle.SetVAlign(gtk.AlignCenter)
	overlay.AddOverlay(circle)
	if mv.IsGIF {
		overlay.AddOverlay(gifBadge())
	}

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) { downloadAndSwap(mv, msg, c, slot, overlay, circle) })
	overlay.AddController(click)
	return overlay
}

// buildMediaRow is the audio/document not-downloaded state: a small green ⬇
// circle plus a two-line label (noun/filename over "Click to download").
func buildMediaRow(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-media-row")

	circle := mediaDownloadCircle(false)
	circle.SetVAlign(gtk.AlignCenter)
	row.Append(circle)

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetVAlign(gtk.AlignCenter)
	title := gtk.NewLabel(mediaRowTitle(*mv))
	title.SetXAlign(0)
	title.AddCSSClass("chatot-media-row-title")
	sub := gtk.NewLabel("Click to download")
	sub.SetXAlign(0)
	sub.AddCSSClass("chatot-media-row-sub")
	col.Append(title)
	col.Append(sub)
	row.Append(col)

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) { downloadAndSwap(mv, msg, c, slot, row, circle) })
	row.AddController(click)
	return row
}

// mediaRowTitle labels a voice/document row — the filename for documents, the
// plain noun for voice notes.
func mediaRowTitle(mv mediaView) string {
	if mv.Kind == "document" && mv.Caption != "" {
		return mediaIcon(mv.Kind) + "  " + mv.Caption
	}
	return mediaIcon(mv.Kind) + "  " + mediaNoun(mv.Kind)
}

// buildDocumentOpenRow is the downloaded-document state: a 📄 + filename with
// an "Open" button, per the mockup.
func buildDocumentOpenRow(mv mediaView) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 9)
	row.AddCSSClass("chatot-media-row")

	icon := gtk.NewLabel("📄")
	row.Append(icon)

	name := mv.Caption
	if name == "" {
		name = mediaNoun(mv.Kind)
	}
	label := gtk.NewLabel(name)
	label.SetXAlign(0)
	label.SetHExpand(true)
	label.SetEllipsize(pango.EllipsizeMiddle)
	row.Append(label)

	open := gtk.NewButtonWithLabel("Open")
	open.AddCSSClass("chatot-media-open")
	open.SetVAlign(gtk.AlignCenter)
	open.ConnectClicked(func() { openFile(mv.LocalPath) })
	row.Append(open)
	return row
}

// downloadAndSwap downloads msg's media off the main loop and, on success,
// replaces `current` in slot with the inline widget (image/video/audio) or the
// document "Open" row; on failure it swaps in a click-to-retry state.
func downloadAndSwap(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, current gtk.Widgetter, circle *gtk.Label) {
	circle.SetText("…")
	go func() {
		path, err := c.DownloadMedia(context.Background(), msg.ID)
		glib.IdleAdd(func() {
			if err != nil {
				slot.Remove(current)
				slot.Append(buildMediaRetry(mv, msg, c, slot))
				return
			}
			mv.HasLocal = true
			mv.LocalPath = path
			slot.Remove(current)
			switch {
			case inlineable(mv.Kind):
				w := inlineMediaWidget(*mv)
				if mv.IsGIF {
					w = withGIFBadge(w)
				}
				slot.Append(w)
			default:
				slot.Append(buildDocumentOpenRow(*mv))
			}
		})
	}()
}

// buildMediaRetry is the failed-download state: a red ↻ circle with a
// "Download failed · click to retry" row.
func buildMediaRetry(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-media-row")

	circle := gtk.NewLabel("↻")
	circle.AddCSSClass("chatot-media-dl")
	circle.AddCSSClass("chatot-media-dl-sm")
	circle.AddCSSClass("chatot-media-dl-fail")
	circle.SetVAlign(gtk.AlignCenter)
	row.Append(circle)

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetVAlign(gtk.AlignCenter)
	title := gtk.NewLabel(mediaRowTitle(*mv))
	title.SetXAlign(0)
	title.AddCSSClass("chatot-media-row-title")
	sub := gtk.NewLabel("Download failed · click to retry")
	sub.SetXAlign(0)
	sub.AddCSSClass("chatot-media-row-fail")
	col.Append(title)
	col.Append(sub)
	row.Append(col)

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) {
		slot.Remove(row)
		if isVisualKind(mv.Kind) {
			slot.Append(buildAttachmentTile(mv, msg, c, slot))
		} else {
			slot.Append(buildMediaRow(mv, msg, c, slot))
		}
	})
	row.AddController(click)
	return row
}

// mediaDownloadCircle is the green ⬇ disc overlaid on / leading an
// undownloaded attachment (large = 38px tile centre, small = 28px row).
func mediaDownloadCircle(large bool) *gtk.Label {
	g := gtk.NewLabel("⬇")
	g.AddCSSClass("chatot-media-dl")
	if large {
		g.AddCSSClass("chatot-media-dl-lg")
	} else {
		g.AddCSSClass("chatot-media-dl-sm")
	}
	return g
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

// mediaNoun / mediaIcon give an uncached attachment a human label + glyph
// (matching the mockup's attachment-load states) instead of a raw "[video]".
func mediaNoun(kind string) string {
	switch kind {
	case "video":
		return "Video"
	case "image":
		return "Photo"
	case "document":
		return "Document"
	case "audio":
		return "Voice message"
	case "sticker":
		return "Sticker"
	default:
		return "Attachment"
	}
}

func mediaIcon(kind string) string {
	switch kind {
	case "video":
		return "🎥"
	case "image":
		return "📷"
	case "document":
		return "📄"
	case "audio":
		return "🎤"
	case "sticker":
		return "🩹"
	default:
		return "📎"
	}
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

// openFile launches path with the desktop's default application for it.
func openFile(path string) {
	launcher := gtk.NewFileLauncher(gio.NewFileForPath(path))
	launcher.Launch(context.Background(), nil, func(res gio.AsyncResulter) {
		_ = launcher.LaunchFinish(res)
	})
}
