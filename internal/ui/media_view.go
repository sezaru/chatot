package ui

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
	"chatot/internal/media"
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
	Size         int64
	DurationSecs int
	MimeType     string
	Filename     string
	// FromMe picks the outgoing-bubble colours for inline players.
	FromMe bool
}

// mediaTileLabel is the caption under a picture tile's download disc:
// "📷 Photo · 840 KB", dropping the size when unknown.
func mediaTileLabel(mv mediaView) string {
	return joinMeta(mediaIcon(mv.Kind)+"  "+mediaNoun(mv.Kind), humanSize(mv.Size))
}

// mediaRowTitleText is the bold first line of a document/voice row: the
// filename for a document, "🎤 Voice message · 0:12" for a voice note.
func mediaRowTitleText(mv mediaView) string {
	if mv.Kind == "document" && mv.Caption != "" {
		return mediaIcon(mv.Kind) + "  " + mv.Caption
	}
	return joinMeta(mediaIcon(mv.Kind)+"  "+mediaNoun(mv.Kind), humanDuration(mv.DurationSecs))
}

// mediaRowMeta is the dim second line: the mockup's "Click to download · PDF ·
// 1.2 MB" for a document, "Click to download · 48 KB" for a voice note. Each
// segment drops out when its value is unknown.
func mediaRowMeta(mv mediaView) string {
	if mv.Kind == "document" {
		return joinMeta("Click to download", docTypeLabel(mv.MimeType, mv.Filename), humanSize(mv.Size))
	}
	return joinMeta("Click to download", humanSize(mv.Size))
}

// mediaOpenMeta is the dim line under a downloaded document's name:
// "PDF · 1.2 MB".
func mediaOpenMeta(mv mediaView) string {
	return joinMeta(docTypeLabel(mv.MimeType, mv.Filename), humanSize(mv.Size))
}

