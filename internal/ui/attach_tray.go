package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// trayItem is one queued attachment: the file plus the caption typed for it.
// Captions are per-item, like the mockup's 💬 marker on a thumbnail.
type trayItem struct {
	Path    string
	Caption string
	// Preview is what the background helpers learned about the file (poster
	// or page image, duration, size); the send uses it as the message's
	// embedded thumbnail and metadata.
	Preview trayPreview
}

// traySendLabel is the header button's text: plain "Send" for a single file,
// "Send N" once more than one is queued, matching the mockup.
func traySendLabel(n int) string {
	if n > 1 {
		return fmt.Sprintf("Send %d", n)
	}
	return "Send"
}

// trayMeta is the mono subtitle under the filename: the type and size the
// mockup shows, with each segment dropping out when unknown.
func trayMeta(path string) string {
	kind := strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
	var size string
	if info, err := os.Stat(path); err == nil {
		size = humanSize(info.Size())
	}
	return joinMeta(kind, size)
}

// trayPreviewable reports whether a queued file can be shown as a picture in
// the big preview slot; everything else gets the "No preview available" card.
func trayPreviewable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".avif", ".svg", ".svgz":
		return true
	}
	return false
}

// trayTileSize is the strip's square thumbnail, per the mockup.
const trayTileSize = 54

// trayGlyph is the placeholder icon for a file with no picture preview,
// chosen by extension the way the mockup picks its tray icon.
func trayGlyph(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mkv", ".webm", ".mov":
		return "🎬"
	case ".mp3", ".ogg", ".opus", ".wav", ".flac", ".m4a":
		return "🎵"
	case ".pdf":
		return "📕"
	case ".zip", ".tar", ".gz", ".xz", ".7z":
		return "🗜"
	}
	return "📄"
}

// AttachTray is the mockup's full-pane send preview: a Cancel/name/Send
// header, a large preview of the selected file, a caption entry, and a
// thumbnail strip with per-item ✕ and a dashed ＋.
//
// It replaces sending straight from the file chooser, which gave no chance to
// review a pick, caption it, or queue a second file.
type AttachTray struct {
	*gtk.Box

	items    []trayItem
	selected int

	title    *gtk.Label
	meta     *gtk.Label
	sendBtn  *gtk.Button
	preview  *gtk.Box
	caption  *gtk.Entry
	strip    *gtk.Box
	onSend   func([]trayItem)
	onAddReq func()
	// previews caches the helper results per path for the tray's lifetime,
	// so redraws (every caption keystroke rebuilds the strip) never rerun
	// ffmpeg or pdftoppm. An entry with Done=false is still being built.
	previews map[string]trayPreview
}

// NewAttachTray builds the tray hidden. onSend receives the queued items when
// the user confirms; onAddReq is the ＋ button, which reopens the file chooser.
func NewAttachTray(onSend func([]trayItem), onAddReq func()) *AttachTray {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-tray")
	root.SetVisible(false)

	t := &AttachTray{Box: root, onSend: onSend, onAddReq: onAddReq, previews: map[string]trayPreview{}}

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	header.AddCSSClass("chatot-tray-header")

	cancel := gtk.NewButtonWithLabel("Cancel")
	cancel.AddCSSClass("flat")
	cancel.AddCSSClass("chatot-tray-cancel")
	// Centred, not filled: a GtkBox stretches its children to the row's
	// 47px otherwise, which turned both buttons into full-height slabs.
	cancel.SetVAlign(gtk.AlignCenter)
	cancel.ConnectClicked(t.Close)
	header.Append(cancel)

	titleCol := gtk.NewBox(gtk.OrientationVertical, 0)
	titleCol.SetHExpand(true)
	titleCol.SetHAlign(gtk.AlignCenter)
	titleCol.SetVAlign(gtk.AlignCenter)
	t.title = gtk.NewLabel("")
	t.title.AddCSSClass("chatot-tray-name")
	t.title.SetEllipsize(pango.EllipsizeMiddle)
	t.meta = gtk.NewLabel("")
	t.meta.AddCSSClass("chatot-tray-meta")
	titleCol.Append(t.title)
	titleCol.Append(t.meta)
	header.Append(titleCol)

	t.sendBtn = gtk.NewButtonWithLabel("Send")
	t.sendBtn.AddCSSClass("chatot-tray-send")
	t.sendBtn.SetVAlign(gtk.AlignCenter)
	t.sendBtn.ConnectClicked(t.send)
	header.Append(t.sendBtn)
	root.Append(header)

	t.preview = gtk.NewBox(gtk.OrientationVertical, 0)
	t.preview.AddCSSClass("chatot-tray-stage")
	t.preview.SetVExpand(true)
	t.preview.SetHAlign(gtk.AlignCenter)
	t.preview.SetVAlign(gtk.AlignCenter)
	root.Append(t.preview)

	captionRow := gtk.NewBox(gtk.OrientationHorizontal, 0)
	captionRow.AddCSSClass("chatot-tray-captionrow")
	t.caption = gtk.NewEntry()
	t.caption.SetPlaceholderText("Caption")
	t.caption.AddCSSClass("chatot-tray-caption")
	t.caption.SetSizeRequest(460, -1)
	// HExpand as well as HAlign: a GtkBox gives an unexpanded child only its
	// natural width and packs it at the start, so the entry would sit
	// left-of-centre with the alignment alone.
	t.caption.SetHExpand(true)
	t.caption.SetHAlign(gtk.AlignCenter)
	// Store the caption as it is typed rather than only on send, so switching
	// thumbnails doesn't silently drop what was written for the current one.
	t.caption.ConnectChanged(func() {
		if t.selected >= 0 && t.selected < len(t.items) {
			t.items[t.selected].Caption = t.caption.Text()
			t.refreshStrip()
		}
	})
	t.caption.ConnectActivate(t.send)
	captionRow.Append(t.caption)
	root.Append(captionRow)

	// Outer box carries the full-bleed strip background and hairline; the
	// inner one holds the centred tiles.
	stripRow := gtk.NewBox(gtk.OrientationHorizontal, 0)
	stripRow.AddCSSClass("chatot-tray-strip")
	t.strip = gtk.NewBox(gtk.OrientationHorizontal, 8)
	t.strip.SetHExpand(true)
	t.strip.SetHAlign(gtk.AlignCenter)
	stripRow.Append(t.strip)
	root.Append(stripRow)

	return t
}

