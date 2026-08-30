package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// mediaTileSize is the fixed square footprint of a Media-tab thumbnail tile.
const mediaTileSize = 96

// mediaMonthGroup is one month's worth of media items, in display order
// (newest-first overall, oldest-to-newest not required within a month).
type mediaMonthGroup struct {
	Header string
	Items  []client.MediaItem
}

// groupMediaByMonth buckets items (assumed newest-first, as ChatMedia/
// ChatMedia's Fake counterpart return them) into consecutive month groups,
// each headed by an upper-cased "MONTH YEAR" label matching the mockup.
// Consecutive-run bucketing (rather than a map) preserves newest-first group
// order without needing a second sort.
func groupMediaByMonth(items []client.MediaItem) []mediaMonthGroup {
	var groups []mediaMonthGroup
	var curKey string
	for _, it := range items {
		t := time.Unix(it.TS, 0)
		key := t.Format("2006-01")
		if key != curKey || len(groups) == 0 {
			groups = append(groups, mediaMonthGroup{Header: strings.ToUpper(t.Format("January 2006"))})
			curKey = key
		}
		g := &groups[len(groups)-1]
		g.Items = append(g.Items, it)
	}
	return groups
}

// docSubtitle renders a doc row's second line: "PDF · 1.2 MB · Aug 12, 2026",
// omitting the size segment when the file isn't downloaded (no LocalPath to
// stat — the store never persists a remote file's size).
func docSubtitle(d client.DocItem) string {
	parts := []string{docKindLabel(d.MimeType)}
	if size, ok := localFileSize(d.LocalPath); ok {
		parts = append(parts, formatFileSize(size))
	}
	parts = append(parts, time.Unix(d.TS, 0).Format("Jan 2, 2006"))
	return strings.Join(parts, " · ")
}

// docKindLabel derives a short uppercase type tag from a MIME type (e.g.
// "application/pdf" -> "PDF"), falling back to "FILE" if it can't parse one.
func docKindLabel(mime string) string {
	slash := strings.LastIndex(mime, "/")
	if slash < 0 || slash == len(mime)-1 {
		return "FILE"
	}
	sub := mime[slash+1:]
	if plus := strings.Index(sub, "+"); plus > 0 {
		sub = sub[:plus]
	}
	return strings.ToUpper(sub)
}

// formatFileSize renders n bytes as a human-readable "1.2 MB"-style string.
func formatFileSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// linkSubtitle renders a link row's second line: "stay.example.com · Aug 12, 2026".
func linkSubtitle(l client.LinkItem) string {
	return l.Host + " · " + time.Unix(l.TS, 0).Format("Jan 2, 2006")
}

// ShowMediaPage opens the "Media, links and docs" page for jid: a modal
// window with three tabs (Media/Links/Docs, each labeled with its count)
// backed by the store's ChatMedia/ChatDocs/ChatLinks reads.
func ShowMediaPage(parent *gtk.Window, c client.Client, jid, contactName string) {
	media, _ := c.ChatMedia(jid)
	links, _ := c.ChatLinks(jid)
	docs, _ := c.ChatDocs(jid)

	dialog := gtk.NewWindow()
	dialog.SetTitle("Media, links and docs")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(420, 520)

	root := gtk.NewBox(gtk.OrientationVertical, 8)
	root.SetMarginTop(12)
	root.SetMarginBottom(12)
	root.SetMarginStart(12)
	root.SetMarginEnd(12)

	heading := gtk.NewLabel("Media, links and docs")
	heading.SetXAlign(0)
	heading.AddCSSClass("chatot-conv-title")
	root.Append(heading)

	subtitle := gtk.NewLabel(contactName)
	subtitle.SetXAlign(0)
	subtitle.AddCSSClass("chatot-conv-subtitle")
	root.Append(subtitle)

	stack := gtk.NewStack()
	stack.SetVExpand(true)
	switcher := gtk.NewStackSwitcher()
	switcher.SetStack(stack)
	switcher.SetHAlign(gtk.AlignCenter)

	stack.AddTitled(buildMediaTab(media, c), "media", fmt.Sprintf("Media %d", len(media)))
	stack.AddTitled(buildLinksTab(links), "links", fmt.Sprintf("Links %d", len(links)))
	stack.AddTitled(buildDocsTab(docs), "docs", fmt.Sprintf("Docs %d", len(docs)))

	root.Append(switcher)
	root.Append(stack)

	dialog.SetChild(root)
	dialog.Present()
}

// buildMediaTab renders the Media tab: a scroller of month-grouped FlowBoxes
// of thumbnail tiles, or an empty-state label if jid has no media.
func buildMediaTab(items []client.MediaItem, c client.Client) gtk.Widgetter {
	if len(items) == 0 {
		return emptyStateLabel("No media yet")
	}

	list := gtk.NewBox(gtk.OrientationVertical, 12)
	list.SetMarginTop(8)
	for _, group := range groupMediaByMonth(items) {
		header := gtk.NewLabel(group.Header)
		header.SetXAlign(0)
		header.AddCSSClass("heading")
		list.Append(header)

		flow := gtk.NewFlowBox()
		flow.SetSelectionMode(gtk.SelectionNone)
		flow.SetMaxChildrenPerLine(4)
		flow.SetRowSpacing(6)
		flow.SetColumnSpacing(6)
		for _, item := range group.Items {
			flow.Append(buildMediaTile(item, c))
		}
		list.Append(flow)
	}

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetChild(list)
	return scroller
}

