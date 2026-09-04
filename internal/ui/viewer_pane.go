package ui

import (
	"context"
	"log"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/graphene"
	"github.com/diamondburned/gotk4/pkg/gsk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
	"chatot/internal/media"
)

// viewerKind classifies a message for the attachment viewer the way the
// design's attKind does: one vocabulary covers photos, clips, GIFs, voice
// notes, PDFs, other files and locations. "" is not viewable (text,
// stickers, view-once media, polls…).
func viewerKind(m client.Message) string {
	if m.Deleted {
		return ""
	}
	if m.Location != nil {
		return "location"
	}
	a := m.Attachment
	if a == nil || a.ViewOnce {
		return ""
	}
	switch a.Kind {
	case "image":
		return "photo"
	case "video":
		if a.IsGIF {
			return "gif"
		}
		return "video"
	case "audio":
		return "audio"
	case "document":
		if strings.EqualFold(filepath.Ext(a.Filename), ".pdf") || strings.Contains(a.MimeType, "pdf") {
			return "pdf"
		}
		return "file"
	}
	return ""
}

// viewerZoomSteps are the design's ZOOM_STEPS: multiples of the fitted
// size, index 0 being "Fit".
var viewerZoomSteps = []float64{1, 1.5, 2, 3, 5}

// viewerMapZoom bounds the location stage's zoom (the mockup's 16..18 band,
// widened a little either way).
const (
	viewerMapZoomMin = 14
	viewerMapZoomMax = 19
	viewerPDFRender  = 1600 // longer side, in px, a PDF page is rasterised at
	viewerThumb      = 52   // filmstrip tile
	viewerDetailsW   = 244
)

// AttachmentViewer is the content pane's attachment viewer (the design's
// pane === 'viewer'): a header with the attachment's identity and actions,
// a stage per kind, a zoom bar or transport under it, the caption, a
// filmstrip of every attachment in the chat, and an optional details
// sidebar. It replaces the standalone photo/video windows.
type AttachmentViewer struct {
	*gtk.Box

	c      client.Client
	window *gtk.Window
	toasts *adw.ToastOverlay
	onBack func()
	// onForward/onReply/onStar act on the shown message; onMenu supplies
	// the ⋯ menu (the bubble's own).
	onForward    func(client.Message)
	onReply      func(client.Message)
	onStar       func(client.Message)
	onMenu       func(client.Message) []menuItem
	onDownloaded func(msgID, path string)
	// wheel is the current picture scroller's zoom-at-point, for WheelZoom.
	wheel func(factor, x, y float64)

	chat  client.Chat
	items []client.Message
	idx   int

	// header
	glyph      *gtk.Label
	title      *gtk.Label
	sub        *gtk.Label
	counter    *gtk.Label
	starBtn    *gtk.Button
	infoBtn    *gtk.Button
	primaryBtn *gtk.Button
	menuBtn    *gtk.Button

	stage     *gtk.Overlay // the current stage widget sits as its child
	stageBox  *gtk.Box     // dark/light surface behind it
	prevBtn   *gtk.Button
	nextBtn   *gtk.Button
	navRings  []*gtk.DrawingArea // the ‹ › buttons' rings, redrawn when the stage scheme flips
	bottom    *gtk.Box           // zoom bar / transport / caption
	strip     *gtk.Box
	stripLbl  *gtk.Label
	details   *gtk.Box
	detailsOn bool

	// per-item state
	player   *mediaPlayer
	zoom     float64 // multiple of the fitted size; 1 = Fit
	zoomLbl  *gtk.Label
	fitBtn   *gtk.Button
	pic      *gtk.Picture // photo/pdf page
	picTex   *gdk.Texture // what pic shows at full size (see applyZoom)
	anchor   zoomAnchor   // pending wheel-zoom scroll target (see zoomAt)
	picW     int
	picH     int
	page     int
	pages    int
	pageLbl  *gtk.Label
	pdfCache map[int]*gdk.Texture
	mapZoom  int
	mapView  *mapView
	mapLbl   *gtk.Label
	loading  bool
	// gen invalidates async work (downloads, page renders) from a previous
	// item once the user has moved on.
	gen int
}

// NewAttachmentViewer builds the pane. onBack returns to the chat.
func NewAttachmentViewer(c client.Client, onBack func()) *AttachmentViewer {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-viewer")
	root.SetFocusable(true)
	v := &AttachmentViewer{Box: root, c: c, onBack: onBack, pdfCache: map[int]*gdk.Texture{}}

	root.Append(v.buildHeader())

	body := gtk.NewBox(gtk.OrientationHorizontal, 0)
	body.SetVExpand(true)
	root.Append(body)

	main := gtk.NewBox(gtk.OrientationVertical, 0)
	main.SetHExpand(true)
	main.SetVExpand(true)
	body.Append(main)

	v.stageBox = gtk.NewBox(gtk.OrientationVertical, 0)
	v.stageBox.AddCSSClass("chatot-viewer-stage")
	v.stageBox.SetVExpand(true)
	v.stageBox.SetHExpand(true)
	v.stage = gtk.NewOverlay()
	v.stage.SetVExpand(true)
	v.stage.SetHExpand(true)
	v.stageBox.Append(v.stage)
	main.Append(v.stageBox)

	v.prevBtn = v.navButton("‹", "Previous · ←", func() { v.step(-1) })
	v.prevBtn.SetHAlign(gtk.AlignStart)
	v.nextBtn = v.navButton("›", "Next · →", func() { v.step(1) })
	v.nextBtn.SetHAlign(gtk.AlignEnd)
	v.stage.AddOverlay(v.prevBtn)
	v.stage.AddOverlay(v.nextBtn)

	v.bottom = gtk.NewBox(gtk.OrientationVertical, 0)
	main.Append(v.bottom)

	stripRow := gtk.NewBox(gtk.OrientationHorizontal, 7)
	stripRow.AddCSSClass("chatot-viewer-strip")
	stripScroller := gtk.NewScrolledWindow()
	stripScroller.SetPolicy(gtk.PolicyExternal, gtk.PolicyNever)
	stripScroller.SetPropagateNaturalWidth(false)
	stripScroller.SetHExpand(true)
	v.strip = gtk.NewBox(gtk.OrientationHorizontal, 7)
	stripScroller.SetChild(v.strip)
	stripRow.Append(stripScroller)
	v.stripLbl = gtk.NewLabel("")
	v.stripLbl.AddCSSClass("chatot-viewer-strip-label")
	stripRow.Append(v.stripLbl)
	main.Append(stripRow)

	v.details = gtk.NewBox(gtk.OrientationVertical, 0)
	v.details.AddCSSClass("chatot-viewer-details")
	v.details.SetSizeRequest(viewerDetailsW, -1)
	v.details.SetVisible(false)
	body.Append(v.details)

	keys := gtk.NewEventControllerKey()
	keys.ConnectKeyPressed(func(keyval, _ uint, _ gdk.ModifierType) bool { return v.onKey(keyval) })
	root.AddController(keys)

	return v
}

// SetWindow sets the parent for dialogs; SetToastOverlay where toasts go.
func (v *AttachmentViewer) SetWindow(w *gtk.Window)             { v.window = w }
func (v *AttachmentViewer) SetToastOverlay(t *adw.ToastOverlay) { v.toasts = t }

// OnForward/OnReply/OnStar/OnMenu wire the header's actions.
func (v *AttachmentViewer) OnForward(f func(client.Message))         { v.onForward = f }
func (v *AttachmentViewer) OnReply(f func(client.Message))           { v.onReply = f }
func (v *AttachmentViewer) OnStar(f func(client.Message))            { v.onStar = f }
func (v *AttachmentViewer) OnMenu(f func(client.Message) []menuItem) { v.onMenu = f }

// OnDownloaded fires after the viewer fetches an attachment, with the
// message ID and the file's path, so the thread can show it too.
func (v *AttachmentViewer) OnDownloaded(f func(msgID, path string)) { v.onDownloaded = f }

// Open shows msgID among the viewable messages of chat (msgs is the loaded
// thread; every attachment in it navigates as one sequence).
func (v *AttachmentViewer) Open(chat client.Chat, msgs []client.Message, msgID string) {
	v.chat = chat
	v.items = v.items[:0]
	v.idx = 0
	for _, m := range msgs {
		if viewerKind(m) == "" {
			continue
		}
		if m.ID == msgID {
			v.idx = len(v.items)
		}
		v.items = append(v.items, m)
	}
	v.detailsOn = false
	v.details.SetVisible(false)
	v.infoBtn.RemoveCSSClass("chatot-viewer-hbtn-on")
	v.show(v.idx)
	v.GrabFocus()
}

// Close pauses playback when the pane is left.
func (v *AttachmentViewer) Close() {
	if v.player != nil {
		v.player.Pause()
	}
}