// humanSize renders a byte count the way the mockup's sublines do ("840 KB",
// "1.2 MB"). An unknown size (0) renders empty so callers can drop the
// segment entirely rather than print "0 B".
func humanSize(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%d KB", (n+512)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}

// humanDuration renders a playback length as the mockup's "0:12" / "1:04:20".
// 0 renders empty — unknown, not "0:00".
func humanDuration(secs int) string {
	if secs <= 0 {
		return ""
	}
	if secs < 3600 {
		return fmt.Sprintf("%d:%02d", secs/60, secs%60)
	}
	return fmt.Sprintf("%d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
}

// docTypeLabel is the short, upper-case document type the mockup shows between
// the filename and the size ("PDF"), derived from the MIME type with the
// extension as a fallback. Empty when neither says anything useful.
func docTypeLabel(mime, filename string) string {
	if i := strings.LastIndex(mime, "/"); i >= 0 {
		sub := mime[i+1:]
		// application/vnd.openxmlformats-officedocument.wordprocessingml.document
		// and friends say nothing a user wants to read; fall through to the
		// extension for those.
		if sub != "" && !strings.HasPrefix(sub, "vnd.") && !strings.HasPrefix(sub, "x-") && len(sub) <= 5 {
			return strings.ToUpper(sub)
		}
	}
	if i := strings.LastIndex(filename, "."); i >= 0 && i+1 < len(filename) {
		if ext := filename[i+1:]; len(ext) <= 5 {
			return strings.ToUpper(ext)
		}
	}
	return ""
}

// joinMeta joins the non-empty parts of a subline with the mockup's " · "
// separator, so a missing size or type silently drops out instead of leaving
// a dangling separator.
func joinMeta(parts ...string) string {
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
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
	v := mediaView{
		IsMedia: true, Kind: a.Kind, Chip: mediaChip(a), Caption: caption,
		IsGIF: a.IsGIF, ViewOnce: a.ViewOnce, Viewed: a.Viewed,
		Size: a.Size, DurationSecs: a.DurationSecs, FromMe: m.FromMe,
	}
	// A document's own name belongs in the title, not repeated in the meta
	// line, so remember the MIME type separately for docTypeLabel.
	v.MimeType = a.MimeType
	v.Filename = a.Filename
	if a.LocalPath != "" {
		if info, err := os.Stat(a.LocalPath); err == nil && !info.IsDir() {
			v.HasLocal = true
			v.LocalPath = a.LocalPath
		}
	}
	// A downloaded picture/video renders in full, never its low-res
	// preview; a document keeps its page thumbnail either way, since the
	// file itself is never drawn inline.
	if (!v.HasLocal || a.Kind == "document") && len(a.Thumbnail) > 0 {
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

// inlineableMedia is inlineable for a concrete attachment. Audio that GTK
// cannot be handed directly (MP3) is transcoded on the way in, so every
// audio kind renders as the voice row.
func inlineableMedia(mv mediaView) bool {
	return inlineable(mv.Kind)
}

// buildMediaContent renders a message's attachment: an inline widget
// (Picture/Video, or MediaControls for audio) if it's cached, otherwise a
// chip. An uncached chip downloads on click (goroutine -> glib.IdleAdd back
// to the main loop) and then either swaps itself for the inline widget or
// flips to an "open" affordance (document).
func buildMediaContent(msg client.Message, mv mediaView, c client.Client, open func(path string)) gtk.Widgetter {
	slot := gtk.NewBox(gtk.OrientationVertical, 0)

	if mv.ViewOnce {
		slot.Append(buildViewOnceContent(msg, mv, c, slot))
		return slot
	}

	if mv.HasLocal && inlineableMedia(mv) {
		widget := inlineMediaWidget(mv, open)
		widget = withMediaOpener(widget, mv, open)
		if mv.IsGIF {
			widget = withGIFBadge(widget)
		}
		slot.Append(widget)
		return slot
	}
	if mv.HasLocal { // downloaded document → "View" row
		slot.Append(buildDocumentOpenRow(mv, open))
		return slot
	}

	// Not downloaded. Per the mockup's attachment-load states: image/video are a
	// hatched 280×115 tile with a centred green ⬇ circle (or the embedded
	// thumbnail with that circle overlaid); audio/document are a compact row
	// with a small ⬇ circle and a two-line label.
	state := mv
	if isVisualKind(mv.Kind) {
		slot.Append(buildAttachmentTile(&state, msg, c, slot, open))
		return slot
	}
	slot.Append(buildMediaRow(&state, msg, c, slot, open))
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
func buildAttachmentTile(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, open func(path string)) gtk.Widgetter {
	// The mockup's undownloaded tile is 280x115 with the download disc centred
	// and the "📷 Photo" label directly beneath it, not pinned to the tile's
	// bottom edge.
	w, h := 280, 115
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
		hatch := gtk.NewBox(gtk.OrientationVertical, 0)
		hatch.AddCSSClass("chatot-media-hatch")
		hatch.SetSizeRequest(w, h)
		overlay.SetChild(hatch)
	}

	center := gtk.NewBox(gtk.OrientationVertical, 7)
	center.SetHAlign(gtk.AlignCenter)
	center.SetVAlign(gtk.AlignCenter)

	circle := mediaDownloadCircle(true)
	circle.SetHAlign(gtk.AlignCenter)
	center.Append(circle)

	caption := gtk.NewLabel(mediaTileLabel(*mv))
	caption.AddCSSClass("chatot-media-caption")
	caption.SetHAlign(gtk.AlignCenter)
	center.Append(caption)

	overlay.AddOverlay(center)
	if mv.IsGIF {
		overlay.AddOverlay(gifBadge())
	}

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) { downloadAndSwap(mv, msg, c, slot, overlay, circle, open) })
	overlay.AddController(click)
	maybeAutoDownload(func() { downloadAndSwap(mv, msg, c, slot, overlay, circle, open) }, mv.Kind, msg.TS)
	return overlay
}

// buildMediaRow is the audio/document not-downloaded state: a small green ⬇
// circle plus a two-line label (noun/filename over "Click to download").
func buildMediaRow(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, open func(path string)) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-media-row")

	circle := mediaDownloadCircle(false)
	circle.SetVAlign(gtk.AlignCenter)
	row.Append(circle)

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetVAlign(gtk.AlignCenter)
	col.SetHExpand(true)
	title := gtk.NewLabel(mediaRowTitleText(*mv))
	title.SetXAlign(0)
	title.SetEllipsize(pango.EllipsizeEnd)
	title.AddCSSClass("chatot-media-row-title")
	sub := gtk.NewLabel(mediaRowMeta(*mv))
	sub.SetXAlign(0)
	sub.AddCSSClass("chatot-media-row-sub")
	col.Append(title)
	col.Append(sub)
	row.Append(col)

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) { downloadAndSwap(mv, msg, c, slot, row, circle, open) })
	row.AddController(click)
	maybeAutoDownload(func() { downloadAndSwap(mv, msg, c, slot, row, circle, open) }, mv.Kind, msg.TS)
	return row
}