// buildMediaTile renders one Media-tab thumbnail: the embedded thumbnail if
// present, else a kind placeholder. Clicking it opens the file if already
// downloaded, else downloads it via the existing DownloadMedia path (F29)
// and opens it once ready.
func buildMediaTile(item client.MediaItem, c client.Client) gtk.Widgetter {
	btn := gtk.NewButton()
	btn.AddCSSClass("flat")
	btn.SetSizeRequest(mediaTileSize, mediaTileSize)

	setContent := func() gtk.Widgetter {
		if len(item.Thumbnail) > 0 {
			if texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(item.Thumbnail)); err == nil {
				pic := gtk.NewPictureForPaintable(texture)
				pic.SetCanShrink(true)
				pic.SetContentFit(gtk.ContentFitCover)
				pic.SetSizeRequest(mediaTileSize, mediaTileSize)
				return pic
			}
		}
		placeholder := gtk.NewLabel(mediaKindGlyph(item.Kind))
		placeholder.AddCSSClass("dim-label")
		placeholder.SetSizeRequest(mediaTileSize, mediaTileSize)
		return placeholder
	}
	btn.SetChild(setContent())

	local := item.LocalPath
	msgID := item.MsgID
	btn.ConnectClicked(func() {
		if local != "" {
			openFile(local)
			return
		}
		btn.SetSensitive(false)
		go func() {
			path, err := c.DownloadMedia(context.Background(), msgID)
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

// mediaKindGlyph is the placeholder glyph for a Media-tab tile with no
// downloaded thumbnail: a video gets a play mark (there's no stored
// duration to show a proper badge — the schema has never persisted one).
func mediaKindGlyph(kind string) string {
	if kind == "video" {
		return "▶"
	}
	return "🖼"
}

// buildLinksTab renders the Links tab: a list of message-with-URL rows, or
// an empty-state label if jid has no links.
func buildLinksTab(items []client.LinkItem) gtk.Widgetter {
	if len(items) == 0 {
		return emptyStateLabel("No links yet")
	}

	list := gtk.NewListBox()
	list.SetSelectionMode(gtk.SelectionNone)
	list.AddCSSClass("boxed-list")
	for _, l := range items {
		row := gtk.NewBox(gtk.OrientationVertical, 2)
		row.SetMarginTop(8)
		row.SetMarginBottom(8)
		row.SetMarginStart(8)
		row.SetMarginEnd(8)

		title := gtk.NewLabel(l.Title)
		title.SetXAlign(0)
		title.SetWrap(true)
		row.Append(title)

		sub := gtk.NewLabel(linkSubtitle(l))
		sub.SetXAlign(0)
		sub.AddCSSClass("dim-label")
		row.Append(sub)

		list.Append(row)
	}

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetChild(list)
	return scroller
}

// buildDocsTab renders the Docs tab: a list of document rows with an Open
// button when the file is downloaded, or an empty-state label.
func buildDocsTab(items []client.DocItem) gtk.Widgetter {
	if len(items) == 0 {
		return emptyStateLabel("No documents yet")
	}

	list := gtk.NewListBox()
	list.SetSelectionMode(gtk.SelectionNone)
	list.AddCSSClass("boxed-list")
	for _, d := range items {
		row := gtk.NewBox(gtk.OrientationHorizontal, 8)
		row.SetMarginTop(8)
		row.SetMarginBottom(8)
		row.SetMarginStart(8)
		row.SetMarginEnd(8)

		icon := gtk.NewLabel("📄")
		row.Append(icon)

		textCol := gtk.NewBox(gtk.OrientationVertical, 2)
		textCol.SetHExpand(true)
		name := gtk.NewLabel(d.Filename)
		name.SetXAlign(0)
		textCol.Append(name)
		sub := gtk.NewLabel(docSubtitle(d))
		sub.SetXAlign(0)
		sub.AddCSSClass("dim-label")
		textCol.Append(sub)
		row.Append(textCol)

		if d.LocalPath != "" {
			path := d.LocalPath
			open := gtk.NewButtonWithLabel("Open")
			open.AddCSSClass("flat")
			open.ConnectClicked(func() { openFile(path) })
			row.Append(open)
		}

		list.Append(row)
	}

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetChild(list)
	return scroller
}

// emptyStateLabel is the shared "nothing here yet" placeholder for a tab.
func emptyStateLabel(text string) gtk.Widgetter {
	label := gtk.NewLabel(text)
	label.AddCSSClass("dim-label")
	label.SetVAlign(gtk.AlignCenter)
	label.SetHAlign(gtk.AlignCenter)
	label.SetVExpand(true)
	return label
}

// localFileSize stats path and returns its size, ok=false if path is empty
// or the stat fails (evicted cache, moved file, etc.).
func localFileSize(path string) (int64, bool) {
	if path == "" {
		return 0, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}
