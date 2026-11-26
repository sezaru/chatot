package ui

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// mediaPaneCols is the mockup's five equal columns; mediaPaneTile is the
// smallest a tile may get before the pane scrolls sideways instead.
const (
	mediaPaneCols = 5
	// mediaPaneTile is a tile's minimum side. The homogeneous columns share
	// the pane's width whatever this is, so it only matters where the pane is
	// narrow: five of these plus the gaps still fit the collapsed 360px window.
	mediaPaneTile = 56
	// mediaPaneFace is the side, in pixels, a tile's picture is decoded to
	// cover: a tile in a wide window at 1.25× scale.
	mediaPaneFace = 256
	// mediaPaneFaceBudget bounds the decoded tile faces kept in memory.
	mediaPaneFaceBudget = 48 << 20
	// mediaPaneStampSide: an embedded thumbnail narrower than this is the
	// message's blurry stamp, and a downloaded file makes a sharper face.
	mediaPaneStampSide = 160
	// mediaPaneFetchTimeout bounds asking the server for a preview.
	mediaPaneFetchTimeout = 30 * time.Second
)

// mediaPanePages are the mockup's three tabs, in its order.
var mediaPanePages = []segmentedPage{
	{"media", "Media"},
	{"links", "Links"},
	{"docs", "Docs"},
}

// mediaTileLabelFor is the small caption in a grid tile's bottom-left corner:
// the weekday for something from the last week, otherwise a short date.
func mediaTileLabelFor(ts int64, now time.Time) string {
	if ts == 0 {
		return ""
	}
	t := time.Unix(ts, 0).In(now.Location())
	if now.Sub(t) < 7*24*time.Hour {
		return t.Format("Mon")
	}
	return t.Format("2 Jan")
}

// mediaGridRow is one row of the Media tab's list: up to five tiles, with
// the day's caption above them when it is the day's first row.
type mediaGridRow struct {
	day   string // "" for a day's later rows
	items []int  // indexes into MediaPage.media
}

var mediaRowModelType = gioutil.NewListModelType[mediaGridRow]()

// mediaGridRows lays media (oldest first) out newest first, one run of
// rows per day, five tiles to a row.
func mediaGridRows(media []client.Message, now time.Time) []mediaGridRow {
	var rows []mediaGridRow
	day := ""
	for i := len(media) - 1; i >= 0; i-- {
		label := mediaDayLabel(media[i].TS, now)
		if len(rows) == 0 || label != day || len(rows[len(rows)-1].items) == mediaPaneCols {
			var row mediaGridRow
			if len(rows) == 0 || label != day {
				row.day = label
			}
			rows = append(rows, row)
			day = label
		}
		last := &rows[len(rows)-1]
		last.items = append(last.items, i)
	}
	return rows
}

// MediaPage is the mockup's right-pane "Media, links and docs": a ← header
// carrying a segmented Media|Links|Docs pill, then a five-column grid of
// square tiles or a list of 38px-icon rows. A tile or a document opens in
// the attachment viewer (see OnOpen).
type MediaPage struct {
	*gtk.Box

	c     client.Client
	jid   string
	stack *gtk.Stack
	body  map[string]*gtk.Box
	// gen drops the result of a load the page has since moved on from.
	gen int

	// The Media tab: a list view of tile rows over media, so a chat with
	// thousands of pictures builds only the rows on screen.
	mediaStack *gtk.Stack // "grid", "empty" or "loading"
	mediaList  *gtk.ListView
	model      *gioutil.ListModel[mediaGridRow]
	rowWidgets map[uintptr]*mediaRowWidget
	media      []client.Message // oldest first, the viewer's order
	docs       []client.Message // oldest first
	mediaIdx   map[string]int   // message id → index in media

	faces  *textureCache
	thumbs *thumbQueue
	// failed holds the keys whose face could not be made, so a tile
	// scrolled past again does not ask the server again while the page
	// stays open.
	failed map[string]bool

	onOpen func(jid string, items []client.Message, msgID string)
}

// mediaRowWidget is a list row's widgets, built once and rebound.
type mediaRowWidget struct {
	root  *gtk.Box
	day   *gtk.Label
	tiles [mediaPaneCols]*mediaTile
}

// mediaTile is one square tile: a blank square that sets its size, the
// face covering it, the kind glyph while there is no face, and the date
// and a clip's length over it.
type mediaTile struct {
	btn   *gtk.Button
	box   *gtk.Overlay
	face  *gtk.Picture
	glyph *gtk.Label
	date  *gtk.Label
	dur   *gtk.Label
	msgID string // "" for an unused slot
	key   string // the face's cache key while bound
}

