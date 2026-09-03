package ui

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// refreshAccountRail rebuilds the mockup's left account strip: one 38×38
// rounded-square initial button per account — green ring on the active one, an
// unread pill on the top-right corner — followed by a dashed ＋ add button.
// Rebuilt whole on every account change (a handful of accounts at most), the
// same way the switcher popover and chip row are.
func (cl *ChatList) refreshAccountRail() {
	if cl.switcher == nil || cl.rail == nil {
		return
	}
	for child := cl.rail.FirstChild(); child != nil; child = cl.rail.FirstChild() {
		cl.rail.Remove(child)
	}

	// In the merged list no single account is "the" active one, so the design
	// draws no ring at all (the header's 🗂 identity carries the state).
	activeID := cl.switcher.ActiveID()
	if cl.merged {
		activeID = ""
	}
	for _, m := range cl.switcher.Accounts() {
		cl.rail.Append(cl.buildRailAccount(accountRowVM(m, activeID)))
	}

	add := gtk.NewButtonWithLabel("＋")
	add.AddCSSClass("chatot-rail-add")
	add.SetTooltipText("Add account")
	add.ConnectClicked(func() {
		if cl.onAddAccount != nil {
			cl.onAddAccount()
		}
	})
	cl.rail.Append(add)
}

// buildRailAccount renders one rail entry from the same view-model the
// switcher popover rows use; clicking it makes that account active (and
// leaves the merged list, which no longer describes what is shown).
func (cl *ChatList) buildRailAccount(vm accountRowView) *gtk.Overlay {
	btn := gtk.NewButtonWithLabel(vm.Initial)
	btn.AddCSSClass("chatot-rail-avatar")
	btn.AddCSSClass(avatarColorClass(vm.ID))
	if vm.Active {
		btn.AddCSSClass("chatot-rail-active")
	}
	btn.SetSizeRequest(38, 38)
	btn.SetTooltipText(vm.Name)
	if letter, ok := btn.Child().(*gtk.Label); ok {
		centreGlyph(letter, 38)
	}
	id := vm.ID
	btn.ConnectClicked(func() {
		if id == cl.switcher.ActiveID() {
			cl.setMerged(false)
			return
		}
		cl.merged = false
		if err := cl.switcher.SetActive(id); err != nil {
			log.Printf("chatot: switch account %q failed: %v", id, err)
			return
		}
		cl.refreshAccountHeader()
	})

	overlay := gtk.NewOverlay()
	overlay.SetChild(btn)
	if vm.ShowUnread {
		badge := gtk.NewLabel(vm.UnreadText)
		badge.AddCSSClass("chatot-rail-badge")
		badge.SetHAlign(gtk.AlignEnd)
		badge.SetVAlign(gtk.AlignStart)
		overlay.AddOverlay(badge)
		overlay.SetMeasureOverlay(badge, false)
	}
	return overlay
}