// Open queues paths and shows the tray. Called with an already-open tray it
// appends, which is what the ＋ button does.
func (t *AttachTray) Open(paths []string) {
	for _, p := range paths {
		t.items = append(t.items, trayItem{Path: p})
		t.ensurePreview(p)
	}
	if len(t.items) == 0 {
		return
	}
	if t.selected >= len(t.items) {
		t.selected = len(t.items) - 1
	}
	t.SetVisible(true)
	t.refresh()
}

// Close discards every queued item and hides the tray.
func (t *AttachTray) Close() {
	t.items = nil
	t.selected = 0
	t.previews = map[string]trayPreview{}
	t.SetVisible(false)
}

// ensurePreview starts the helpers for path unless they already ran (or are
// running). The result lands on the main loop and redraws the tray if it
// is still showing that file.
func (t *AttachTray) ensurePreview(path string) {
	if _, ok := t.previews[path]; ok {
		return
	}
	t.previews[path] = trayPreview{}
	go func() {
		p := buildTrayPreview(path)
		glib.IdleAdd(func() {
			if _, ok := t.previews[path]; !ok {
				return // the tray was closed meanwhile
			}
			t.previews[path] = p
			if t.Visible() {
				t.refresh()
			}
		})
	}()
}

// send hands the queue to the callback and closes.
func (t *AttachTray) send() {
	if len(t.items) == 0 {
		return
	}
	items := t.items
	for i := range items {
		items[i].Preview = t.previews[items[i].Path]
	}
	t.Close()
	if t.onSend != nil {
		t.onSend(items)
	}
}

// remove drops the item at i, closing the tray when it was the last one and
// redrawing otherwise.
func (t *AttachTray) remove(i int) {
	if !t.removeAt(i) {
		return
	}
	if len(t.items) == 0 {
		t.Close()
		return
	}
	t.refresh()
}

// removeAt is remove's pure half: it drops the item at i and keeps `selected`
// pointing at the same file. Split out so the index bookkeeping is testable
// without a display. Reports whether anything was removed.
func (t *AttachTray) removeAt(i int) bool {
	if i < 0 || i >= len(t.items) {
		return false
	}
	t.items = append(t.items[:i], t.items[i+1:]...)
	// Removing an item BEFORE the selected one shifts everything after it down
	// by one, so the index has to follow. Clamping alone would silently move
	// the preview to a different file than the one the user had open.
	if i < t.selected {
		t.selected--
	}
	if t.selected >= len(t.items) {
		t.selected = len(t.items) - 1
	}
	if t.selected < 0 {
		t.selected = 0
	}
	return true
}