// NewMediaPage builds the page. onBack closes it.
func NewMediaPage(c client.Client, onBack func()) *MediaPage {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	p := &MediaPage{
		Box: root, c: c, body: map[string]*gtk.Box{},
		rowWidgets: map[uintptr]*mediaRowWidget{},
		faces:      newTextureCache(mediaPaneFaceBudget),
		thumbs:     newThumbQueue(2, func(f func()) { glib.IdleAdd(f) }),
		failed:     map[string]bool{},
	}

	p.stack = gtk.NewStack()
	p.stack.SetVExpand(true)
	for _, page := range mediaPanePages {
		if page.ID == "media" {
			p.stack.AddNamed(p.buildMediaTab(), page.ID)
			continue
		}
		content := gtk.NewBox(gtk.OrientationVertical, 0)
		content.AddCSSClass("chatot-pane-body")
		p.body[page.ID] = content

		scroller := gtk.NewScrolledWindow()
		scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
		scroller.SetVExpand(true)
		scroller.SetChild(content)
		p.stack.AddNamed(scroller, page.ID)
	}

	header := gtk.NewBox(gtk.OrientationHorizontal, 10)
	header.AddCSSClass("chatot-pane-header")

	back := gtk.NewButtonWithLabel("←")
	back.AddCSSClass("flat")
	back.AddCSSClass("chatot-pane-back")
	back.ConnectClicked(onBack)
	header.Append(back)

	title := gtk.NewLabel("Media, links and docs")
	title.SetXAlign(0)
	title.SetHExpand(true)
	// The title gives way before the segmented switcher does: in the
	// collapsed 360px window it wraps onto two lines, as in the mockup.
	title.SetWrap(true)
	title.SetWrapMode(pango.WrapWordChar)
	title.SetLines(2)
	title.SetEllipsize(pango.EllipsizeEnd)
	title.AddCSSClass("chatot-pane-title")
	header.Append(title)

	header.Append(newSegmentedSwitcher(p.stack, mediaPanePages, false))
	root.Append(header)
	root.Append(p.stack)
	return p
}

// OnOpen sets what a tile or a document does: open msgID in the viewer,
// among items (the tab's attachments, oldest first).
func (p *MediaPage) OnOpen(f func(jid string, items []client.Message, msgID string)) {
	p.onOpen = f
}

// buildMediaTab is the Media tab: the tile list, its empty state, and a
// spinner while a chat's media is read.
func (p *MediaPage) buildMediaTab() gtk.Widgetter {
	p.model = mediaRowModelType.New()

	factory := gtk.NewSignalListItemFactory()
	factory.ConnectSetup(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		item.SetActivatable(false)
		item.SetSelectable(false)
		item.SetFocusable(false)
		w := p.newRowWidget()
		p.rowWidgets[widgetKey(w.root)] = w
		item.SetChild(w.root)
	})
	widgetOf := func(item *gtk.ListItem) *mediaRowWidget {
		if child := item.Child(); child != nil {
			return p.rowWidgets[widgetKey(child)]
		}
		return nil
	}
	factory.ConnectBind(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		pos := int(item.Position())
		if w := widgetOf(item); w != nil && pos >= 0 && pos < p.model.Len() {
			p.bindRow(w, p.model.At(pos))
		}
	})
	factory.ConnectUnbind(func(obj *glib.Object) {
		if w := widgetOf(obj.Cast().(*gtk.ListItem)); w != nil {
			for _, t := range w.tiles {
				p.unbindTile(t)
			}
		}
	})
	factory.ConnectTeardown(func(obj *glib.Object) {
		if w := widgetOf(obj.Cast().(*gtk.ListItem)); w != nil {
			delete(p.rowWidgets, widgetKey(w.root))
		}
	})

	p.mediaList = gtk.NewListView(gtk.NewNoSelection(p.model), &factory.ListItemFactory)
	p.mediaList.AddCSSClass("chatot-pane-body")
	p.mediaList.AddCSSClass("chatot-mediagrid-list")
	p.mediaList.SetFocusable(false)
	// Rows at their natural height: at its minimum a tile's picture is
	// only its 56px size request, not the square its width calls for.
	p.mediaList.SetVScrollPolicy(gtk.ScrollNatural)

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetChild(p.mediaList)

	p.mediaStack = gtk.NewStack()
	p.mediaStack.AddNamed(scroller, "grid")
	p.mediaStack.AddNamed(newPaneEmptyState("🖼", "No media yet", "Photos and videos from this chat land here."), "empty")
	spinner := adw.NewSpinner()
	spinner.SetSizeRequest(32, 32)
	spinner.SetHAlign(gtk.AlignCenter)
	spinner.SetVAlign(gtk.AlignCenter)
	p.mediaStack.AddNamed(spinner, "loading")
	p.mediaStack.SetVisibleChildName("loading")
	return p.mediaStack
}

