package ui

import (
	"context"
	"log"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// stickerTileHeight is the mockup's tile height in the 4-column grid.
const stickerTileHeight = 74

// stickerFilter matches webp (WhatsApp's sticker format) and, failing that,
// any image — SendSticker sends non-webp files best-effort.
func stickerFilter() *gtk.FileFilter {
	f := gtk.NewFileFilter()
	f.AddMIMEType("image/webp")
	f.AddPattern("*.webp")
	f.AddMIMEType("image/*")
	return f
}

// stickerMenuItems is a sticker tile's right-click menu. A WhatsApp
// favourite is only hidden here (the phone keeps its star), so the row
// says so.
func stickerMenuItems(st client.Sticker, remove func()) []menuItem {
	label := "Remove sticker"
	if st.FromWhatsApp {
		label = "Remove from this device"
	}
	return []menuItem{{Icon: "🗑", Label: label, Destructive: true, OnActivate: remove}}
}

// newStickerTab builds the Stickers tab: the library (files added here and
// the account's WhatsApp favourites, most recently used first) as a 4-column
// grid — click to send, right-click or long-press to remove — over a
// footer with an "Add sticker" chip that opens a file picker. popover is
// the tab's containing popover, closed on every send and re-read on every
// open so favourites the phone just starred show up.
func newStickerTab(c *Composer, popover *gtk.Popover) gtk.Widgetter {
	root := gtk.NewBox(gtk.OrientationVertical, 6)
	root.SetMarginTop(8)
	root.SetMarginBottom(8)
	root.SetMarginStart(8)
	root.SetMarginEnd(8)
	root.SetSizeRequest(280, 260)

	flow := gtk.NewFlowBox()
	flow.AddCSSClass("chatot-picker-grid")
	flow.SetSelectionMode(gtk.SelectionNone)
	flow.SetMinChildrenPerLine(4)
	flow.SetMaxChildrenPerLine(4)
	flow.SetHomogeneous(true)
	flow.SetRowSpacing(6)
	flow.SetColumnSpacing(6)
	flow.SetActivateOnSingleClick(true)
	flow.SetVAlign(gtk.AlignStart)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetChild(flow)

	empty := gtk.NewLabel("No stickers yet\nStar one on your phone, or add a picture")
	empty.AddCSSClass("dim-label")
	empty.SetWrap(true)
	empty.SetJustify(gtk.JustifyCenter)
	empty.SetVAlign(gtk.AlignCenter)
	empty.SetHAlign(gtk.AlignCenter)
	empty.SetVExpand(true)

	root.Append(scroller)
	root.Append(empty)

	footer := gtk.NewBox(gtk.OrientationHorizontal, 8)
	footer.AddCSSClass("chatot-picker-footer")
	caption := gtk.NewLabel("Recently used")
	caption.AddCSSClass("dim-label")
	caption.SetXAlign(0)
	caption.SetHExpand(true)
	footer.Append(caption)
	addBtn := gtk.NewButtonWithLabel("Add sticker")
	addBtn.AddCSSClass("chatot-chip")
	addBtn.AddCSSClass("flat")
	footer.Append(addBtn)
	root.Append(footer)

	var library []client.Sticker
	var refresh func()
	remove := func(st client.Sticker) {
		go func() {
			if err := c.c.RemoveSticker(context.Background(), st.Key); err != nil {
				log.Printf("chatot: remove sticker: %v", err)
			}
			glib.IdleAdd(refresh)
		}()
	}
	refresh = func() {
		clearFlowBox(flow)
		stickers, err := c.c.Stickers()
		if err != nil {
			log.Printf("chatot: list stickers: %v", err)
		}
		library = stickers
		if len(library) == 0 {
			scroller.SetVisible(false)
			empty.SetVisible(true)
			return
		}
		empty.SetVisible(false)
		scroller.SetVisible(true)
		for _, st := range library {
			st := st
			tile := newStickerTile(st.Path)
			attachStickerMenu(root, tile, func() []menuItem {
				return stickerMenuItems(st, func() { remove(st) })
			})
			flow.Append(tile)
		}
	}

	flow.ConnectChildActivated(func(child *gtk.FlowBoxChild) {
		i := child.Index()
		if i < 0 || i >= len(library) {
			return
		}
		popover.Popdown()
		c.sendSticker(library[i].Path)
	})
	addBtn.ConnectClicked(func() { c.pickSticker(popover) })

	popover.ConnectMap(refresh)
	refresh()

	return root
}

// attachStickerMenu pops items over tile on a right-click or a long press.
func attachStickerMenu(host gtk.Widgetter, tile *gtk.Box, items func() []menuItem) {
	right := gtk.NewGestureClick()
	right.SetButton(gdk.BUTTON_SECONDARY)
	right.ConnectPressed(func(nPress int, x, y float64) {
		showChatContextMenu(host, tile, items(), x, y)
	})
	tile.AddController(right)
	long := gtk.NewGestureLongPress()
	long.SetTouchOnly(false)
	long.ConnectPressed(func(x, y float64) {
		showChatContextMenu(host, tile, items(), x, y)
	})
	tile.AddController(long)
}

// newStickerTile renders one library sticker fitted inside a rounded tile.
func newStickerTile(path string) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-sticker-tile")
	box.SetOverflow(gtk.OverflowHidden)
	box.SetSizeRequest(-1, stickerTileHeight)

	pic := newAsyncPicture(path, 2*stickerTileHeight)
	pic.SetCanShrink(true)
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetHExpand(true)
	pic.SetVExpand(true)
	pic.SetMarginTop(4)
	pic.SetMarginBottom(4)
	pic.SetMarginStart(4)
	pic.SetMarginEnd(4)
	box.Append(pic)
	return box
}