// refresh redraws the header, preview, caption and strip for the current
// selection.
func (t *AttachTray) refresh() {
	if t.selected < 0 || t.selected >= len(t.items) {
		return
	}
	item := t.items[t.selected]
	t.title.SetLabel(filepath.Base(item.Path))
	t.meta.SetLabel(trayMeta(item.Path))
	t.sendBtn.SetLabel(traySendLabel(len(t.items)))

	// SetText re-enters ConnectChanged, which would write the caption back
	// onto whichever item is selected — harmless here because the selection is
	// already the one being loaded, but the order matters.
	t.caption.SetText(item.Caption)

	removeAllChildren(t.preview)
	t.preview.Append(t.newTrayPreview(item.Path))
	t.refreshStrip()
}

// trayStageW/H is the mockup's preview slot: a 4:3 card at most 460px wide.
const (
	trayStageW = 460
	trayStageH = 345
)

// newTrayPreview builds the big centre slot for path: the picture itself
// for an image, a player for a video (poster first) or an audio file, the
// rendered first page for a PDF, and otherwise the mockup's "No preview
// available" card. The mockup only ever previews pictures; the players and
// the page render are chatot's additions on top of it.
func (t *AttachTray) newTrayPreview(path string) gtk.Widgetter {
	p := t.previews[path]
	switch trayKindOf(path) {
	case trayImage:
		pic := gtk.NewPictureForFilename(path)
		return trayStage(pic)
	case trayVideo:
		return t.videoStage(path, p)
	case trayAudio:
		return audioStage(path, p)
	case trayPDF:
		if len(p.Image) > 0 {
			if pixbuf, err := pixbufFromBytes(p.Image); err == nil {
				pic := gtk.NewPictureForPaintable(gdk.NewTextureForPixbuf(pixbuf))
				return trayStage(pic)
			}
		}
	}
	return trayNoPreview(path, p)
}

// trayStage wraps a picture in the tinted 4:3 card the mockup's stage is,
// so a small picture reads as a preview rather than as a stray icon
// floating in the middle of an empty pane.
func trayStage(pic *gtk.Picture) gtk.Widgetter {
	pic.SetCanShrink(true)
	// ScaleDown, not Contain: Contain happily blows a 32px icon up to the
	// full 460px stage. The preview should never upscale past 1:1.
	pic.SetContentFit(gtk.ContentFitScaleDown)
	pic.SetHExpand(true)
	pic.SetVExpand(true)

	stage := gtk.NewBox(gtk.OrientationVertical, 0)
	stage.AddCSSClass("chatot-tray-image")
	stage.SetSizeRequest(trayStageW, trayStageH)
	stage.Append(pic)
	return stage
}

// videoStage is a GtkVideo in the stage card with the clip's poster frame
// laid over it until playback starts: GtkVideo paints black until the
// first frame is decoded, and a black card looked like a broken preview.
// A clip the media backend cannot decode keeps the poster and says so.
func (t *AttachTray) videoStage(path string, p trayPreview) gtk.Widgetter {
	player := newMediaPlayer(path, p.Seconds)
	view := newVideoStage(player, p.Image)
	view.SetHExpand(true)
	view.SetVExpand(true)
	if player.stream != nil {
		player.stream.NotifyProperty("error", func() {
			if err := player.stream.Error(); err != nil {
				note := gtk.NewLabel("Can't play here · " + err.Error())
				note.AddCSSClass("chatot-tray-meta")
				note.SetHAlign(gtk.AlignCenter)
				note.SetVAlign(gtk.AlignEnd)
				note.SetMarginBottom(8)
				view.AddOverlay(note)
			}
		})
	}
	// A click on the frame toggles playback, like the viewer's Space.
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) { player.Toggle() })
	view.AddController(click)

	stage := gtk.NewBox(gtk.OrientationVertical, 0)
	stage.AddCSSClass("chatot-tray-image")
	stage.SetSizeRequest(trayStageW, trayStageH)
	stage.Append(view)
	stage.Append(newTransportBar(player, nil))
	return stage
}