// Load points the page at a chat and rebuilds all three tabs. Called each
// time the page is shown so a newly-downloaded attachment appears. The
// store is read off the main loop: a chat with years of pictures and
// links takes a moment to gather.
func (p *MediaPage) Load(jid string) {
	if jid != p.jid {
		p.thumbs.clear()
	}
	// A preview that failed (offline, say) is tried again each time the
	// page opens.
	p.failed = map[string]bool{}
	p.jid = jid
	p.gen++
	gen := p.gen
	p.stack.SetVisibleChildName("media")
	p.mediaStack.SetVisibleChildName("loading")
	removeAllChildren(p.body["links"])
	removeAllChildren(p.body["docs"])

	c := p.c
	go func() {
		media, err := c.ChatAttachments(jid, "image", "video")
		if err != nil {
			log.Printf("chatot: media page: %v", err)
		}
		docs, err := c.ChatAttachments(jid, "document")
		if err != nil {
			log.Printf("chatot: media page: %v", err)
		}
		links, err := c.ChatLinks(jid)
		if err != nil {
			log.Printf("chatot: media page: %v", err)
		}
		glib.IdleAdd(func() {
			if gen != p.gen {
				return
			}
			p.fillMedia(media)
			p.docs = docs
			p.fillLinks(p.body["links"], links)
			p.fillDocs(p.body["docs"])
		})
	}()
}

// SetLocalPath records that msgID's attachment was downloaded to path (by
// the viewer), so its tile can show the picture itself and the viewer,
// opened from here again, finds the file.
func (p *MediaPage) SetLocalPath(msgID, path string) {
	if i := slices.IndexFunc(p.docs, func(m client.Message) bool { return m.ID == msgID }); i >= 0 && p.docs[i].Attachment != nil {
		a := *p.docs[i].Attachment
		a.LocalPath = path
		p.docs[i].Attachment = &a
		p.fillDocs(p.body["docs"])
		return
	}
	i, ok := p.mediaIdx[msgID]
	if !ok || p.media[i].Attachment == nil {
		return
	}
	a := *p.media[i].Attachment
	a.LocalPath = path
	p.media[i].Attachment = &a
	key := p.faceKey(msgID)
	delete(p.failed, key)
	p.faces.drop(key)
	p.rebindTiles(msgID)
}

// Remove drops a message deleted (here, on the phone, or for everyone)
// from the page, as the viewer drops it.
func (p *MediaPage) Remove(chatJID, msgID string) {
	if chatJID != p.jid {
		return
	}
	if i := slices.IndexFunc(p.docs, func(m client.Message) bool { return m.ID == msgID }); i >= 0 {
		p.docs = slices.Delete(p.docs, i, i+1)
		p.fillDocs(p.body["docs"])
	}
	if i, ok := p.mediaIdx[msgID]; ok {
		p.setMedia(slices.Delete(p.media, i, i+1))
	}
}

func (p *MediaPage) fillMedia(media []client.Message) {
	if p.setMedia(media) {
		p.mediaList.ScrollTo(0, gtk.ListScrollNone, nil)
	}
}

// setMedia lays media out in the grid, reporting whether there is any.
func (p *MediaPage) setMedia(media []client.Message) bool {
	p.media = media
	p.mediaIdx = make(map[string]int, len(media))
	for i, m := range media {
		p.mediaIdx[m.ID] = i
	}
	rows := mediaGridRows(media, time.Now())
	p.model.Splice(0, p.model.Len(), rows...)
	if len(rows) == 0 {
		p.mediaStack.SetVisibleChildName("empty")
		return false
	}
	p.mediaStack.SetVisibleChildName("grid")
	return true
}