// buildDocumentOpenRow is the downloaded-document state (mockup dlReady):
// a 📄 glyph, the filename over a dim "PDF · 1.2 MB" line, and an outlined
// "View" pill that opens the file in the attachment viewer (open), or with
// the desktop's app when there is no viewer to hand it to.
func buildDocumentOpenRow(mv mediaView, open func(path string)) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 9)
	row.AddCSSClass("chatot-media-row")

	icon := gtk.NewLabel(mediaIcon(mv.Kind))
	icon.AddCSSClass("chatot-doc-glyph")
	icon.SetVAlign(gtk.AlignCenter)
	row.Append(icon)

	name := mv.Caption
	if name == "" {
		name = mediaNoun(mv.Kind)
	}
	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetVAlign(gtk.AlignCenter)
	col.SetHExpand(true)
	label := gtk.NewLabel(name)
	label.SetXAlign(0)
	label.SetEllipsize(pango.EllipsizeMiddle)
	label.SetMaxWidthChars(28)
	label.AddCSSClass("chatot-doc-name")
	col.Append(label)
	if meta := mediaOpenMeta(mv); meta != "" {
		sub := gtk.NewLabel(meta)
		sub.SetXAlign(0)
		sub.AddCSSClass("chatot-media-row-sub")
		col.Append(sub)
	}
	row.Append(col)

	view := gtk.NewButtonWithLabel("View")
	view.AddCSSClass("chatot-media-open")
	view.SetVAlign(gtk.AlignCenter)
	view.SetFocusOnClick(false)
	path := mv.LocalPath
	view.ConnectClicked(func() {
		if open != nil {
			open(path)
			return
		}
		openFile(path)
	})
	row.Append(view)
	return row
}

// withMediaOpener gives a downloaded picture a click-to-open, and a video a
// small ⛶ button in its corner that opens the viewer (a click on a GtkVideo
// already toggles playback, so the clip cannot be the click target itself).
func withMediaOpener(widget gtk.Widgetter, mv mediaView, open func(path string)) gtk.Widgetter {
	if open == nil {
		return widget
	}
	if mv.Kind == "image" {
		return openOnClick(widget, mv.LocalPath, open)
	}
	return widget
}

// downloadAndSwap downloads msg's media off the main loop and, on success,
// replaces `current` in slot with the inline widget (image/video/audio) or the
// document "Open" row; on failure it swaps in a click-to-retry state.
func downloadAndSwap(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, current gtk.Widgetter, circle *gtk.Label, open func(path string)) {
	circle.SetText("…")
	go func() {
		path, err := c.DownloadMedia(context.Background(), msg.ID)
		glib.IdleAdd(func() {
			if err != nil {
				slot.Remove(current)
				slot.Append(buildMediaRetry(mv, msg, c, slot, open))
				return
			}
			mv.HasLocal = true
			mv.LocalPath = path
			slot.Remove(current)
			switch {
			case inlineableMedia(*mv):
				w := inlineMediaWidget(*mv, open)
				w = withMediaOpener(w, *mv, open)
				if mv.IsGIF {
					w = withGIFBadge(w)
				}
				slot.Append(w)
			default:
				slot.Append(buildDocumentOpenRow(*mv, open))
			}
		})
	}()
}

// buildMediaRetry is the failed-download state: a red ↻ circle with a
// "Download failed · click to retry" row.
func buildMediaRetry(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, open func(path string)) gtk.Widgetter {
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
	col.SetHExpand(true)
	title := gtk.NewLabel(mediaRowTitleText(*mv))
	title.SetXAlign(0)
	title.SetEllipsize(pango.EllipsizeEnd)
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
			slot.Append(buildAttachmentTile(mv, msg, c, slot, open))
		} else {
			slot.Append(buildMediaRow(mv, msg, c, slot, open))
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
		// The mockup's spent copy — a view-once photo is gone once opened,
		// which "Opened" alone doesn't say.
		return "opened", "No longer available", true
	}
	return "view once", "Click to open · closes after viewing", false
}

// viewOnceNoun labels a view-once bubble by attachment kind; only image and
// video ever carry WhatsApp's viewOnce flag. There is no matching icon
// helper: the mockup's card leads with an outlined "1" disc, not a glyph.
func viewOnceNoun(kind string) string {
	if kind == "video" {
		return "Video"
	}
	return "Photo"
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
	btn := gtk.NewButton()
	btn.SetChild(viewOnceRow(state, viewOnceNoun(state.Kind)+" · "+title, subtitle, false))
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-viewonce-btn")
	btn.ConnectClicked(func() { onViewOnceClicked(&state, msg, c, slot, btn) })
	return btn
}

