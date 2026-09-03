package ui

import (
	"context"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// mediaPaneCols is the mockup's five equal columns; mediaPaneTile is the
// smallest a tile may get before the pane scrolls sideways instead.
const (
	mediaPaneCols = 5
	mediaPaneTile = 96
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

// MediaPage is the mockup's right-pane "Media, links and docs": a ← header
// carrying a segmented Media|Links|Docs pill, then a five-column grid of
// square tiles or a list of 38px-icon rows.
//
// It replaces a separate 420px window with a StackSwitcher, which the design
// never had — media belongs in the content pane beside its chat.
type MediaPage struct {
	*gtk.Box

	c     client.Client
	jid   string
	stack *gtk.Stack
	body  map[string]*gtk.Box
}

// NewMediaPage builds the page. onBack closes it.
func NewMediaPage(c client.Client, onBack func()) *MediaPage {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	p := &MediaPage{Box: root, c: c, body: map[string]*gtk.Box{}}

	p.stack = gtk.NewStack()
	p.stack.SetVExpand(true)
	for _, page := range mediaPanePages {
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
	title.AddCSSClass("chatot-pane-title")
	header.Append(title)

	header.Append(newSegmentedSwitcher(p.stack, mediaPanePages, false))
	root.Append(header)
	root.Append(p.stack)
	return p
}

// Load points the page at a chat and rebuilds all three tabs. Called each
// time the page is shown so a newly-downloaded attachment appears.
func (p *MediaPage) Load(jid string) {
	p.jid = jid
	p.stack.SetVisibleChildName("media")

	media, _ := p.c.ChatMedia(jid)
	links, _ := p.c.ChatLinks(jid)
	docs, _ := p.c.ChatDocs(jid)

	p.fill("media", func(box *gtk.Box) { p.fillMedia(box, media) })
	p.fill("links", func(box *gtk.Box) { p.fillLinks(box, links) })
	p.fill("docs", func(box *gtk.Box) { p.fillDocs(box, docs) })
}

// fill clears a tab and repopulates it.
func (p *MediaPage) fill(id string, build func(*gtk.Box)) {
	box := p.body[id]
	removeAllChildren(box)
	build(box)
}

func (p *MediaPage) fillMedia(box *gtk.Box, items []client.MediaItem) {
	if len(items) == 0 {
		box.Append(newPaneEmptyState("🖼", "No media yet", "Photos and videos from this chat land here."))
		return
	}
	// One grid per day, under a small caption, newest day first (items
	// arrive newest first from the client).
	now := time.Now()
	var grid *gtk.Grid
	day := ""
	n := 0
	for _, item := range items {
		if label := mediaDayLabel(item.TS, now); grid == nil || label != day {
			day = label
			caption := gtk.NewLabel(label)
			caption.AddCSSClass("chatot-mediagrid-day")
			caption.SetXAlign(0)
			box.Append(caption)
			grid = newMediaGrid()
			box.Append(grid)
			n = 0
		}
		grid.Attach(p.newGridTile(item, now), n%mediaPaneCols, n/mediaPaneCols, 1, 1)
		n++
	}
	padMediaGrid(grid, n)
}

// padMediaGrid fills the first row's unused columns with empty expanders so
// a day with fewer than five items still lays out as five equal columns
// (column-homogeneous only equalises columns that exist).
func padMediaGrid(grid *gtk.Grid, n int) {
	for col := n; col < mediaPaneCols; col++ {
		filler := gtk.NewBox(gtk.OrientationHorizontal, 0)
		filler.SetHExpand(true)
		grid.Attach(filler, col, 0, 1, 1)
	}
}

// newMediaGrid is one day's tile grid: five equal columns that share the
// pane's width, like the mockup's repeat(5,1fr). (A GtkFlowBox keeps its
// children at their natural width and leaves the slack unused.)
func newMediaGrid() *gtk.Grid {
	grid := gtk.NewGrid()
	grid.SetColumnHomogeneous(true)
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(8)
	grid.SetHExpand(true)
	return grid
}

// squareTexture is a 1×1 transparent texture: a GtkPicture showing it asks
// for a height equal to its width, which is how a placeholder tile stays
// square at whatever width its column gets.
var squareTexture = gdk.NewMemoryTexture(1, 1, gdk.MemoryB8G8R8A8Premultiplied, glib.NewBytesWithGo([]byte{0, 0, 0, 0}), 4)

// mediaDayLabel is a day section's caption: the thread's own separator
// wording (Today, Yesterday, a weekday, then a date) so both surfaces
// name a day the same way.
func mediaDayLabel(ts int64, now time.Time) string {
	if ts == 0 {
		return "Undated"
	}
	return dayText(ts, now)
}

// newGridTile is one square tile: the embedded thumbnail (or a kind glyph)
// with the date overlaid in its bottom-left corner, per the mockup.
func (p *MediaPage) newGridTile(item client.MediaItem, now time.Time) gtk.Widgetter {
	overlay := gtk.NewOverlay()

	// The tile's body is always a square picture (the thumbnail, or a blank
	// square under a kind glyph) so its height follows the column width.
	var paintable gdk.Paintabler = squareTexture
	if len(item.Thumbnail) > 0 {
		if texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(item.Thumbnail)); err == nil {
			paintable = texture
		}
	}
	pic := gtk.NewPictureForPaintable(paintable)
	pic.SetCanShrink(true)
	pic.SetContentFit(gtk.ContentFitCover)
	pic.SetSizeRequest(mediaPaneTile, mediaPaneTile)
	pic.SetHExpand(true)
	pic.AddCSSClass("chatot-mediagrid-tile")
	if paintable == gdk.Paintabler(squareTexture) {
		pic.AddCSSClass("chatot-mediagrid-placeholder")
		glyph := gtk.NewLabel(mediaKindGlyph(item.Kind))
		glyph.AddCSSClass("chatot-mediagrid-glyph")
		glyph.SetHAlign(gtk.AlignCenter)
		glyph.SetVAlign(gtk.AlignCenter)
		overlay.AddOverlay(glyph)
	}
	overlay.SetChild(pic)

	if label := mediaTileLabelFor(item.TS, now); label != "" {
		caption := gtk.NewLabel(label)
		caption.AddCSSClass("chatot-mediagrid-date")
		caption.SetHAlign(gtk.AlignStart)
		caption.SetVAlign(gtk.AlignEnd)
		overlay.AddOverlay(caption)
	}

	// Square, and stretching with the pane like the mockup's repeat(5,1fr)
	// columns: the frame keeps the aspect while the flow box shares the
	// width out.
	btn := gtk.NewButton()
	btn.SetChild(overlay)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btn")
	btn.SetHExpand(true)
	local, msgID := item.LocalPath, item.MsgID
	btn.ConnectClicked(func() {
		if local != "" {
			openFile(local)
			return
		}
		btn.SetSensitive(false)
		go func() {
			path, err := p.c.DownloadMedia(context.Background(), msgID)
			glib.IdleAdd(func() {
				btn.SetSensitive(true)
				if err != nil {
					return
				}
				local = path
				openFile(local)
			})
		}()
	})
	return btn
}

func (p *MediaPage) fillLinks(box *gtk.Box, items []client.LinkItem) {
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

func (p *MediaPage) fillDocs(box *gtk.Box, items []client.DocItem) {
	if len(items) == 0 {
		box.Append(newPaneEmptyState("📄", "No documents yet", "Files shared in this chat collect here."))
		return
	}
	for _, d := range items {
		sub := joinMeta(docKindLabel(d.MimeType), docSize(d.LocalPath))
		local, msgID := d.LocalPath, d.MsgID
		box.Append(newPaneListRow("📄", d.Filename, sub,
			time.Unix(d.TS, 0).Format("2 Jan"), func() {
				if local != "" {
					openFile(local)
					return
				}
				go func() {
					path, err := p.c.DownloadMedia(context.Background(), msgID)
					if err != nil {
						return
					}
					glib.IdleAdd(func() { openFile(path) })
				}()
			}))
	}
}

// docSize renders a downloaded document's on-disk size, empty when it isn't
// cached (the store has never persisted a remote size for these rows).
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