// Current is the message on show.
func (v *AttachmentViewer) Current() (client.Message, bool) {
	if v.idx < 0 || v.idx >= len(v.items) {
		return client.Message{}, false
	}
	return v.items[v.idx], true
}

// ---- header -------------------------------------------------------------

func (v *AttachmentViewer) buildHeader() gtk.Widgetter {
	h := gtk.NewBox(gtk.OrientationHorizontal, 9)
	h.AddCSSClass("chatot-viewer-header")

	back := v.headerButton("←", "Back to chat · Esc", v.back)
	h.Append(back)

	v.glyph = gtk.NewLabel("🖼")
	v.glyph.AddCSSClass("chatot-viewer-glyph")
	v.glyph.SetVAlign(gtk.AlignCenter)
	h.Append(v.glyph)

	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	v.title = gtk.NewLabel("")
	v.title.AddCSSClass("chatot-viewer-title")
	v.title.SetXAlign(0)
	v.title.SetEllipsize(pango.EllipsizeEnd)
	col.Append(v.title)
	v.sub = gtk.NewLabel("")
	v.sub.AddCSSClass("chatot-viewer-sub")
	v.sub.SetXAlign(0)
	v.sub.SetEllipsize(pango.EllipsizeEnd)
	col.Append(v.sub)
	h.Append(col)

	v.counter = gtk.NewLabel("")
	v.counter.AddCSSClass("chatot-viewer-counter")
	h.Append(v.counter)

	sep := gtk.NewBox(gtk.OrientationVertical, 0)
	sep.AddCSSClass("chatot-viewer-hsep")
	sep.SetSizeRequest(1, 20)
	sep.SetVAlign(gtk.AlignCenter)
	h.Append(sep)

	h.Append(v.headerButton("↩", "Reply in chat", func() {
		if m, ok := v.Current(); ok && v.onReply != nil {
			v.back()
			v.onReply(m)
		}
	}))
	h.Append(v.headerButton("↪", "Forward", func() {
		if m, ok := v.Current(); ok && v.onForward != nil {
			v.onForward(m)
		}
	}))
	v.starBtn = v.headerButton("☆", "Star", func() {
		if m, ok := v.Current(); ok && v.onStar != nil {
			v.onStar(m)
			m.Starred = !m.Starred
			v.items[v.idx] = m
			v.refreshStar()
			if m.Starred {
				showToast(v.toasts, "Attachment starred")
			} else {
				showToast(v.toasts, "Unstarred")
			}
		}
	})
	h.Append(v.starBtn)
	v.infoBtn = v.headerButton("ℹ", "Details", v.toggleDetails)
	h.Append(v.infoBtn)
	v.menuBtn = v.headerButton("⋯", "More", func() {
		if m, ok := v.Current(); ok && v.onMenu != nil {
			popupMenuBelow(v.menuBtn, v.onMenu(m))
		}
	})
	h.Append(v.menuBtn)

	v.primaryBtn = gtk.NewButtonWithLabel("Save…")
	v.primaryBtn.AddCSSClass("chatot-viewer-primary")
	v.primaryBtn.SetVAlign(gtk.AlignCenter)
	v.primaryBtn.SetFocusOnClick(false)
	v.primaryBtn.ConnectClicked(v.primary)
	h.Append(v.primaryBtn)
	return h
}

func (v *AttachmentViewer) headerButton(glyph, tip string, onClick func()) *gtk.Button {
	b := gtk.NewButtonWithLabel(glyph)
	b.AddCSSClass("flat")
	b.RemoveCSSClass("text-button")
	b.AddCSSClass("chatot-viewer-hbtn")
	b.SetTooltipText(tip)
	b.SetVAlign(gtk.AlignCenter)
	b.SetFocusOnClick(false)
	b.ConnectClicked(onClick)
	return b
}

// navRingWidth is the ‹ › buttons' ring stroke. The mockup's is a 1px
// hairline; drawn by GTK (border or inset shadow) a ring that thin breaks
// into four bright points at a fractional display scale, so it is stroked
// with cairo, a little wider, at the mockup's alpha.
const navRingWidth = 2.0

func (v *AttachmentViewer) navButton(glyph, tip string, onClick func()) *gtk.Button {
	b := gtk.NewButton()
	b.AddCSSClass("chatot-viewer-nav")
	ring := gtk.NewDrawingArea()
	ring.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		c := area.Color()
		alpha := 0.15
		if v.stageBox.HasCSSClass("chatot-viewer-stage-dark") {
			alpha = 0.22
		}
		r := math.Min(float64(w), float64(h))/2 - navRingWidth/2
		cr.Arc(float64(w)/2, float64(h)/2, r, 0, 2*math.Pi)
		cr.SetSourceRGBA(float64(c.Red()), float64(c.Green()), float64(c.Blue()), alpha)
		cr.SetLineWidth(navRingWidth)
		cr.Stroke()
	})
	v.navRings = append(v.navRings, ring)
	label := gtk.NewLabel(glyph)
	face := gtk.NewOverlay()
	face.SetChild(ring)
	face.AddOverlay(label)
	b.SetChild(face)
	b.SetTooltipText(tip)
	b.SetVAlign(gtk.AlignCenter)
	b.SetMarginStart(14)
	b.SetMarginEnd(14)
	b.SetFocusOnClick(false)
	b.ConnectClicked(onClick)
	return b
}

func (v *AttachmentViewer) back() {
	v.Close()
	if v.onBack != nil {
		v.onBack()
	}
}

func (v *AttachmentViewer) step(d int) {
	n := v.idx + d
	if n < 0 || n >= len(v.items) {
		return
	}
	v.show(n)
}

func (v *AttachmentViewer) onKey(keyval uint) bool {
	switch keyval {
	case gdk.KEY_Escape:
		v.back()
	case gdk.KEY_Left:
		v.step(-1)
	case gdk.KEY_Right:
		v.step(1)
	case gdk.KEY_plus, gdk.KEY_KP_Add, gdk.KEY_equal:
		v.zoomBy(1)
	case gdk.KEY_minus, gdk.KEY_KP_Subtract:
		v.zoomBy(-1)
	case gdk.KEY_0, gdk.KEY_KP_0:
		v.setZoom(1)
	case gdk.KEY_space:
		if v.player != nil {
			v.player.Toggle()
		}
	case gdk.KEY_f, gdk.KEY_F11:
		v.fullscreen()
	default:
		return false
	}
	return true
}

// ---- per-item rendering -------------------------------------------------

// viewerFrom is who sent the attachment, as the header and details name
// them: "You", the group sender, or the chat itself.
func (v *AttachmentViewer) from(m client.Message) string {
	if m.FromMe {
		return "You"
	}
	if v.chat.IsGroup && m.FromJID != "" {
		if n := v.c.ContactName(m.FromJID); n != "" {
			return n
		}
		return bareJIDUser(m.FromJID)
	}
	if v.chat.Name != "" {
		return v.chat.Name
	}
	return bareJIDUser(m.ChatJID)
}

// sentAt is "Today at 15:38" / "02/01/2026 at 09:14".
func sentAt(ts int64, now time.Time) string {
	return dayText(ts, now) + " at " + time.Unix(ts, 0).In(now.Location()).Format("15:04")
}

// viewerTitle names the attachment for the header.
func viewerTitle(m client.Message, kind string) string {
	switch kind {
	case "location":
		if m.Location.Name != "" {
			return m.Location.Name
		}
		if m.Location.IsLive {
			return "Live location"
		}
		return "Shared location"
	case "audio":
		return "Voice message"
	case "video":
		return "Video"
	case "gif":
		return "Animated GIF"
	case "photo":
		return "Photo"
	}
	if m.Attachment.Filename != "" {
		return m.Attachment.Filename
	}
	return "Document"
}

// viewerKindLabel is the noun the fetch state and details use.
func viewerKindLabel(m client.Message, kind string) string {
	switch kind {
	case "photo":
		return "Photo"
	case "video":
		return "Video"
	case "gif":
		return "GIF"
	case "pdf":
		return "PDF document"
	case "audio":
		return "Voice message"
	case "location":
		return "Location"
	}
	if t := docTypeLabel(m.Attachment.MimeType, m.Attachment.Filename); t != "" {
		return t + " file"
	}
	return "File"
}

func viewerGlyph(kind string) string {
	switch kind {
	case "video":
		return "🎥"
	case "gif":
		return "🎞"
	case "pdf", "file":
		return "📄"
	case "audio":
		return "🎤"
	case "location":
		return "📍"
	}
	return "🖼"
}

// hasLocal reports whether the attachment's file is on disk.
func hasLocal(m client.Message) (string, bool) {
	if m.Attachment == nil || m.Attachment.LocalPath == "" {
		return "", false
	}
	mv := mediaVM(m)
	return mv.LocalPath, mv.HasLocal
}

