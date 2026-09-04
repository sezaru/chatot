package ui

import (
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// menuItem is one row of a mockup-style popover menu: a fixed-width glyph
// column, a label, and an optional right-aligned accelerator. A row with
// Separator set renders as a hairline instead and carries nothing else.
//
// The mockup builds every menu (＋, the app ⋮, the chat ⋮, a message's ⋯, the
// chat-row context menu, the labels popover) from this same shape, so the
// per-menu code here only has to return a []menuItem and hand it to
// popupMenu.
// MenuItem is menuItem for callers outside the package that supply menu
// rows (the chat row's right-click menu comes from main).
type MenuItem = menuItem

type menuItem struct {
	// Icon is the 16px-wide glyph column. Empty leaves the column blank so
	// labels still line up with neighbouring rows that do have one.
	Icon string
	// Mark draws the tray mark in the glyph column instead of Icon (the
	// About row).
	Mark  bool
	Label string
	// Accel renders right-aligned in a small monospace face, e.g. "Ctrl+F".
	Accel string
	// Destructive paints the row red (the mockup's #c01c28), for Delete /
	// Block / Clear.
	Destructive bool
	// Dim marks a non-actionable section heading, like the chat-row menu's
	// "Lists" caption above the label checkboxes.
	Dim bool
	// DotColor, when set, replaces the glyph column with a 9px rounded colour
	// swatch — the label/list rows in the chip-row overflow menu.
	DotColor string
	// Count renders right-aligned in accent green, for the label rows' chat
	// counts. Distinct from Accel, which is dim and monospaced.
	Count string
	// Separator makes this a hairline rather than a row.
	Separator bool
	// OnActivate runs on click. Nil rows are inert (headings, or an item
	// whose action lives elsewhere).
	OnActivate func()
}

// menuSeparator returns the hairline row that divides a menu into sections.
func menuSeparator() menuItem { return menuItem{Separator: true} }

// menuItemLabels returns the labels of the actionable rows in items, skipping
// separators. Menus are specified as data so their contents can be asserted
// in tests without a display.
func menuItemLabels(items []menuItem) []string {
	labels := make([]string, 0, len(items))
	for _, it := range items {
		if it.Separator {
			continue
		}
		labels = append(labels, it.Label)
	}
	return labels
}

// menuItemCSSClasses returns the CSS classes one menu row carries, most
// general first.
func menuItemCSSClasses(it menuItem) []string {
	classes := []string{"chatot-menu-item"}
	if it.Destructive {
		classes = append(classes, "chatot-menu-item-danger")
	}
	if it.Dim {
		classes = append(classes, "chatot-menu-item-dim")
	}
	return classes
}

// newMenuPopover builds an arrowless popover styled as the mockup's menu card
// and fills it with items. The caller owns parenting and popping it.
func newMenuPopover(items []menuItem) *gtk.Popover {
	pop := gtk.NewPopover()
	// No arrow: the mockup's menus are plain floating cards, and GtkPopover
	// would otherwise draw a pointer nub into the card's rounded edge.
	pop.SetHasArrow(false)
	pop.AddCSSClass("chatot-menu")
	pop.SetChild(buildMenuBox(items, pop))
	return pop
}

// setMenuPopoverItems replaces pop's contents with items, for menus whose rows
// depend on state that changes between popups (pinned/muted/archived wording,
// the chat-row menu's label checkboxes).
func setMenuPopoverItems(pop *gtk.Popover, items []menuItem) {
	pop.SetChild(buildMenuBox(items, pop))
}

// buildMenuBox lays out items as the menu card's content. Activating a row
// closes pop first, so an action that opens a dialog or another popover isn't
// fighting a popover that is still up.
func buildMenuBox(items []menuItem, pop *gtk.Popover) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 1)
	box.AddCSSClass("chatot-menu-box")

	for _, it := range items {
		if it.Separator {
			sep := gtk.NewSeparator(gtk.OrientationHorizontal)
			sep.AddCSSClass("chatot-menu-sep")
			box.Append(sep)
			continue
		}
		box.Append(buildMenuRow(it, pop))
	}
	return box
}