func (p *MediaPage) newRowWidget() *mediaRowWidget {
	w := &mediaRowWidget{root: gtk.NewBox(gtk.OrientationVertical, 0)}
	w.root.SetMarginBottom(8)
	w.day = gtk.NewLabel("")
	w.day.AddCSSClass("chatot-mediagrid-day")
	w.day.SetXAlign(0)
	w.root.Append(w.day)

	// Five equal columns that share the pane's width, like the mockup's
	// repeat(5,1fr). A slot a short row leaves unused stays in the grid,
	// invisible, so the columns keep their width.
	grid := gtk.NewGrid()
	grid.SetColumnHomogeneous(true)
	grid.SetColumnSpacing(8)
	grid.SetHExpand(true)
	for col := range w.tiles {
		t := p.newTile()
		w.tiles[col] = t
		grid.Attach(t.btn, col, 0, 1, 1)
	}
	w.root.Append(grid)
	return w
}

// squareTexture is a 1×1 transparent texture: a GtkPicture showing it asks
// for a height equal to its width, which is how a tile stays square at
// whatever width its column gets.
var squareTexture = gdk.NewMemoryTexture(1, 1, gdk.MemoryB8G8R8A8Premultiplied, glib.NewBytesWithGo([]byte{0, 0, 0, 0}), 4)

func (p *MediaPage) newTile() *mediaTile {
	t := &mediaTile{box: gtk.NewOverlay()}
	t.box.AddCSSClass("chatot-mediagrid-tile")
	t.box.SetOverflow(gtk.OverflowHidden)

	// The square sets the tile's size; the face is an overlay, so a
	// landscape or portrait picture never changes it.
	square := gtk.NewPictureForPaintable(squareTexture)
	square.SetCanShrink(true)
	square.SetSizeRequest(mediaPaneTile, mediaPaneTile)
	square.SetHExpand(true)
	t.box.SetChild(square)

	t.face = gtk.NewPicture()
	t.face.SetCanShrink(true)
	t.face.SetContentFit(gtk.ContentFitCover)
	t.box.AddOverlay(t.face)

	t.glyph = gtk.NewLabel("")
	t.glyph.AddCSSClass("chatot-mediagrid-glyph")
	t.glyph.SetHAlign(gtk.AlignCenter)
	t.glyph.SetVAlign(gtk.AlignCenter)
	t.box.AddOverlay(t.glyph)

	t.date = gtk.NewLabel("")
	t.date.AddCSSClass("chatot-mediagrid-date")
	t.date.SetHAlign(gtk.AlignStart)
	t.date.SetVAlign(gtk.AlignEnd)
	t.box.AddOverlay(t.date)

	t.dur = gtk.NewLabel("")
	t.dur.AddCSSClass("chatot-mediagrid-date")
	t.dur.AddCSSClass("chatot-mediagrid-length")
	t.dur.SetHAlign(gtk.AlignEnd)
	t.dur.SetVAlign(gtk.AlignEnd)
	t.box.AddOverlay(t.dur)

	t.btn = gtk.NewButton()
	t.btn.SetChild(t.box)
	t.btn.AddCSSClass("flat")
	t.btn.AddCSSClass("chatot-card-btn")
	t.btn.SetHExpand(true)
	t.btn.ConnectClicked(func() {
		if t.msgID != "" && p.onOpen != nil {
			p.onOpen(p.jid, p.media, t.msgID)
		}
	})
	return t
}

func (p *MediaPage) bindRow(w *mediaRowWidget, row mediaGridRow) {
	w.day.SetVisible(row.day != "")
	w.day.SetLabel(row.day)
	now := time.Now()
	for col, t := range w.tiles {
		p.unbindTile(t)
		if col >= len(row.items) {
			t.msgID = ""
			t.btn.SetOpacity(0)
			t.btn.SetCanTarget(false)
			t.btn.SetCanFocus(false)
			continue
		}
		t.btn.SetOpacity(1)
		t.btn.SetCanTarget(true)
		t.btn.SetCanFocus(true)
		p.bindTile(t, p.media[row.items[col]], now)
	}
}

func (p *MediaPage) bindTile(t *mediaTile, m client.Message, now time.Time) {
	t.msgID = m.ID
	t.key = p.faceKey(m.ID)
	p.thumbs.want(t.key)
	t.date.SetLabel(mediaTileLabelFor(m.TS, now))
	kind := ""
	if m.Attachment != nil {
		kind = m.Attachment.Kind
	}
	clip := kind == "video" && m.Attachment.DurationSecs > 0
	t.dur.SetVisible(clip)
	if clip {
		t.dur.SetLabel(humanDuration(m.Attachment.DurationSecs))
	}
	t.glyph.SetLabel(mediaKindGlyph(kind))
	if tex, ok := p.faces.get(t.key); ok {
		p.showFace(t, tex)
		return
	}
	p.showFace(t, nil)
	if !p.failed[t.key] {
		p.requestFace(t.key, m)
	}
}