func (v *AttachmentViewer) show(i int) {
	if i < 0 || i >= len(v.items) {
		return
	}
	v.gen++
	if v.player != nil {
		v.player.Pause()
		v.player = nil
	}
	v.idx = i
	m := v.items[i]
	kind := viewerKind(m)
	now := time.Now()
	v.zoom = 1
	v.page = 1
	v.pages = 0
	v.pdfCache = map[int]*gdk.Texture{}
	v.mapZoom = 17
	v.pic = nil
	v.anchor = zoomAnchor{}
	v.mapView = nil
	v.loading = false

	// header
	v.glyph.SetLabel(viewerGlyph(kind))
	v.title.SetLabel(viewerTitle(m, kind))
	v.sub.SetLabel(v.subline(m, kind, now))
	if len(v.items) > 1 {
		v.counter.SetLabel(strconv.Itoa(i+1) + " / " + strconv.Itoa(len(v.items)))
	} else {
		v.counter.SetLabel("")
	}
	v.refreshStar()
	v.prevBtn.SetVisible(i > 0)
	v.nextBtn.SetVisible(i < len(v.items)-1)

	dark := kind == "photo" || kind == "video" || kind == "gif"
	if dark {
		v.stageBox.AddCSSClass("chatot-viewer-stage-dark")
	} else {
		v.stageBox.RemoveCSSClass("chatot-viewer-stage-dark")
	}
	for _, ring := range v.navRings {
		ring.QueueDraw()
	}

	// primary action
	path, local := hasLocal(m)
	switch {
	case kind == "location":
		v.primaryBtn.SetLabel("Open in Maps")
	case !local:
		v.primaryBtn.SetLabel("Download")
	case kind == "pdf" || kind == "file":
		v.primaryBtn.SetLabel("Open")
	default:
		v.primaryBtn.SetLabel("Save…")
	}

	removeAllChildren(v.bottom)
	switch {
	case kind == "location":
		v.stage.SetChild(v.locationStage(m))
	case !local:
		v.stage.SetChild(v.fetchStage(m, kind))
	case kind == "photo":
		v.stage.SetChild(v.photoStage(path))
		v.bottom.Append(v.zoomBar(false))
	case kind == "video" || kind == "gif":
		v.player = newMediaPlayer(path, m.Attachment.DurationSecs)
		v.player.SetLoop(kind == "gif")
		stage := newVideoStage(v.player, m.Attachment.Thumbnail)
		v.stage.SetChild(v.centred(stage, true))
		v.bottom.Append(newTransportBar(v.player, v.fullscreen))
	case kind == "audio":
		v.player = newPendingPlayer(m.Attachment.DurationSecs)
		preparePlayable(v.player, path, m.Attachment.MimeType, func(err error) {
			showToast(v.toasts, "Can't play this audio here: "+err.Error())
		})
		v.stage.SetChild(v.centred(v.audioCard(m, now), false))
		v.bottom.Append(newTransportBar(v.player, nil))
	case kind == "pdf":
		v.stage.SetChild(v.pdfStage(path))
		v.bottom.Append(v.zoomBar(true))
	default:
		v.stage.SetChild(v.centred(v.fileCard(m, kind), false))
	}
	if cap := v.captionRow(m); cap != nil {
		v.bottom.Append(cap)
	}
	v.refreshStrip()
	if v.detailsOn {
		v.fillDetails()
	}
}

// subline is "From · Today at 15:38 · 2.1 MB · 2268 × 4032".
func (v *AttachmentViewer) subline(m client.Message, kind string, now time.Time) string {
	parts := []string{v.from(m), sentAt(m.TS, now)}
	if kind == "location" {
		if m.Location.IsLive {
			parts = append(parts, "live location")
		} else {
			parts = append(parts, "static point")
		}
		return strings.Join(parts, " · ")
	}
	a := m.Attachment
	parts = append(parts, humanSize(a.Size))
	if kind == "pdf" && v.pages > 0 {
		parts = append(parts, strconv.Itoa(v.pages)+" pages")
	}
	if kind == "video" || kind == "gif" || kind == "audio" {
		parts = append(parts, humanDuration(a.DurationSecs))
	}
	if kind == "photo" && a.Width > 0 && a.Height > 0 {
		parts = append(parts, strconv.Itoa(a.Width)+" × "+strconv.Itoa(a.Height))
	}
	return joinMeta(parts...)
}

func (v *AttachmentViewer) refreshStar() {
	m, ok := v.Current()
	if !ok {
		return
	}
	if m.Starred {
		v.starBtn.SetLabel("⭐")
		v.starBtn.SetTooltipText("Unstar")
	} else {
		v.starBtn.SetLabel("☆")
		v.starBtn.SetTooltipText("Star")
	}
}

// centred wraps a stage widget so it sits in the middle of the stage with
// the design's 24px inset; fill lets it take the whole stage (video).
func (v *AttachmentViewer) centred(w gtk.Widgetter, fill bool) gtk.Widgetter {
	base := gtk.BaseWidget(w)
	base.SetMarginTop(24)
	base.SetMarginBottom(24)
	base.SetMarginStart(24)
	base.SetMarginEnd(24)
	if fill {
		base.SetHExpand(true)
		base.SetVExpand(true)
		return w
	}
	base.SetHAlign(gtk.AlignCenter)
	base.SetVAlign(gtk.AlignCenter)
	base.SetHExpand(true)
	base.SetVExpand(true)
	return w
}

// ---- stages -------------------------------------------------------------

// fetchStage is the not-downloaded state: a 64px ⬇ disc, the kind and
// size, and the invitation to fetch it into this view.
func (v *AttachmentViewer) fetchStage(m client.Message, kind string) gtk.Widgetter {
	col := gtk.NewBox(gtk.OrientationVertical, 13)
	col.SetHAlign(gtk.AlignCenter)
	col.SetVAlign(gtk.AlignCenter)
	col.SetVExpand(true)
	col.SetHExpand(true)

	disc := newRoundGlyphButton("⬇", 64)
	disc.RemoveCSSClass("chatot-round-btn")
	disc.AddCSSClass("chatot-viewer-fetch")
	disc.SetFocusOnClick(false)
	col.Append(disc)

	title := gtk.NewLabel(joinMeta(viewerKindLabel(m, kind), humanSize(m.Attachment.Size)))
	title.AddCSSClass("chatot-viewer-fetch-title")
	col.Append(title)

	sub := gtk.NewLabel("Not downloaded yet · click to fetch it into this view")
	sub.AddCSSClass("chatot-viewer-fetch-sub")
	col.Append(sub)

	disc.ConnectClicked(func() { v.download(m, col, disc, title, sub) })
	return col
}

// download fetches the current attachment and re-renders the stage.
func (v *AttachmentViewer) download(m client.Message, col *gtk.Box, disc *gtk.Button, title, sub *gtk.Label) {
	if v.loading {
		return
	}
	v.loading = true
	gen := v.gen
	disc.SetSensitive(false)
	sub.SetLabel("Downloading…")
	go func() {
		path, err := v.c.DownloadMedia(context.Background(), m.ID)
		glib.IdleAdd(func() {
			if v.gen != gen {
				return
			}
			v.loading = false
			if err != nil {
				disc.SetSensitive(true)
				disc.AddCSSClass("chatot-viewer-fetch-failed")
				title.SetLabel("Download failed")
				sub.SetLabel("Click to retry — the sender may need to be online")
				return
			}
			a := *m.Attachment
			a.LocalPath = path
			m.Attachment = &a
			v.items[v.idx] = m
			if v.onDownloaded != nil {
				v.onDownloaded(m.ID, path)
			}
			v.show(v.idx)
		})
	}()
}

// photoStage is the picture on the dark stage, fitted or zoomed (see
// applyZoom) inside a scroller.
func (v *AttachmentViewer) photoStage(path string) gtk.Widgetter {
	texture, err := gdk.NewTextureFromFilename(path)
	if err != nil {
		return v.centred(v.brokenCard("This picture can't be decoded here.", path), false)
	}
	v.pic = gtk.NewPictureForPaintable(texture)
	v.picTex = texture
	v.picW, v.picH = texture.Width(), texture.Height()
	return v.pictureScroller()
}