// buildMenuRow builds one clickable menu row: [16px glyph][label][accel].
func buildMenuRow(it menuItem, pop *gtk.Popover) *gtk.Button {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)

	// The glyph column is always present, even when empty, so every label in
	// the menu starts at the same x.
	if it.DotColor != "" {
		row.Append(newMenuDot(it.DotColor))
	} else if it.Mark {
		// 11px: the bird fills its box where the 13px emoji glyphs around
		// it do not, so it sits smaller to weigh the same; centred in the
		// 16px column.
		mark := newTrayMark(11)
		mark.(*gtk.Image).SetSizeRequest(16, -1)
		row.Append(mark)
	} else {
		icon := gtk.NewLabel(it.Icon)
		icon.AddCSSClass("chatot-menu-icon")
		icon.SetSizeRequest(16, -1)
		row.Append(icon)
	}

	label := gtk.NewLabel(it.Label)
	label.SetXAlign(0)
	label.SetHExpand(true)
	row.Append(label)

	if it.Count != "" {
		count := gtk.NewLabel(it.Count)
		count.AddCSSClass("chatot-menu-count")
		row.Append(count)
	}

	if it.Accel != "" {
		accel := gtk.NewLabel(it.Accel)
		accel.AddCSSClass("chatot-menu-accel")
		row.Append(accel)
	}

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	for _, c := range menuItemCSSClasses(it) {
		btn.AddCSSClass(c)
	}
	if it.OnActivate == nil {
		btn.SetSensitive(false)
	} else {
		action := it.OnActivate
		btn.ConnectClicked(func() {
			if pop != nil {
				pop.Popdown()
			}
			action()
		})
	}
	return btn
}

// withoutMenuItem returns items with the row labelled label removed. It lets a
// caller drop a row that the mockup's item list always includes but this
// particular context can't offer, e.g. "Edit message" on a message that is
// past WhatsApp's edit window.
func withoutMenuItem(items []menuItem, label string) []menuItem {
	out := make([]menuItem, 0, len(items))
	for _, it := range items {
		if !it.Separator && it.Label == label {
			continue
		}
		out = append(out, it)
	}
	return out
}

// newMenuDot builds the 9px colour swatch that stands in for the glyph column
// on a list/label row. The fill comes from a per-widget CSS provider: a class
// rule on the shared sheet can't carry a per-label colour.
func newMenuDot(hex string) *gtk.Label {
	dot := gtk.NewLabel("")
	dot.AddCSSClass("chatot-menu-dot")
	dot.SetSizeRequest(9, 9)
	dot.SetVAlign(gtk.AlignCenter)
	css := gtk.NewCSSProvider()
	css.LoadFromString("label { background-color: " + hex + "; border-radius: 3px; }")
	dot.StyleContext().AddProvider(css, widgetPriority(uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)))
	return dot
}

// labelMenuRow is one list/label offered in the chip-row overflow menu.
type labelMenuRow struct {
	ID    string
	Name  string
	Color string
	Count int
}

// labelMenuItems builds the chip-row overflow menu: a colour-swatch row per
// list with its chat count, then "Manage lists…". onPick receives the picked
// list's ID.
func labelMenuItems(rows []labelMenuRow, onPick func(id string), onManage func()) []menuItem {
	items := make([]menuItem, 0, len(rows)+2)
	for _, r := range rows {
		count := ""
		if r.Count > 0 {
			count = strconv.Itoa(r.Count)
		}
		id := r.ID
		var activate func()
		if onPick != nil {
			activate = func() { onPick(id) }
		}
		items = append(items, menuItem{
			DotColor:   r.Color,
			Label:      r.Name,
			Count:      count,
			OnActivate: activate,
		})
	}
	if len(rows) > 0 {
		items = append(items, menuSeparator())
	}
	return append(items, menuItem{Icon: "＋", Label: "Manage lists…", Dim: true, OnActivate: onManage})
}
