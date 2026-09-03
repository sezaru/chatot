package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// sidebarFormPage is the stack page name every in-sidebar form shares; the
// chat list itself lives on "chats".
const sidebarFormPage = "form"

// showSidebarForm swaps the sidebar's chat list for a form — "← New chat",
// "← New group", "← New community" — per the mockup, which renders these in
// the sidebar rather than as separate windows. footer, when non-nil, is
// pinned under the scrolling body (the design's full-width green button).
func (cl *ChatList) showSidebarForm(title string, body gtk.Widgetter, footer gtk.Widgetter) {
	page := gtk.NewBox(gtk.OrientationVertical, 0)

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	header.AddCSSClass("chatot-sidebar-formhead")

	back := gtk.NewButtonWithLabel("←")
	back.AddCSSClass("flat")
	back.AddCSSClass("chatot-pane-back")
	back.ConnectClicked(cl.closeSidebarForm)
	header.Append(back)

	label := gtk.NewLabel(title)
	label.SetXAlign(0)
	label.SetHExpand(true)
	label.AddCSSClass("chatot-sidebar-formtitle")
	header.Append(label)
	page.Append(header)

	page.Append(body)
	if footer != nil {
		page.Append(footer)
	}

	// One reusable slot rather than a page per form: only one can be open, and
	// each is rebuilt from current data every time it opens anyway.
	if old := cl.modes.ChildByName(sidebarFormPage); old != nil {
		cl.modes.Remove(old)
	}
	cl.modes.AddNamed(page, sidebarFormPage)
	cl.modes.SetVisibleChildName(sidebarFormPage)
	cl.updateTabBarVisibility()
}

// closeSidebarForm returns the sidebar to the active tab's page.
func (cl *ChatList) closeSidebarForm() {
	cl.modes.SetVisibleChildName(cl.tabPageName())
	cl.updateTabBarVisibility()
	// The shot hooks' handles into the form point at widgets that are gone.
	cl.shotSel, cl.shotName, cl.shotCreate = nil, nil, nil
	cl.shotPickFirst, cl.shotPhoto = nil, nil
}

// sidebarSearchEntry is the pill search field every sidebar form leads with,
// styled like the chat list's.
func sidebarSearchEntry(placeholder string) *gtk.SearchEntry {
	entry := gtk.NewSearchEntry()
	entry.SetPlaceholderText(placeholder)
	entry.AddCSSClass("chatot-search-entry")
	entry.SetHExpand(true)
	return entry
}

// sidebarScroller wraps a form's body list so a long contact list scrolls
// inside the sidebar instead of forcing the window taller (the same
// GtkListBox minimum-height trap the chat list documents).
func sidebarScroller(child gtk.Widgetter) *gtk.ScrolledWindow {
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetMinContentHeight(0)
	scroller.SetSizeRequest(-1, 80)
	scroller.SetChild(child)
	return scroller
}

// peopleRow is the mockup's contact row inside a sidebar form: a 34px avatar,
// the name over a dim status line, and a green ✓ disc when picked.
func peopleRow(c client.Client, cache *avatarCache, chat client.Chat, status string, picked bool, onClick func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.Append(buildAvatar(c, cache, chat.JID, contactInitial(chat.Name), 34))

	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(chat.Name)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.AddCSSClass("chatot-people-name")
	col.Append(name)
	if status != "" {
		sub := gtk.NewLabel(status)
		sub.SetXAlign(0)
		sub.SetEllipsize(pango.EllipsizeEnd)
		sub.AddCSSClass("chatot-people-status")
		col.Append(sub)
	}
	row.Append(col)

	if picked {
		check := newCheckGlyph(20, true)
		check.AddCSSClass("chatot-people-check")
		row.Append(check)
	}

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-people-row")
	btn.ConnectClicked(onClick)
	return btn
}

// pickedChip is one removable person chip above the people list: a small
// initial avatar, the name and a ✕.
func pickedChip(name, initial, jid string, onRemove func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 5)

	avatar := newAvatarInitial(jid, initial, 18)
	avatar.AddCSSClass("chatot-picked-avatar")
	avatar.SetVAlign(gtk.AlignCenter)
	row.Append(avatar)

	label := gtk.NewLabel(name + " ✕")
	label.AddCSSClass("chatot-picked-name")
	row.Append(label)

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-picked-chip")
	btn.SetTooltipText("Remove " + name)
	btn.ConnectClicked(onRemove)
	return btn
}

// sidebarPrimaryButton is the design's full-width green action at the foot of
// a sidebar form ("Next · name the group", "Create group").
func sidebarPrimaryButton(label string, onClick func()) (*gtk.Box, *gtk.Button) {
	bar := gtk.NewBox(gtk.OrientationVertical, 0)
	bar.AddCSSClass("chatot-sidebar-formfoot")

	btn := gtk.NewButtonWithLabel(label)
	btn.AddCSSClass("chatot-sidebar-primary")
	btn.SetHExpand(true)
	// A caller that wires the click itself passes nil; connecting a nil func
	// would make the first click a nil-call panic (the group-name step did).
	if onClick != nil {
		btn.ConnectClicked(onClick)
	}
	bar.Append(btn)
	return bar, btn
}