// pictureScroller hosts v.pic, centred, in a scroller for zoomed sizes.
func (v *AttachmentViewer) pictureScroller() gtk.Widgetter {
	v.pic.SetContentFit(gtk.ContentFitContain)
	v.pic.SetHAlign(gtk.AlignCenter)
	v.pic.SetVAlign(gtk.AlignCenter)
	v.pic.AddCSSClass("chatot-viewer-picture")
	v.pic.SetMarginTop(24)
	v.pic.SetMarginBottom(24)
	v.pic.SetMarginStart(24)
	v.pic.SetMarginEnd(24)
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scroller.SetHExpand(true)
	scroller.SetVExpand(true)
	scroller.SetChild(v.pic)
	// The wheel zooms (a notch up = in, down = out) rather than panning:
	// captured ahead of the scrolled window so it never scrolls the stage.
	// The zoom keeps the point under the pointer where it was.
	wheel := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical)
	wheel.SetPropagationPhase(gtk.PhaseCapture)
	var px, py float64
	motion := gtk.NewEventControllerMotion()
	motion.ConnectMotion(func(x, y float64) { px, py = x, y })
	scroller.AddController(motion)
	wheel.ConnectScroll(func(_, dy float64) bool {
		// A wheel notch is dy = ±1; a touchpad streams fractions of that,
		// so the zoom follows the finger instead of jumping a step.
		v.zoomAt(scroller, math.Pow(wheelZoomPerNotch, -dy), px, py)
		return true
	})
	scroller.AddController(wheel)
	v.wheel = func(f, x, y float64) { v.zoomAt(scroller, f, x, y) }
	hadj, vadj := scroller.HAdjustment(), scroller.VAdjustment()
	hadj.ConnectChanged(func() { v.applyAnchor(hadj, vadj) })
	vadj.ConnectChanged(func() { v.applyAnchor(hadj, vadj) })
	v.applyZoom()
	return scroller
}

// wheelZoomPerNotch is the zoom change of one wheel notch: fine enough that
// a few notches feel continuous, and a touchpad's fractional deltas finer
// still.
const wheelZoomPerNotch = 1.12

// viewerPicMargin is the margin around the picture in its scroller: the
// scrolled content is the picture plus twice this on each axis.
const viewerPicMargin = 24.0

// zoomAnchor is where the scroller should sit once the picture takes its
// new zoomed size: the values to set, and the content extents at which
// they are final. Kept on the viewer so that a burst of wheel events (a
// touchpad sends several per frame) only ever has one target, applied by
// one handler, instead of a stale handler per event pulling the scroller
// back and forth.
type zoomAnchor struct {
	on       bool
	hv, vv   float64
	hup, vup float64
}

// zoomAt multiplies the zoom by factor keeping the picture point under
// (x, y), in the scroller's coordinates, where it is, so wheel zoom homes
// in on the pointer. The picture is centred when smaller than the stage,
// so the point is taken relative to the drawn picture, not the scroller.
func (v *AttachmentViewer) zoomAt(scroller *gtk.ScrolledWindow, factor, x, y float64) {
	if v.pic == nil || factor <= 0 {
		return
	}
	hadj, vadj := scroller.HAdjustment(), scroller.VAdjustment()
	pw, ph := hadj.PageSize(), vadj.PageSize()
	fit := v.fitScale()
	w0 := float64(v.picW) * v.zoom * fit
	h0 := float64(v.picH) * v.zoom * fit
	// The pointed-at picture point as fractions of the picture, the
	// picture's top-left sitting at its margin, or centred when smaller
	// than the page. Page size doesn't change with zoom, so this holds
	// before and after layout alike.
	fx := (hadj.Value() + x - picOffset(pw, w0)) / w0
	fy := (vadj.Value() + y - picOffset(ph, h0)) / h0
	fx = math.Min(1, math.Max(0, fx))
	fy = math.Min(1, math.Max(0, fy))
	v.setZoom(v.zoom * factor)
	w1 := float64(v.picW) * v.zoom * fit
	h1 := float64(v.picH) * v.zoom * fit
	v.anchor = zoomAnchor{
		on:  true,
		hv:  fx*w1 - x + picOffset(pw, w1),
		vv:  fy*h1 - y + picOffset(ph, h1),
		hup: math.Max(pw, w1+2*viewerPicMargin),
		vup: math.Max(ph, h1+2*viewerPicMargin),
	}
	// Reachable already (zooming out, or clamped at a bound), or once the
	// adjustments take the new extents during layout (see applyAnchor).
	v.applyAnchor(hadj, vadj)
}

// picOffset is where the picture's left/top edge sits in the scrolled
// content: at its margin when it overflows the page, else centred.
func picOffset(page, pic float64) float64 {
	return math.Max(viewerPicMargin, (page-pic)/2)
}

// applyAnchor places the scroller at the pending zoom anchor, and retires
// it once the adjustments have the extents it was computed for. Wired to
// both adjustments' "changed" (extents changed in layout), which happens
// before that frame paints, so the picture never shows at the old offset.
func (v *AttachmentViewer) applyAnchor(hadj, vadj *gtk.Adjustment) {
	a := v.anchor
	if !a.on {
		return
	}
	hadj.SetValue(a.hv)
	vadj.SetValue(a.vv)
	if math.Abs(hadj.Upper()-a.hup) < 1.5 && math.Abs(vadj.Upper()-a.vup) < 1.5 {
		v.anchor.on = false
	}
}

// fitScale is the scale that fits the current picture in the stage.
func (v *AttachmentViewer) fitScale() float64 {
	if v.picW <= 0 || v.picH <= 0 {
		return 1
	}
	sw := float64(v.stage.AllocatedWidth() - 48)
	sh := float64(v.stage.AllocatedHeight() - 48)
	if sw <= 0 || sh <= 0 {
		sw, sh = 560, 620
	}
	return math.Min(sw/float64(v.picW), sh/float64(v.picH))
}

// applyZoom sizes the picture for the current zoom step: at Fit it may
// shrink to the stage; zoomed it asks for its scaled pixel size and the
// scroller pans.
func (v *AttachmentViewer) applyZoom() {
	if v.pic == nil {
		return
	}
	fit := v.fitScale()
	mul := v.zoom
	atFit := v.atFit()
	if atFit {
		v.pic.SetPaintable(v.picTex)
		v.pic.SetCanShrink(true)
	} else {
		// A GtkPicture never asks for less than its paintable's own size
		// once it may not shrink, so a size request below that is ignored
		// and the picture would jump to 100%. Show it through a paintable
		// whose intrinsic size is the scaled size instead: a render node
		// referencing the texture, scaled by the GPU, no pixels copied.
		scale := fit * mul
		w := float32(float64(v.picW)*scale + 0.5)
		h := float32(float64(v.picH)*scale + 0.5)
		v.pic.SetPaintable(scaledPaintable(v.picTex, w, h))
		v.pic.SetCanShrink(false)
	}
	v.pic.SetSizeRequest(-1, -1)
	if v.zoomLbl != nil {
		shown := fit * mul
		if shown > 1 && atFit {
			shown = 1 // a small picture is not upscaled to fit
		}
		v.zoomLbl.SetLabel(strconv.Itoa(int(shown*100+0.5)) + "%")
	}
	if v.fitBtn != nil {
		if atFit {
			v.fitBtn.AddCSSClass("chatot-viewer-fit-on")
		} else {
			v.fitBtn.RemoveCSSClass("chatot-viewer-fit-on")
		}
	}
}

// atFit reports whether the zoom is (within rounding) the fitted size.
func (v *AttachmentViewer) atFit() bool { return math.Abs(v.zoom-1) < 0.005 }

// zoomBy moves to the next design step above (d > 0) or below (d < 0) the
// current zoom, which the wheel may have left between steps.
func (v *AttachmentViewer) zoomBy(d int) { v.setZoom(nextZoomStep(v.zoom, d)) }

// nextZoomStep is the first of viewerZoomSteps strictly above (d > 0) or
// below (d < 0) z, or z's nearest bound when there is none. A z within
// rounding of a step counts as that step.
func nextZoomStep(z float64, d int) float64 {
	const eps = 0.005
	if d > 0 {
		for _, s := range viewerZoomSteps {
			if s > z+eps {
				return s
			}
		}
		return viewerZoomSteps[len(viewerZoomSteps)-1]
	}
	for i := len(viewerZoomSteps) - 1; i >= 0; i-- {
		if viewerZoomSteps[i] < z-eps {
			return viewerZoomSteps[i]
		}
	}
	return viewerZoomSteps[0]
}

// setZoom sets the zoom multiple, clamped to the design's range.
func (v *AttachmentViewer) setZoom(z float64) {
	if v.pic == nil {
		return
	}
	z = math.Max(viewerZoomSteps[0], math.Min(viewerZoomSteps[len(viewerZoomSteps)-1], z))
	v.zoom = z
	v.applyZoom()
}