// unbindTile lets a tile's face request lapse once it leaves the screen.
func (p *MediaPage) unbindTile(t *mediaTile) {
	if t.key != "" {
		p.thumbs.unwant(t.key)
		t.key = ""
	}
}

// showFace puts tex on the tile, or its kind glyph on a tinted square
// when there is none yet.
func (p *MediaPage) showFace(t *mediaTile, tex *gdk.Texture) {
	if tex == nil {
		t.face.SetPaintable(nil)
		t.box.AddCSSClass("chatot-mediagrid-placeholder")
		t.glyph.SetVisible(true)
		return
	}
	t.face.SetPaintable(tex)
	t.box.RemoveCSSClass("chatot-mediagrid-placeholder")
	t.glyph.SetVisible(false)
}

// rebindTiles refreshes the bound tiles showing msgID.
func (p *MediaPage) rebindTiles(msgID string) {
	i, ok := p.mediaIdx[msgID]
	if !ok {
		return
	}
	now := time.Now()
	for _, w := range p.rowWidgets {
		for _, t := range w.tiles {
			if t.msgID == msgID && t.key != "" {
				p.unbindTile(t)
				p.bindTile(t, p.media[i], now)
			}
		}
	}
}

// faceKey is the face cache's key for msgID in the page's chat.
func (p *MediaPage) faceKey(msgID string) string { return p.jid + "/" + msgID }

// requestFace queues the decode (or fetch) that makes m's tile face: the
// embedded thumbnail, the downloaded picture when that thumbnail is only a
// stamp or missing, and failing both the sharper preview WhatsApp keeps on
// its servers. A clip with no thumbnail and no preview keeps its glyph.
func (p *MediaPage) requestFace(key string, m client.Message) {
	a := m.Attachment
	if a == nil {
		return
	}
	thumb := a.Thumbnail
	path := ""
	if a.Kind == "image" {
		path, _ = hasLocal(m)
	}
	fetch := len(thumb) == 0 && path == ""
	c, id := p.c, m.ID
	p.thumbs.push(thumbJob{key: key, run: func() func() {
		var jpeg []byte
		if fetch {
			var err error
			// Bounded: the fetch holds one of the queue's two workers.
			ctx, cancel := context.WithTimeout(context.Background(), mediaPaneFetchTimeout)
			jpeg, err = c.DownloadThumbnail(ctx, id)
			cancel()
			if err != nil {
				if !errors.Is(err, client.ErrNoThumbnail) {
					log.Printf("chatot: media page preview for %s: %v", id, err)
				}
				return func() { p.markFailed(key) }
			}
			thumb = jpeg
		}
		pb := decodeTileFace(thumb, path)
		return func() {
			if jpeg != nil {
				p.setThumbnail(id, jpeg)
			}
			if pb == nil {
				p.markFailed(key)
				return
			}
			tex := gdk.NewTextureForPixbuf(pb)
			p.faces.put(key, tex, textureBytes(tex))
			for _, w := range p.rowWidgets {
				for _, t := range w.tiles {
					if t.key == key {
						p.showFace(t, tex)
					}
				}
			}
		}
	}})
}

// markFailed records that key's face could not be made, unless another
// request made one meanwhile.
func (p *MediaPage) markFailed(key string) {
	if _, ok := p.faces.get(key); !ok {
		p.failed[key] = true
	}
}

// setThumbnail keeps a fetched preview on the page's copy of the message,
// so the viewer opened from here shows it too.
func (p *MediaPage) setThumbnail(msgID string, jpeg []byte) {
	i, ok := p.mediaIdx[msgID]
	if !ok || p.media[i].Attachment == nil {
		return
	}
	a := *p.media[i].Attachment
	a.Thumbnail = jpeg
	p.media[i].Attachment = &a
}

// decodeTileFace makes a tile's picture: thumb when it is sharp enough,
// else the file at path, else thumb however small. nil when neither
// decodes. Runs off the main loop.
func decodeTileFace(thumb []byte, path string) *gdkpixbuf.Pixbuf {
	var stamp *gdkpixbuf.Pixbuf
	if len(thumb) > 0 {
		pb, err := smallPixbufFromBytes(thumb, mediaPaneFace)
		if err == nil && (path == "" || min(pb.Width(), pb.Height()) >= mediaPaneStampSide) {
			return pb
		}
		stamp = pb
	}
	if path != "" {
		pb, err := gdkpixbufAtScale(path, 2*mediaPaneFace)
		if err == nil {
			pb = pb.ApplyEmbeddedOrientation()
			if w, h := coverSide(pb.Width(), pb.Height(), mediaPaneFace); w < pb.Width() && h < pb.Height() {
				pb = pb.ScaleSimple(w, h, gdkpixbuf.InterpBilinear)
			}
			return pb
		}
		log.Printf("chatot: media page tile for %s: %v", path, err)
	}
	return stamp
}

