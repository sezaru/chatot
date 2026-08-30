package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// stickerRecentsCap is the ring size for the Stickers tab's "Recently used"
// row. Persisting recents across restarts is deferred — this is session-only.
const stickerRecentsCap = 8

// stickerRecents is a most-recently-used ring of sent sticker paths, kept
// free of GTK so it's unit-testable directly.
type stickerRecents struct {
	items []string
	cap   int
}

// newStickerRecents returns an empty ring holding at most capN paths.
func newStickerRecents(capN int) *stickerRecents {
	return &stickerRecents{cap: capN}
}

// Add pushes path to the front of the ring, moving it there if already
// present (dedup) rather than adding a second entry, then trims to cap.
func (r *stickerRecents) Add(path string) {
	for i, p := range r.items {
		if p == path {
			r.items = append(r.items[:i], r.items[i+1:]...)
			break
		}
	}
	r.items = append([]string{path}, r.items...)
	if len(r.items) > r.cap {
		r.items = r.items[:r.cap]
	}
}

// Items returns the ring's contents, most-recent first, as a fresh slice.
func (r *stickerRecents) Items() []string {
	out := make([]string, len(r.items))
	copy(out, r.items)
	return out
}

// stickerFilter matches webp (WhatsApp's sticker format) and, failing that,
// any image — SendSticker sends non-webp files best-effort.
func stickerFilter() *gtk.FileFilter {
	f := gtk.NewFileFilter()
	f.AddMIMEType("image/webp")
	f.AddPattern("*.webp")
	f.AddMIMEType("image/*")
	return f
}

// newStickerTab builds the Stickers tab: a "Recently used" grid fed from
// c.stickerRecents (click to resend), and an "Add sticker…" button that opens
// a file picker and sends+records the pick. No pack marketplace — that's
// explicitly out of scope for this feature. popover is the tab's containing
// popover, closed on every pick/resend and re-read on every reopen so newly
// sent stickers show up without a rebuild of the whole tab.
func newStickerTab(c *Composer, popover *gtk.Popover) gtk.Widgetter {
	root := gtk.NewBox(gtk.OrientationVertical, 6)
	root.SetMarginTop(8)
	root.SetMarginBottom(8)
	root.SetMarginStart(8)
	root.SetMarginEnd(8)
	root.SetSizeRequest(280, 260)

	header := gtk.NewLabel("Recently used")
	header.AddCSSClass("dim-label")
	header.SetXAlign(0)
	root.Append(header)

	flow := gtk.NewFlowBox()
	flow.SetSelectionMode(gtk.SelectionNone)
	flow.SetMaxChildrenPerLine(4)
	flow.SetRowSpacing(4)
	flow.SetColumnSpacing(4)
	flow.SetActivateOnSingleClick(true)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetChild(flow)

	empty := gtk.NewLabel("No recent stickers — add one below")
	empty.AddCSSClass("dim-label")
	empty.SetWrap(true)
	empty.SetVAlign(gtk.AlignCenter)
	empty.SetHAlign(gtk.AlignCenter)
	empty.SetVExpand(true)

	root.Append(scroller)
	root.Append(empty)

	addBtn := gtk.NewButtonWithLabel("Add sticker…")
	addBtn.AddCSSClass("flat")
	root.Append(addBtn)

	var recentPaths []string
	refresh := func() {
		clearFlowBox(flow)
		recentPaths = c.stickerRecents.Items()
		if len(recentPaths) == 0 {
			scroller.SetVisible(false)
			empty.SetVisible(true)
			return
		}
		empty.SetVisible(false)
		scroller.SetVisible(true)
		for _, p := range recentPaths {
			flow.Append(newStickerTile(p))
		}
	}

	flow.ConnectChildActivated(func(child *gtk.FlowBoxChild) {
		i := child.Index()
		if i < 0 || i >= len(recentPaths) {
			return
		}
		popover.Popdown()
		c.sendSticker(recentPaths[i])
	})
	addBtn.ConnectClicked(func() { c.pickSticker(popover) })

	popover.ConnectMap(refresh)
	refresh()

	return root
}

// newStickerTile renders one recent sticker as a small preview thumbnail.
func newStickerTile(path string) gtk.Widgetter {
	pic := gtk.NewPictureForFilename(path)
	pic.SetCanShrink(true)
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetSizeRequest(56, 56)
	pic.AddCSSClass("chatot-sticker")
	return pic
}