// zoomBar is the design's − 13% ＋ Fit row, with ‹ 1 / 8 › in front of it
// for a PDF.
func (v *AttachmentViewer) zoomBar(pages bool) gtk.Widgetter {
	// The strip spans the pane (its top hairline and surface); the
	// controls sit centred inside it.
	strip := gtk.NewBox(gtk.OrientationHorizontal, 0)
	strip.AddCSSClass("chatot-viewer-zoombar")
	strip.SetHExpand(true)
	bar := gtk.NewBox(gtk.OrientationHorizontal, 8)
	bar.SetHAlign(gtk.AlignCenter)
	bar.SetHExpand(true)
	strip.Append(bar)
	if pages {
		pg := gtk.NewBox(gtk.OrientationHorizontal, 2)
		pg.SetMarginEnd(6)
		prev := v.headerButton("‹", "Previous page", func() { v.setPage(v.page - 1) })
		prev.AddCSSClass("chatot-viewer-pagebtn")
		pg.Append(prev)
		v.pageLbl = gtk.NewLabel("1 / 1")
		v.pageLbl.AddCSSClass("chatot-viewer-pagelbl")
		pg.Append(v.pageLbl)
		next := v.headerButton("›", "Next page", func() { v.setPage(v.page + 1) })
		next.AddCSSClass("chatot-viewer-pagebtn")
		pg.Append(next)
		bar.Append(pg)
	}
	minus := gtk.NewButtonWithLabel("−")
	minus.RemoveCSSClass("text-button")
	minus.AddCSSClass("chatot-viewer-zoombtn")
	minus.SetTooltipText("Zoom out · −")
	minus.SetFocusOnClick(false)
	minus.ConnectClicked(func() { v.zoomBy(-1) })
	bar.Append(minus)
	v.zoomLbl = gtk.NewLabel("100%")
	v.zoomLbl.AddCSSClass("chatot-viewer-zoomlbl")
	bar.Append(v.zoomLbl)
	plus := gtk.NewButtonWithLabel("＋")
	plus.RemoveCSSClass("text-button")
	plus.AddCSSClass("chatot-viewer-zoombtn")
	plus.SetTooltipText("Zoom in · +")
	plus.SetFocusOnClick(false)
	plus.ConnectClicked(func() { v.zoomBy(1) })
	bar.Append(plus)
	v.fitBtn = gtk.NewButtonWithLabel("Fit")
	v.fitBtn.AddCSSClass("chatot-viewer-zoombtn")
	v.fitBtn.AddCSSClass("chatot-viewer-fitbtn")
	v.fitBtn.SetTooltipText("Fit to window · 0")
	v.fitBtn.SetFocusOnClick(false)
	v.fitBtn.SetMarginStart(4)
	v.fitBtn.ConnectClicked(func() { v.setZoom(1) })
	bar.Append(v.fitBtn)
	// The stage's size is only known once mapped; refit then.
	glib.IdleAdd(func() { v.applyZoom() })
	return strip
}

// pdfStage renders the current page onto a white sheet; the page count
// comes from pdfinfo and pages are rasterised on demand and cached.
func (v *AttachmentViewer) pdfStage(path string) gtk.Widgetter {
	v.pic = gtk.NewPicture()
	v.picW, v.picH = 595, 842
	gen := v.gen
	go func() {
		n, _ := media.PDFPageCount(context.Background(), path)
		glib.IdleAdd(func() {
			if v.gen != gen {
				return
			}
			if n < 1 {
				n = 1
			}
			v.pages = n
			v.refreshPageLabel()
			if m, ok := v.Current(); ok {
				v.sub.SetLabel(v.subline(m, "pdf", time.Now()))
			}
		})
	}()
	v.renderPage(path, 1)
	return v.pictureScroller()
}

func (v *AttachmentViewer) refreshPageLabel() {
	if v.pageLbl != nil {
		v.pageLbl.SetLabel(strconv.Itoa(v.page) + " / " + strconv.Itoa(maxInt(v.pages, 1)))
	}
}

func (v *AttachmentViewer) setPage(n int) {
	m, ok := v.Current()
	if !ok || viewerKind(m) != "pdf" {
		return
	}
	if n < 1 || n > maxInt(v.pages, 1) {
		return
	}
	v.page = n
	v.refreshPageLabel()
	path, _ := hasLocal(m)
	v.renderPage(path, n)
}

// renderPage paints page n of the PDF into v.pic, rasterising it off the
// main loop the first time.
func (v *AttachmentViewer) renderPage(path string, n int) {
	if tex, ok := v.pdfCache[n]; ok {
		v.picTex = tex
		v.picW, v.picH = tex.Width(), tex.Height()
		v.applyZoom()
		return
	}
	gen := v.gen
	go func() {
		jpeg, err := media.PDFPageAt(context.Background(), path, n, viewerPDFRender)
		glib.IdleAdd(func() {
			if v.gen != gen || v.pic == nil {
				return
			}
			if err != nil {
				log.Printf("chatot: render pdf page %d of %s: %v", n, path, err)
				showToast(v.toasts, "Couldn't render the page: "+err.Error())
				return
			}
			tex, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(jpeg))
			if err != nil {
				return
			}
			v.pdfCache[n] = tex
			if v.page == n {
				v.picTex = tex
				v.picW, v.picH = tex.Width(), tex.Height()
				v.applyZoom()
			}
		})
	}()
}

// audioCard is the design's voice-note card: sender avatar, name, "Voice
// message · 48 KB", the length, and a waveform whose played part is accent.
func (v *AttachmentViewer) audioCard(m client.Message, now time.Time) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationVertical, 16)
	card.AddCSSClass("chatot-viewer-card")
	card.SetSizeRequest(400, -1)

	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	from := v.from(m)
	avatarJID := nonADJID(m.FromJID)
	if m.FromMe {
		avatarJID = v.c.OwnJID()
	}
	avatar := newAvatarInitial(avatarJID, initialFor(from), 36)
	avatar.SetVAlign(gtk.AlignCenter)
	row.Append(avatar)
	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(from)
	name.AddCSSClass("chatot-viewer-card-title")
	name.SetXAlign(0)
	col.Append(name)
	sub := gtk.NewLabel(joinMeta("Voice message", humanSize(m.Attachment.Size)))
	sub.AddCSSClass("chatot-viewer-card-sub")
	sub.SetXAlign(0)
	col.Append(sub)
	row.Append(col)
	total := gtk.NewLabel(humanDuration(m.Attachment.DurationSecs))
	total.AddCSSClass("chatot-viewer-counter")
	row.Append(total)
	card.Append(row)

	wave := gtk.NewDrawingArea()
	wave.SetSizeRequest(-1, 44)
	wave.SetHExpand(true)
	p := v.player
	wave.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawWaveform(cr, float64(w), float64(h), p.Progress(), isDark())
	})
	seek := gtk.NewGestureClick()
	seek.ConnectReleased(func(_ int, x, _ float64) {
		if w := float64(wave.AllocatedWidth()); w > 0 {
			p.SeekTo(x / w)
		}
	})
	wave.AddController(seek)
	p.watchUntilDestroyed(wave, func() {
		wave.QueueDraw()
		if p.Ready() {
			total.SetLabel(humanClock(p.Duration()))
		}
	})
	card.Append(wave)
	return card
}

// drawWaveform paints the design's 44 bars: a deterministic pseudo-envelope
// (the file's real one isn't decoded), accent up to progress.
func drawWaveform(cr *cairo.Context, w, h, progress float64, dark bool) {
	const n = 44
	gap := 2.0
	bw := (w - gap*(n-1)) / n
	if bw < 1 {
		bw = 1
	}
	for i := 0; i < n; i++ {
		bh := 6 + math.Round(30*math.Abs(math.Sin(float64(i)*0.68))*(0.5+0.5*math.Abs(math.Cos(float64(i)*0.21))))
		x := float64(i) * (bw + gap)
		if float64(i)/n < progress {
			cr.SetSourceRGB(0x1b/255.0, 0x8c/255.0, 0x72/255.0)
		} else if dark {
			cr.SetSourceRGBA(1, 1, 1, 0.32)
		} else {
			cr.SetSourceRGBA(0, 0, 0, 0.24)
		}
		roundedRectPath(cr, x, (h-bh)/2, bw, bh, bw/2)
		cr.Fill()
	}
}

// fileCard is the design's "no preview" card for other document kinds.
func (v *AttachmentViewer) fileCard(m client.Message, kind string) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationVertical, 14)
	card.AddCSSClass("chatot-viewer-card")
	card.AddCSSClass("chatot-viewer-filecard")
	card.SetSizeRequest(340, -1)

	ext := fileExt(m.Attachment.Filename)
	tile := gtk.NewLabel(ext)
	tile.AddCSSClass("chatot-viewer-exttile")
	tile.AddCSSClass(extTileClass(ext))
	tile.SetHAlign(gtk.AlignCenter)
	card.Append(tile)

	name := gtk.NewLabel(viewerTitle(m, kind))
	name.AddCSSClass("chatot-viewer-file-name")
	name.SetWrap(true)
	name.SetWrapMode(pango.WrapWordChar)
	name.SetJustify(gtk.JustifyCenter)
	name.SetMaxWidthChars(30)
	card.Append(name)
	meta := gtk.NewLabel(joinMeta(viewerKindLabel(m, kind), humanSize(m.Attachment.Size), v.from(m)))
	meta.AddCSSClass("chatot-viewer-card-sub")
	card.Append(meta)
	note := gtk.NewLabel("Chatot has no preview for this file type. Open it in the app your desktop uses for it.")
	note.AddCSSClass("chatot-viewer-file-note")
	note.SetWrap(true)
	note.SetJustify(gtk.JustifyCenter)
	note.SetMaxWidthChars(38)
	card.Append(note)

	if !v.detailsOn {
		actions := gtk.NewBox(gtk.OrientationHorizontal, 8)
		actions.SetHAlign(gtk.AlignCenter)
		actions.Append(newChipButton("Save as…", func() { v.saveAs(m) }))
		actions.Append(newChipButton("Show in Files", func() { v.showInFiles(m) }))
		card.Append(actions)
	}
	return card
}