// mediaDayLabel is a day section's caption: the thread's own separator
// wording (Today, Yesterday, a weekday, then a date) so both surfaces
// name a day the same way.
func mediaDayLabel(ts int64, now time.Time) string {
	if ts == 0 {
		return "Undated"
	}
	return dayText(ts, now)
}

func (p *MediaPage) fillLinks(box *gtk.Box, items []client.LinkItem) {
	removeAllChildren(box)
	if len(items) == 0 {
		box.Append(newPaneEmptyState("🔗", "No links yet", "Links shared in this chat collect here."))
		return
	}
	for _, l := range items {
		url := l.URL
		box.Append(newPaneListRow("🔗", l.Title, l.Host,
			time.Unix(l.TS, 0).Format("2 Jan"), func() { openURI(url) }))
	}
}

// fillDocs lists p.docs newest first; a row opens the document in the
// viewer, where a PDF pages through and any file can be opened or saved.
func (p *MediaPage) fillDocs(box *gtk.Box) {
	removeAllChildren(box)
	if len(p.docs) == 0 {
		box.Append(newPaneEmptyState("📄", "No documents yet", "Files shared in this chat collect here."))
		return
	}
	for _, d := range slices.Backward(p.docs) {
		a := d.Attachment
		size := humanSize(a.Size)
		if size == "" {
			size = docSize(a.LocalPath)
		}
		sub := joinMeta(docKindLabel(a.MimeType), size)
		id := d.ID
		box.Append(newPaneListRow("📄", a.Filename, sub,
			time.Unix(d.TS, 0).Format("2 Jan"), func() {
				if p.onOpen != nil {
					p.onOpen(p.jid, p.docs, id)
				}
			}))
	}
}

// docSize renders a downloaded document's on-disk size, empty when it isn't
// cached.
func docSize(path string) string {
	if size, ok := localFileSize(path); ok {
		return humanSize(size)
	}
	return ""
}

// newPaneListRow is the mockup's Links/Docs row: a 38px rounded icon square,
// a bold title over a dim subtitle, and the date at the right.
func newPaneListRow(glyph, title, sub, date string, onClick func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)

	icon := gtk.NewLabel(glyph)
	icon.AddCSSClass("chatot-pane-rowicon")
	icon.SetSizeRequest(38, 38)
	icon.SetVAlign(gtk.AlignCenter)
	row.Append(icon)

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(title)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.AddCSSClass("chatot-pane-rowtitle")
	col.Append(name)
	if sub != "" {
		s := gtk.NewLabel(sub)
		s.SetXAlign(0)
		s.SetEllipsize(pango.EllipsizeEnd)
		s.AddCSSClass("chatot-pane-rowsub")
		col.Append(s)
	}
	row.Append(col)

	when := gtk.NewLabel(date)
	when.AddCSSClass("chatot-pane-rowdate")
	when.SetVAlign(gtk.AlignCenter)
	row.Append(when)

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-pane-row")
	btn.ConnectClicked(onClick)
	return btn
}

// newPaneEmptyState is the shared centred placeholder for an empty pane tab,
// matching the starred page's.
func newPaneEmptyState(glyph, title, hint string) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.AddCSSClass("chatot-pane-empty")
	box.SetVExpand(true)
	box.SetVAlign(gtk.AlignCenter)
	box.SetHAlign(gtk.AlignCenter)

	disc := gtk.NewLabel(glyph)
	disc.AddCSSClass("chatot-pane-empty-disc")
	disc.SetSizeRequest(56, 56)
	disc.SetHAlign(gtk.AlignCenter)
	box.Append(disc)

	t := gtk.NewLabel(title)
	t.AddCSSClass("chatot-pane-empty-title")
	box.Append(t)

	h := gtk.NewLabel(hint)
	h.AddCSSClass("chatot-pane-empty-hint")
	h.SetWrap(true)
	h.SetJustify(gtk.JustifyCenter)
	h.SetMaxWidthChars(34)
	box.Append(h)
	return box
}