// viewOnceRow is the mockup's view-once card: a 28px circle *outlined* in the
// accent with a "1" inside (not a filled disc, and not the 📷 glyph), then a
// bold title over a dim subtitle.
func viewOnceRow(mv mediaView, title, subtitle string, spent bool) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-viewonce")
	if spent {
		row.AddCSSClass("chatot-viewonce-spent")
	}

	disc := gtk.NewLabel("1")
	disc.AddCSSClass("chatot-viewonce-disc")
	disc.SetVAlign(gtk.AlignCenter)
	row.Append(disc)

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetVAlign(gtk.AlignCenter)
	col.SetHExpand(true)
	name := gtk.NewLabel(title)
	name.SetXAlign(0)
	name.AddCSSClass("chatot-viewonce-title")
	sub := gtk.NewLabel(subtitle)
	sub.SetXAlign(0)
	sub.AddCSSClass("chatot-viewonce-subtitle")
	col.Append(name)
	col.Append(sub)
	row.Append(col)
	return row
}

// viewOnceTombstone renders the permanent spent placeholder for an opened
// view-once attachment.
func viewOnceTombstone(mv mediaView) gtk.Widgetter {
	_, subtitle, _ := viewOnceRenderState(true, true)
	return viewOnceRow(mv, viewOnceNoun(mv.Kind)+" · opened", subtitle, true)
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

// inlinePhotoSide is the photo bubble's width.
const inlinePhotoSide = 280

func inlineMediaWidget(mv mediaView, open func(path string)) gtk.Widgetter {
	switch mv.Kind {
	case "video":
		return newVideoTile(mv, open)
	case "audio":
		return newVoiceBubble(mv, open)
	case "sticker":
		p := newAsyncPicture(mv.LocalPath, stickerRenderSize*2)
		p.SetCanShrink(true)
		p.SetContentFit(gtk.ContentFitContain)
		p.SetSizeRequest(stickerRenderSize, stickerRenderSize)
		p.AddCSSClass("chatot-sticker")
		return p
	}
	// Decoded at twice the tile so it stays crisp on HiDPI. The tile is
	// as tall as the picture at the bubble width, so a wide screenshot is
	// not letterboxed in a 200px box (the size settles once decoded; the
	// texture is memoised, so a recycled row gets it at once).
	p := gtk.NewPicture()
	p.SetCanShrink(true)
	p.SetContentFit(gtk.ContentFitContain)
	p.SetSizeRequest(inlinePhotoSide, inlinePhotoMinH)
	loadPictureAsync(mv.LocalPath, inlinePhotoSide*2, func(t gdk.Paintabler) {
		p.SetPaintable(t)
		p.SetSizeRequest(inlinePhotoSide, inlinePhotoHeight(t.IntrinsicWidth(), t.IntrinsicHeight()))
	})
	return p
}

// inlinePhotoMinH/MaxH bound the photo bubble's height: a banner-shaped
// screenshot still gets a readable strip, a tall portrait doesn't fill the
// thread.
const (
	inlinePhotoMinH = 80
	inlinePhotoMaxH = 360
)

// inlinePhotoHeight is the bubble height for a w×h picture shown at
// inlinePhotoSide wide, keeping its aspect within the bounds.
func inlinePhotoHeight(w, h int) int {
	if w <= 0 || h <= 0 {
		return 200
	}
	fit := int(float64(inlinePhotoSide)*float64(h)/float64(w) + 0.5)
	if fit < inlinePhotoMinH {
		return inlinePhotoMinH
	}
	if fit > inlinePhotoMaxH {
		return inlinePhotoMaxH
	}
	return fit
}

// videoTileW/H is the mockup's downloaded-video tile: the poster frame with
// the play pill in its bottom-left corner; playback happens in the viewer.
const (
	videoTileW = 280
	videoTileH = 115
)

// newVideoTile is the downloaded-clip bubble content: the poster (the
// sender's embedded frame, else one grabbed from the file) under a pill
// holding a white play disc and the mono length. A click opens the viewer.
func newVideoTile(mv mediaView, open func(path string)) gtk.Widgetter {
	overlay := gtk.NewOverlay()
	overlay.AddCSSClass("chatot-media-tile")
	overlay.SetOverflow(gtk.OverflowHidden)
	overlay.SetSizeRequest(videoTileW, videoTileH)

	setPoster := func(jpeg []byte) bool {
		pixbuf, err := pixbufFromBytes(jpeg)
		if err != nil {
			return false
		}
		overlay.SetChild(coverThumb(pixbuf, videoTileW, videoTileH, 10, false))
		return true
	}
	if !mv.HasThumbnail || !setPoster(mv.Thumbnail) {
		hatch := gtk.NewBox(gtk.OrientationVertical, 0)
		hatch.AddCSSClass("chatot-media-hatch")
		hatch.SetSizeRequest(videoTileW, videoTileH)
		overlay.SetChild(hatch)
		path := mv.LocalPath
		go func() {
			jpeg, err := media.VideoPoster(context.Background(), path)
			if err != nil {
				return
			}
			glib.IdleAdd(func() { setPoster(jpeg) })
		}()
	}

	pill := gtk.NewBox(gtk.OrientationHorizontal, 8)
	pill.AddCSSClass("chatot-video-pill")
	pill.SetHAlign(gtk.AlignStart)
	pill.SetVAlign(gtk.AlignEnd)
	pill.SetMarginStart(8)
	pill.SetMarginBottom(8)
	disc := gtk.NewLabel("▶")
	disc.AddCSSClass("chatot-video-pill-disc")
	disc.SetVAlign(gtk.AlignCenter)
	pill.Append(disc)
	if !mv.IsGIF {
		length := gtk.NewLabel(humanDuration(mv.DurationSecs))
		length.AddCSSClass("chatot-video-pill-len")
		length.SetVAlign(gtk.AlignCenter)
		pill.Append(length)
	}
	overlay.AddOverlay(pill)

	if open == nil {
		return openOnClick(overlay, mv.LocalPath, openFile)
	}
	return openOnClick(overlay, mv.LocalPath, open)
}

// newVoiceBubble is the downloaded voice-note/audio row backed by a player
// that plays in place. An MP3 is transcoded first (the row stays disabled
// meanwhile); if that fails the row turns into a plain Open row.
func newVoiceBubble(mv mediaView, open func(path string)) gtk.Widgetter {
	slot := gtk.NewBox(gtk.OrientationVertical, 0)
	player := sharedVoicePlayer(mv.LocalPath, mv.MimeType, mv.DurationSecs)
	fallback := func() gtk.Widgetter {
		fb := mv
		fb.Caption = mediaNoun(mv.Kind)
		return buildDocumentOpenRow(fb, nil)
	}
	if player.failed != nil {
		slot.Append(fallback())
		return slot
	}
	var onOpen func()
	if open != nil {
		path := mv.LocalPath
		onOpen = func() { open(path) }
	}
	row := newVoiceRow(player, mv.FromMe, onOpen)
	slot.Append(row)
	swapped := false
	player.watchUntilDestroyed(row, func() {
		if player.failed == nil || swapped {
			return
		}
		swapped = true
		slot.Remove(row)
		slot.Append(fallback())
	})
	return slot
}

// attachmentPreview is the one-line stand-in for an attachment wherever a
// message is summarised (reply quotes, notifications, starred rows): the
// same vocabulary as the chat list's preview.
func attachmentPreview(a client.Attachment) string {
	switch a.Kind {
	case "sticker":
		return "🙂 Sticker"
	case "image":
		return "📷 " + firstNonEmptyText(a.Caption, "Photo")
	case "video":
		if a.IsGIF {
			return "🎞 " + firstNonEmptyText(a.Caption, "GIF")
		}
		return "🎥 " + firstNonEmptyText(a.Caption, "Video")
	case "audio":
		if a.DurationSecs > 0 {
			return "🎤 " + humanDuration(a.DurationSecs)
		}
		return "🎤 Voice message"
	case "document":
		return "📄 " + firstNonEmptyText(a.Caption, a.Filename, "Document")
	}
	return "📎 " + firstNonEmptyText(a.Caption, a.Filename, "Attachment")
}

func firstNonEmptyText(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// openURI hands a URL to the desktop's default handler (the browser, for the
// map links on location bubbles).
func openURI(uri string) {
	launcher := gtk.NewURILauncher(uri)
	launcher.Launch(context.Background(), nil, func(res gio.AsyncResulter) {
		_ = launcher.LaunchFinish(res)
	})
}

// openFile launches path with the desktop's default application for it.
// The portal route is tried first; when it fails (no portal, no default
// app registered with it) xdg-open gets a turn, and a failure there is
// logged rather than swallowed so a dead Open button leaves a trace.
func openFile(path string) {
	launcher := gtk.NewFileLauncher(gio.NewFileForPath(path))
	launcher.Launch(context.Background(), nil, func(res gio.AsyncResulter) {
		if err := launcher.LaunchFinish(res); err != nil {
			log.Printf("chatot: open %s via portal: %v; trying xdg-open", path, err)
			go func() {
				if err := exec.Command("xdg-open", path).Run(); err != nil {
					log.Printf("chatot: xdg-open %s: %v", path, err)
				}
			}()
		}
	})
}