// brokenCard is the file card's cousin for a picture GTK can't decode.
func (v *AttachmentViewer) brokenCard(text, path string) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationVertical, 12)
	card.AddCSSClass("chatot-viewer-card")
	card.SetSizeRequest(340, -1)
	lbl := gtk.NewLabel(text)
	lbl.AddCSSClass("chatot-viewer-file-note")
	lbl.SetWrap(true)
	card.Append(lbl)
	card.Append(newChipButton("Open with…", func() { openFile(path) }))
	return card
}

// fileExt is the upper-case extension, at most four letters, for tiles.
func fileExt(name string) string {
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(name), "."))
	if len(ext) > 4 {
		ext = ext[:4]
	}
	if ext == "" {
		ext = "FILE"
	}
	return ext
}

// extTileClass picks the tile colour: red for PDF, green for spreadsheets,
// slate for the rest (the design's vTileBg).
func extTileClass(ext string) string {
	switch ext {
	case "PDF":
		return "chatot-viewer-ext-pdf"
	case "ODS", "XLSX", "XLS", "CSV":
		return "chatot-viewer-ext-sheet"
	}
	return "chatot-viewer-ext-file"
}

// locationStage is the interactive map with the pin, zoom, LIVE badge,
// the place card and the attribution.
func (v *AttachmentViewer) locationStage(m client.Message) gtk.Widgetter {
	loc := m.Location
	frame := gtk.NewOverlay()
	frame.AddCSSClass("chatot-viewer-map")
	frame.SetOverflow(gtk.OverflowHidden)
	frame.SetSizeRequest(300, 240)
	frame.SetHExpand(true)
	frame.SetVExpand(true)
	frame.SetMarginTop(24)
	frame.SetMarginBottom(24)
	frame.SetMarginStart(24)
	frame.SetMarginEnd(24)

	v.mapView = newMapView(sharedTiles)
	v.mapView.SetCentre(loc.Latitude, loc.Longitude)
	v.mapView.SetZoom(v.mapZoom)
	v.mapView.SetMarker(markerPin, loc.Latitude, loc.Longitude, 0)
	v.mapView.SetInteractive(true)
	v.mapView.onViewportChanged = func() {
		v.mapZoom = v.mapView.Zoom()
		v.refreshMapLabel(m)
	}
	frame.SetChild(v.mapView)

	lv := locationVM(m)
	if lv.Live {
		badge := gtk.NewBox(gtk.OrientationHorizontal, 6)
		badge.AddCSSClass("chatot-location-live-badge")
		badge.SetHAlign(gtk.AlignStart)
		badge.SetVAlign(gtk.AlignStart)
		badge.SetMarginStart(14)
		badge.SetMarginTop(14)
		dot := gtk.NewBox(gtk.OrientationVertical, 0)
		dot.AddCSSClass("chatot-location-live-dot")
		dot.SetSizeRequest(6, 6)
		dot.SetVAlign(gtk.AlignCenter)
		badge.Append(dot)
		badge.Append(gtk.NewLabel("LIVE"))
		frame.AddOverlay(badge)
	}

	zoomCol := gtk.NewBox(gtk.OrientationVertical, 0)
	zoomCol.AddCSSClass("chatot-loc-zoom")
	zoomCol.SetHAlign(gtk.AlignEnd)
	zoomCol.SetVAlign(gtk.AlignStart)
	zoomCol.SetMarginEnd(14)
	zoomCol.SetMarginTop(14)
	zin := gtk.NewButtonWithLabel("＋")
	zin.AddCSSClass("flat")
	zin.AddCSSClass("chatot-loc-zoom-btn")
	zin.SetTooltipText("Zoom in")
	zin.SetFocusOnClick(false)
	zin.ConnectClicked(func() { v.mapZoomBy(1) })
	zoomCol.Append(zin)
	zsep := gtk.NewBox(gtk.OrientationVertical, 0)
	zsep.AddCSSClass("chatot-loc-zoom-sep")
	zsep.SetSizeRequest(-1, 1)
	zoomCol.Append(zsep)
	zout := gtk.NewButtonWithLabel("−")
	zout.AddCSSClass("flat")
	zout.AddCSSClass("chatot-loc-zoom-btn")
	zout.SetTooltipText("Zoom out")
	zout.SetFocusOnClick(false)
	zout.ConnectClicked(func() { v.mapZoomBy(-1) })
	zoomCol.Append(zout)
	frame.AddOverlay(zoomCol)

	card := gtk.NewBox(gtk.OrientationVertical, 3)
	card.AddCSSClass("chatot-viewer-loccard")
	card.SetHAlign(gtk.AlignStart)
	card.SetVAlign(gtk.AlignEnd)
	card.SetMarginStart(14)
	card.SetMarginBottom(14)
	title := gtk.NewLabel(viewerTitle(m, "location"))
	title.AddCSSClass("chatot-viewer-card-title")
	title.SetXAlign(0)
	card.Append(title)
	sub := gtk.NewLabel(lv.Sub)
	sub.AddCSSClass("chatot-viewer-card-sub")
	sub.SetXAlign(0)
	sub.SetEllipsize(pango.EllipsizeEnd)
	sub.SetMaxWidthChars(40)
	card.Append(sub)
	if lv.NeedsPlace {
		lookupPlace(loc.Latitude, loc.Longitude, func(place string) {
			if place != "" {
				sub.SetLabel(place)
			}
		})
	}
	v.mapLbl = gtk.NewLabel("")
	v.mapLbl.AddCSSClass("chatot-viewer-loccoords")
	v.mapLbl.SetXAlign(0)
	card.Append(v.mapLbl)
	v.refreshMapLabel(m)
	frame.AddOverlay(card)

	attribution := gtk.NewLabel("© OpenStreetMap")
	attribution.AddCSSClass("chatot-loc-attribution")
	attribution.SetHAlign(gtk.AlignEnd)
	attribution.SetVAlign(gtk.AlignEnd)
	attribution.SetMarginEnd(8)
	attribution.SetMarginBottom(6)
	frame.AddOverlay(attribution)
	return frame
}

func (v *AttachmentViewer) mapZoomBy(d int) {
	if v.mapView == nil {
		return
	}
	z := v.mapView.Zoom() + d
	if z < viewerMapZoomMin {
		z = viewerMapZoomMin
	}
	if z > viewerMapZoomMax {
		z = viewerMapZoomMax
	}
	v.mapView.SetZoom(z)
	v.mapZoom = z
	if m, ok := v.Current(); ok {
		v.refreshMapLabel(m)
	}
}

func (v *AttachmentViewer) refreshMapLabel(m client.Message) {
	if v.mapLbl == nil || m.Location == nil {
		return
	}
	v.mapLbl.SetLabel(coordText(m.Location.Latitude, m.Location.Longitude) + " · z" + strconv.Itoa(v.mapZoom))
}

// coordText is "41.14548, -8.61052" (five decimals, the design's).
func coordText(lat, lon float64) string {
	return strconv.FormatFloat(lat, 'f', 5, 64) + ", " + strconv.FormatFloat(lon, 'f', 5, 64)
}

// captionRow shows the attachment's caption and its reactions, or nothing.
func (v *AttachmentViewer) captionRow(m client.Message) gtk.Widgetter {
	caption := ""
	if m.Attachment != nil {
		caption = m.Attachment.Caption
	}
	reacts := reactionViews(m.Reactions)
	if caption == "" && len(reacts) == 0 {
		return nil
	}
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-viewer-caption")
	text := gtk.NewLabel(caption)
	text.AddCSSClass("chatot-viewer-caption-text")
	text.SetXAlign(0)
	text.SetWrap(true)
	text.SetHExpand(true)
	row.Append(text)
	if len(reacts) > 0 {
		pills := gtk.NewBox(gtk.OrientationHorizontal, 4)
		pills.SetVAlign(gtk.AlignStart)
		for _, r := range reacts {
			pill := gtk.NewLabel(r.Emoji + reactionCountText(len(r.Reactors)))
			pill.AddCSSClass("chatot-viewer-react")
			pills.Append(pill)
		}
		row.Append(pills)
	}
	return row
}

// ---- filmstrip ----------------------------------------------------------