// audioStage is the audio card: the 🎵 glyph, the file's type, size and
// length, and a transport so the file can be listened to before sending.
func audioStage(path string, p trayPreview) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationVertical, 8)
	card.AddCSSClass("chatot-tray-nopreview")
	card.SetSizeRequest(360, 270)
	card.SetVAlign(gtk.AlignCenter)

	glyph := gtk.NewLabel("🎵")
	glyph.AddCSSClass("chatot-tray-glyph")
	glyph.SetVExpand(true)
	glyph.SetVAlign(gtk.AlignEnd)
	card.Append(glyph)

	meta := gtk.NewLabel(trayMetaWithDuration(path, p.Seconds))
	meta.AddCSSClass("chatot-tray-meta")
	card.Append(meta)

	// The same transport the viewer uses; an MP3 is transcoded first (the
	// bar stays disabled until then) since GTK's backend cannot play one.
	player := newPendingPlayer(p.Seconds)
	bar := newTransportBar(player, nil)
	gtk.BaseWidget(bar).AddCSSClass("chatot-tray-audio")
	gtk.BaseWidget(bar).SetSizeRequest(320, -1)
	gtk.BaseWidget(bar).SetHAlign(gtk.AlignCenter)
	gtk.BaseWidget(bar).SetVExpand(true)
	gtk.BaseWidget(bar).SetVAlign(gtk.AlignStart)
	gtk.BaseWidget(bar).SetMarginTop(6)
	// The stage is rebuilt on every selection change; the old one stops.
	gtk.BaseWidget(bar).ConnectUnrealize(player.Pause)
	card.Append(bar)
	preparePlayable(player, path, "", func(err error) {
		note := gtk.NewLabel("Can't play here · " + err.Error())
		note.AddCSSClass("chatot-tray-meta")
		note.SetWrap(true)
		card.Append(note)
	})
	return card
}

// trayNoPreview is the mockup's "No preview available" card.
func trayNoPreview(path string, p trayPreview) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationVertical, 8)
	card.AddCSSClass("chatot-tray-nopreview")
	card.SetSizeRequest(360, 270)
	card.SetVAlign(gtk.AlignCenter)

	glyph := gtk.NewLabel(trayGlyph(path))
	glyph.AddCSSClass("chatot-tray-glyph")
	glyph.SetVExpand(true)
	glyph.SetVAlign(gtk.AlignEnd)
	card.Append(glyph)

	label := gtk.NewLabel("No preview available")
	label.AddCSSClass("chatot-tray-noprev-label")
	card.Append(label)

	meta := gtk.NewLabel(trayMetaWithDuration(path, p.Seconds))
	meta.AddCSSClass("chatot-tray-meta")
	meta.SetVExpand(true)
	meta.SetVAlign(gtk.AlignStart)
	card.Append(meta)
	return card
}

// trayMetaWithDuration is trayMeta plus a "m:ss" length when known.
func trayMetaWithDuration(path string, seconds int) string {
	if seconds <= 0 {
		return trayMeta(path)
	}
	return joinMeta(trayMeta(path), humanDuration(seconds))
}

// refreshStrip rebuilds the thumbnail row: one 54px tile per queued file with
// its own ✕, then the dashed ＋.
func (t *AttachTray) refreshStrip() {
	removeAllChildren(t.strip)
	for i, item := range t.items {
		idx := i
		tile := gtk.NewButton()
		// An image thumbnails itself; anything else falls back to its glyph.
		// The thumbnail is drawn into a fixed 54px square rather than shown
		// through a GtkPicture, whose natural size is the image's own pixels
		// and which therefore blew a photo up to fill the strip.
		if thumb := t.newTrayThumb(item.Path, trayTileSize); thumb != nil {
			tile.SetChild(thumb)
		} else {
			tile.SetChild(gtk.NewLabel(trayGlyph(item.Path)))
		}
		tile.AddCSSClass("chatot-tray-tile")
		if i == t.selected {
			tile.AddCSSClass("chatot-tray-tile-on")
		}
		tile.SetSizeRequest(54, 54)
		tile.ConnectClicked(func() {
			t.selected = idx
			t.refresh()
		})

		// The ✕ overhangs the tile's corner, so the tile and its remove button
		// share an overlay rather than sitting in a row.
		stack := gtk.NewOverlay()
		stack.SetChild(tile)

		del := gtk.NewButtonWithLabel("✕")
		del.AddCSSClass("chatot-tray-remove")
		del.SetHAlign(gtk.AlignEnd)
		del.SetVAlign(gtk.AlignStart)
		del.ConnectClicked(func() { t.remove(idx) })
		stack.AddOverlay(del)

		if item.Caption != "" {
			marker := gtk.NewLabel("💬")
			marker.AddCSSClass("chatot-tray-captioned")
			marker.SetHAlign(gtk.AlignStart)
			marker.SetVAlign(gtk.AlignEnd)
			stack.AddOverlay(marker)
		}
		t.strip.Append(stack)
	}

	add := gtk.NewButtonWithLabel("＋")
	add.AddCSSClass("chatot-tray-add")
	add.SetSizeRequest(54, 54)
	add.SetTooltipText("Add files")
	add.ConnectClicked(func() {
		if t.onAddReq != nil {
			t.onAddReq()
		}
	})
	t.strip.Append(add)
}