func (v *AttachmentViewer) refreshStrip() {
	removeAllChildren(v.strip)
	for i, m := range v.items {
		v.strip.Append(v.stripTile(i, m))
	}
	v.stripLbl.SetLabel(pluralCount(len(v.items), "attachment", "attachments") + " in this chat")
}

// stripTile is one 52px filmstrip tile: a cover thumbnail for a picture or
// clip, a coloured extension tile for a document, a glyph otherwise, with
// a small badge (⬇ pending, the clip's length, GIF).
func (v *AttachmentViewer) stripTile(i int, m client.Message) gtk.Widgetter {
	kind := viewerKind(m)
	overlay := gtk.NewOverlay()
	overlay.AddCSSClass("chatot-viewer-thumb")
	overlay.SetOverflow(gtk.OverflowHidden)
	overlay.SetSizeRequest(viewerThumb, viewerThumb)
	if i == v.idx {
		overlay.AddCSSClass("chatot-viewer-thumb-on")
	}

	var face gtk.Widgetter
	switch kind {
	case "pdf", "file":
		ext := fileExt(m.Attachment.Filename)
		lbl := gtk.NewLabel(ext)
		lbl.AddCSSClass("chatot-viewer-thumb-ext")
		lbl.AddCSSClass(extTileClass(ext))
		face = lbl
	case "location":
		if len(m.Location.Thumbnail) > 0 {
			if pb, err := pixbufFromBytes(m.Location.Thumbnail); err == nil {
				face = coverThumb(pb, viewerThumb, viewerThumb, 0, false)
			}
		}
		if face == nil {
			lbl := gtk.NewLabel("📍")
			lbl.AddCSSClass("chatot-viewer-thumb-glyph")
			face = lbl
		}
	case "audio":
		lbl := gtk.NewLabel("🎤")
		lbl.AddCSSClass("chatot-viewer-thumb-glyph")
		lbl.AddCSSClass("chatot-viewer-thumb-chip")
		face = lbl
	default:
		if len(m.Attachment.Thumbnail) > 0 {
			if pb, err := pixbufFromBytes(m.Attachment.Thumbnail); err == nil {
				face = coverThumb(pb, viewerThumb, viewerThumb, 0, false)
			}
		}
		if face == nil {
			if path, ok := hasLocal(m); ok && kind == "photo" {
				if pb, err := gdkpixbufAtScale(path, viewerThumb*2); err == nil {
					face = coverThumb(pb, viewerThumb, viewerThumb, 0, false)
				}
			}
		}
		if face == nil {
			lbl := gtk.NewLabel(viewerGlyph(kind))
			lbl.AddCSSClass("chatot-viewer-thumb-glyph")
			lbl.AddCSSClass("chatot-media-hatch")
			face = lbl
		}
	}
	// No expand flags on the face: GTK propagates them up to the strip,
	// which would then claim the stage's height.
	overlay.SetChild(face)

	badge := ""
	switch {
	case kind != "location" && !localOK(m):
		badge = "⬇"
	case kind == "video" || kind == "audio":
		badge = humanDuration(m.Attachment.DurationSecs)
	case kind == "gif":
		badge = "GIF"
	}
	if badge != "" {
		b := gtk.NewLabel(badge)
		b.AddCSSClass("chatot-viewer-thumb-badge")
		b.SetHAlign(gtk.AlignEnd)
		b.SetVAlign(gtk.AlignEnd)
		b.SetMarginEnd(3)
		b.SetMarginBottom(3)
		overlay.AddOverlay(b)
	}

	btn := gtk.NewButton()
	btn.SetChild(overlay)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btn")
	btn.AddCSSClass("chatot-viewer-thumb-btn")
	btn.SetFocusOnClick(false)
	btn.SetTooltipText(viewerTitle(m, kind) + " · " + time.Unix(m.TS, 0).Format("15:04"))
	idx := i
	btn.ConnectClicked(func() { v.show(idx) })
	return btn
}

func localOK(m client.Message) bool {
	_, ok := hasLocal(m)
	return ok
}

// ---- details sidebar ----------------------------------------------------

// ToggleDetails opens/closes the details sidebar (the header's ℹ).
func (v *AttachmentViewer) ToggleDetails() { v.toggleDetails() }

func (v *AttachmentViewer) toggleDetails() {
	v.detailsOn = !v.detailsOn
	v.details.SetVisible(v.detailsOn)
	if v.detailsOn {
		v.infoBtn.AddCSSClass("chatot-viewer-hbtn-on")
		v.fillDetails()
	} else {
		v.infoBtn.RemoveCSSClass("chatot-viewer-hbtn-on")
	}
	// The file card hides its buttons while the sidebar offers the same.
	if m, ok := v.Current(); ok {
		if k := viewerKind(m); k == "file" && localOK(m) {
			v.stage.SetChild(v.centred(v.fileCard(m, k), false))
		}
	}
}

func (v *AttachmentViewer) fillDetails() {
	removeAllChildren(v.details)
	m, ok := v.Current()
	if !ok {
		return
	}
	kind := viewerKind(m)
	now := time.Now()

	head := gtk.NewBox(gtk.OrientationHorizontal, 8)
	head.AddCSSClass("chatot-viewer-details-head")
	cap := newSectionCaption("Details")
	cap.SetHExpand(true)
	head.Append(cap)
	head.Append(v.headerButton("✕", "Hide details", v.toggleDetails))
	v.details.Append(head)

	who := gtk.NewBox(gtk.OrientationHorizontal, 10)
	who.AddCSSClass("chatot-viewer-details-who")
	from := v.from(m)
	avatarJID := nonADJID(m.FromJID)
	if m.FromMe {
		avatarJID = v.c.OwnJID()
	}
	who.Append(newAvatarInitial(avatarJID, initialFor(from), 34))
	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(from)
	name.AddCSSClass("chatot-viewer-card-title")
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	col.Append(name)
	when := gtk.NewLabel(sentAt(m.TS, now))
	when.AddCSSClass("chatot-viewer-card-sub")
	when.SetXAlign(0)
	col.Append(when)
	who.Append(col)
	v.details.Append(who)

	facts := gtk.NewBox(gtk.OrientationVertical, 0)
	facts.AddCSSClass("chatot-viewer-facts")
	for _, f := range v.facts(m, kind) {
		row := gtk.NewBox(gtk.OrientationHorizontal, 10)
		row.AddCSSClass("chatot-viewer-fact")
		k := gtk.NewLabel(f[0])
		k.AddCSSClass("chatot-viewer-fact-k")
		k.SetXAlign(0)
		k.SetSizeRequest(74, -1)
		row.Append(k)
		val := gtk.NewLabel(f[1])
		val.AddCSSClass("chatot-viewer-fact-v")
		val.SetXAlign(0)
		val.SetWrap(true)
		val.SetWrapMode(pango.WrapWordChar)
		val.SetHExpand(true)
		row.Append(val)
		facts.Append(row)
	}
	v.details.Append(facts)

	actions := gtk.NewBox(gtk.OrientationVertical, 6)
	actions.AddCSSClass("chatot-viewer-actions")
	show := gtk.NewButtonWithLabel("Show in chat")
	show.AddCSSClass("chatot-viewer-showinchat")
	show.SetFocusOnClick(false)
	show.ConnectClicked(func() { v.back() })
	actions.Append(show)
	for _, a := range v.infoActions(m, kind) {
		lbl := gtk.NewLabel(a.label)
		lbl.SetXAlign(0)
		lbl.SetHExpand(true)
		b := gtk.NewButton()
		b.SetChild(lbl)
		b.AddCSSClass("flat")
		b.AddCSSClass("chatot-viewer-action")
		b.SetFocusOnClick(false)
		onClick := a.onClick
		b.ConnectClicked(onClick)
		actions.Append(b)
	}
	v.details.Append(actions)

	keys := gtk.NewBox(gtk.OrientationVertical, 5)
	keys.AddCSSClass("chatot-viewer-keys")
	keys.Append(newSectionCaption("Keyboard"))
	for _, k := range [][2]string{{"Esc", "Back to chat"}, {"← →", "Previous / next"}, {"+ − 0", "Zoom in, out, fit"}, {"Space", "Play or pause"}} {
		row := gtk.NewBox(gtk.OrientationHorizontal, 8)
		key := gtk.NewLabel(k[0])
		key.AddCSSClass("chatot-viewer-key")
		key.SetSizeRequest(58, -1)
		row.Append(key)
		act := gtk.NewLabel(k[1])
		act.AddCSSClass("chatot-viewer-card-sub")
		act.SetXAlign(0)
		row.Append(act)
		keys.Append(row)
	}
	v.details.Append(keys)

	spacer := gtk.NewBox(gtk.OrientationVertical, 0)
	spacer.SetVExpand(true)
	v.details.Append(spacer)
	foot := gtk.NewLabel("🔒 End-to-end encrypted. Files stay in Chatot's cache until you save them.")
	foot.AddCSSClass("chatot-viewer-foot")
	foot.SetWrap(true)
	foot.SetXAlign(0)
	v.details.Append(foot)
}

// facts are the details sidebar's key/value rows for the shown item.
func (v *AttachmentViewer) facts(m client.Message, kind string) [][2]string {
	chat := v.chat.Name
	var rows [][2]string
	switch kind {
	case "location":
		rows = [][2]string{{"Kind", "Location"}, {"Point", coordText(m.Location.Latitude, m.Location.Longitude)}, {"Zoom", "z" + strconv.Itoa(v.mapZoom)}}
	case "pdf":
		pages := ""
		if v.pages > 0 {
			pages = " · " + strconv.Itoa(v.pages) + " pages"
		}
		rows = [][2]string{{"Kind", "PDF" + pages}, {"Size", humanSize(m.Attachment.Size)}}
	case "file":
		rows = [][2]string{{"Kind", viewerKindLabel(m, kind)}, {"Size", humanSize(m.Attachment.Size)}}
	case "audio":
		rows = [][2]string{{"Kind", mimeKindText(m.Attachment.MimeType, "Audio")}, {"Length", humanDuration(m.Attachment.DurationSecs)}, {"Size", humanSize(m.Attachment.Size)}}
	case "video", "gif":
		k := mimeKindText(m.Attachment.MimeType, "Video")
		if kind == "gif" {
			k = "GIF · loop"
		}
		rows = [][2]string{{"Kind", k}, {"Pixels", pixelsText(m.Attachment)}, {"Length", humanDuration(m.Attachment.DurationSecs)}, {"Size", humanSize(m.Attachment.Size)}}
	default:
		rows = [][2]string{{"Kind", mimeKindText(m.Attachment.MimeType, "Image")}, {"Pixels", pixelsText(m.Attachment)}, {"Size", humanSize(m.Attachment.Size)}}
	}
	if kind == "photo" && v.picW > 0 && rows[1][1] == "" {
		rows[1][1] = strconv.Itoa(v.picW) + " × " + strconv.Itoa(v.picH)
	}
	rows = append(rows, [2]string{"Chat", chat})
	kept := rows[:0]
	for _, r := range rows {
		if r[1] != "" {
			kept = append(kept, r)
		}
	}
	return kept
}

// mimeKindText turns "audio/ogg; codecs=opus" into "OGG · opus", "video/mp4"
// into "MP4", falling back to the noun.
func mimeKindText(mime, fallback string) string {
	if mime == "" {
		return fallback
	}
	main := mime
	codec := ""
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		main = strings.TrimSpace(mime[:i])
		rest := strings.TrimSpace(mime[i+1:])
		if strings.HasPrefix(rest, "codecs=") {
			codec = strings.Trim(strings.TrimPrefix(rest, "codecs="), `"`)
		}
	}
	if i := strings.IndexByte(main, '/'); i >= 0 {
		main = main[i+1:]
	}
	main = strings.ToUpper(strings.TrimPrefix(main, "x-"))
	if main == "" {
		return fallback
	}
	if codec != "" {
		return main + " · " + codec
	}
	return main
}

func pixelsText(a *client.Attachment) string {
	if a == nil || a.Width <= 0 || a.Height <= 0 {
		return ""
	}
	return strconv.Itoa(a.Width) + " × " + strconv.Itoa(a.Height)
}

type viewerAction struct {
	label   string
	onClick func()
}

// infoActions are the sidebar's per-kind actions (the design's vInfoActions).
func (v *AttachmentViewer) infoActions(m client.Message, kind string) []viewerAction {
	path, local := hasLocal(m)
	if kind != "location" && !local {
		return []viewerAction{{"Download now", func() { v.primary() }}}
	}
	switch kind {
	case "location":
		return []viewerAction{
			{"Copy coordinates", func() { copyText(v.toasts, coordText(m.Location.Latitude, m.Location.Longitude)) }},
			{"Copy map link", func() { copyText(v.toasts, mapsURL(m.Location.Latitude, m.Location.Longitude)) }},
			{"Open in Maps", func() { openURI(mapsURL(m.Location.Latitude, m.Location.Longitude)) }},
		}
	case "pdf", "file":
		return []viewerAction{
			{"Save as…", func() { v.saveAs(m) }},
			{"Open with…", func() { openFile(path) }},
			{"Show in Files", func() { v.showInFiles(m) }},
		}
	case "audio":
		return []viewerAction{
			{"Save as…", func() { v.saveAs(m) }},
			{"Open with…", func() { openFile(path) }},
		}
	case "video", "gif":
		return []viewerAction{
			{"Save as…", func() { v.saveAs(m) }},
			{"Open with…", func() { openFile(path) }},
		}
	}
	return []viewerAction{
		{"Copy image", func() { v.copyImage(path) }},
		{"Save as…", func() { v.saveAs(m) }},
		{"Open with…", func() { openFile(path) }},
	}
}

// ---- actions ------------------------------------------------------------

// primary is the header's accent button: Download, Open, Open in Maps or
// Save… depending on the item.
func (v *AttachmentViewer) primary() {
	m, ok := v.Current()
	if !ok {
		return
	}
	kind := viewerKind(m)
	if kind == "location" {
		openURI(mapsURL(m.Location.Latitude, m.Location.Longitude))
		return
	}
	path, local := hasLocal(m)
	if !local {
		if col, ok := v.stage.Child().(*gtk.Box); ok {
			if disc, ok := col.FirstChild().(*gtk.Button); ok {
				disc.Activate()
				return
			}
		}
		return
	}
	if kind == "pdf" || kind == "file" {
		openFile(path)
		return
	}
	v.saveAs(m)
}

// suggestedName is the Save dialog's proposal for msg's file.
func suggestedName(m client.Message, path string) string {
	if m.Attachment != nil && m.Attachment.Filename != "" {
		return m.Attachment.Filename
	}
	stem := map[string]string{"audio": "voice", "video": "video", "image": "photo"}[m.Attachment.Kind]
	if stem == "" {
		stem = "file"
	}
	ext := strings.ToLower(filepath.Ext(path))
	return stem + "-" + time.Unix(m.TS, 0).Format("2006-01-02-150405") + ext
}

func (v *AttachmentViewer) saveAs(m client.Message) {
	path, ok := hasLocal(m)
	if !ok {
		return
	}
	fd := gtk.NewFileDialog()
	fd.SetTitle("Save attachment")
	fd.SetInitialName(suggestedName(m, path))
	fd.Save(context.Background(), v.window, func(res gio.AsyncResulter) {
		file, err := fd.SaveFinish(res)
		if err != nil {
			return // cancelled
		}
		if err := copyFile(path, file.Path()); err != nil {
			showToast(v.toasts, "Couldn't save: "+err.Error())
			return
		}
		showToast(v.toasts, "Saved to "+file.Path())
	})
}

func (v *AttachmentViewer) showInFiles(m client.Message) {
	path, ok := hasLocal(m)
	if !ok {
		return
	}
	launcher := gtk.NewFileLauncher(gio.NewFileForPath(path))
	launcher.OpenContainingFolder(context.Background(), v.window, func(res gio.AsyncResulter) {
		if err := launcher.OpenContainingFolderFinish(res); err != nil {
			log.Printf("chatot: show in files %s: %v", path, err)
			showToast(v.toasts, "Couldn't open the folder: "+err.Error())
			return
		}
		showToast(v.toasts, "Revealing in Files…")
	})
}

func (v *AttachmentViewer) copyImage(path string) {
	texture, err := gdk.NewTextureFromFilename(path)
	if err != nil {
		showToast(v.toasts, "Couldn't copy the picture")
		return
	}
	gdk.DisplayGetDefault().Clipboard().SetTexture(texture)
	showToast(v.toasts, "Copied to clipboard")
}

// Fullscreen is the ⤢ action — a dev/screenshot hook.
func (v *AttachmentViewer) Fullscreen() { v.fullscreen() }

// fullscreen opens the clip alone over the whole screen (the design's ⤢).
func (v *AttachmentViewer) fullscreen() {
	m, ok := v.Current()
	if !ok {
		return
	}
	kind := viewerKind(m)
	if kind != "video" && kind != "gif" {
		return
	}
	path, local := hasLocal(m)
	if !local {
		return
	}
	showVideoFullscreen(v.window, path, m, v.player)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// scaledPaintable is tex drawn at w×h: a paintable whose intrinsic size is
// exactly that, so a GtkPicture showing it measures w×h.
func scaledPaintable(tex *gdk.Texture, w, h float32) gdk.Paintabler {
	if tex == nil {
		return nil
	}
	snap := gtk.NewSnapshot()
	snap.AppendScaledTexture(tex, gsk.ScalingFilterTrilinear, graphene.RectAlloc().Init(0, 0, w, h))
	return snap.ToPaintable(graphene.NewSizeAlloc().Init(w, h))
}
