package ui

import (
	"log"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// AccountSwitcher is the slice of *client.AccountManager the sidebar header
// needs: the multi-account switcher UI reads accounts and swaps the active one
// through this, while the rest of the app keeps holding a plain client.Client.
type AccountSwitcher interface {
	Accounts() []client.AccountMeta
	ActiveID() string
	SetActive(string) error
}

// accountRowView holds the pure display fields for one account row in the
// switcher popover. accountRowVM computes it so the active-check and unread
// badge logic can be unit-tested without a display.
type accountRowView struct {
	ID         string
	Initial    string
	Name       string
	Status     string
	Active     bool
	ShowUnread bool
	UnreadText string
}

// accountRowVM derives the display view-model for one account, marking it
// active when its ID matches activeID and formatting the unread badge (capped
// at "99+") like the chat rows do.
func accountRowVM(m client.AccountMeta, activeID string) accountRowView {
	v := accountRowView{
		ID:      m.ID,
		Initial: initialFor(m.Name),
		Name:    m.Name,
		Status:  m.Status,
		Active:  m.ID == activeID,
	}
	if m.Unread > 0 {
		v.ShowUnread = true
		if m.Unread > 99 {
			v.UnreadText = "99+"
		} else {
			v.UnreadText = strconv.Itoa(m.Unread)
		}
	}
	return v
}

// accountStatusDot maps an account's status line to the mockup's status dot
// and text colour classes: green for connected, red for a logged-out
// account that needs a rescan.
func accountStatusDot(status string) string {
	if status == "Connected" {
		return "chatot-status-ok"
	}
	return "chatot-status-bad"
}

// accountRowStatus is the switcher row's second line: the connection state,
// then "· N unread" when the account has any.
func accountRowStatus(vm accountRowView) string {
	if vm.ShowUnread && vm.Status == "Connected" {
		return vm.Status + " · " + vm.UnreadText + " unread"
	}
	return vm.Status
}

// buildAccountPopover builds the switcher: the mockup's 300px card with one
// row per account (32px avatar, bold label, a status dot and line, an unread
// pill or the active check), a hairline, then the three actions. Rebuilt on
// each open via the header MenuButton's create-popup func, so status/unread
// stay fresh and switching the active account re-checks the right row.
func (cl *ChatList) buildAccountPopover() *gtk.Popover {
	pop := gtk.NewPopover()
	// The same floating card as every other menu: no arrow, 12px radius, a
	// hairline and a soft shadow.
	pop.SetHasArrow(false)
	pop.AddCSSClass("chatot-menu")
	pop.AddCSSClass("chatot-account-menu")
	box := gtk.NewBox(gtk.OrientationVertical, 1)
	box.AddCSSClass("chatot-account-switcher")

	activeID := cl.switcher.ActiveID()
	for _, m := range cl.switcher.Accounts() {
		vm := accountRowVM(m, activeID)
		id := vm.ID
		btn := gtk.NewButton()
		btn.AddCSSClass("chatot-switcher-row")
		if vm.Active && !cl.merged {
			btn.AddCSSClass("chatot-switcher-row-current")
		}
		btn.SetChild(buildAccountRow(vm))
		btn.ConnectClicked(func() {
			pop.Popdown()
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
		box.Append(btn)
	}

	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep.AddCSSClass("chatot-menu-sep")
	box.Append(sep)

	// The mockup's 🗂 row: every account's chats in one list, with a coloured
	// stripe and an account prefix on each row. It reads as selected while
	// the merged list is showing.
	merged := cl.buildAccountMenuItem("🗂", "All accounts in one list", func() {
		cl.setMerged(true)
	}, pop)
	if cl.merged {
		merged.AddCSSClass("chatot-switcher-row-current")
	}
	box.Append(merged)
	box.Append(cl.buildAccountMenuItem("＋", "Add account…", func() {
		if cl.onAddAccount != nil {
			cl.onAddAccount()
		}
	}, pop))
	box.Append(cl.buildAccountMenuItem("⚙", "Manage accounts…", func() {
		if cl.onManageAccounts != nil {
			cl.onManageAccounts()
		}
	}, pop))

	pop.SetChild(box)
	return pop
}

// buildAccountMenuItem builds one of the switcher's action rows — a 16px
// glyph column then the label, like every other menu row — that pops the
// switcher down before running onClick.
func (cl *ChatList) buildAccountMenuItem(icon, label string, onClick func(), pop *gtk.Popover) *gtk.Button {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	glyph := gtk.NewLabel(icon)
	glyph.AddCSSClass("chatot-menu-icon")
	glyph.SetSizeRequest(16, -1)
	row.Append(glyph)
	text := gtk.NewLabel(label)
	text.SetXAlign(0)
	text.SetHExpand(true)
	row.Append(text)

	item := gtk.NewButton()
	item.SetChild(row)
	item.AddCSSClass("chatot-menu-item")
	item.AddCSSClass("chatot-account-action")
	item.ConnectClicked(func() {
		pop.Popdown()
		onClick()
	})
	return item
}

// buildAccountRow constructs the widget tree for one switcher row: 32px
// palette avatar, the label over a status dot + line, then either an unread
// pill (other accounts) or a check (the active one).
func buildAccountRow(vm accountRowView) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)

	avatar := newAvatarInitial(vm.ID, vm.Initial, 32)
	avatar.AddCSSClass("chatot-account-row-avatar")
	avatar.SetVAlign(gtk.AlignCenter)
	row.Append(avatar)

	textCol := gtk.NewBox(gtk.OrientationVertical, 1)
	textCol.SetHExpand(true)
	textCol.SetVAlign(gtk.AlignCenter)

	nameLabel := gtk.NewLabel(vm.Name)
	nameLabel.SetXAlign(0)
	nameLabel.SetEllipsize(pango.EllipsizeEnd)
	nameLabel.AddCSSClass("chatot-account-row-name")
	textCol.Append(nameLabel)

	statusRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	dot := gtk.NewBox(gtk.OrientationVertical, 0)
	dot.AddCSSClass("chatot-status-dot")
	dot.AddCSSClass(accountStatusDot(vm.Status))
	dot.SetSizeRequest(6, 6)
	dot.SetVAlign(gtk.AlignCenter)
	statusRow.Append(dot)
	statusLabel := gtk.NewLabel(accountRowStatus(vm))
	statusLabel.SetXAlign(0)
	statusLabel.SetEllipsize(pango.EllipsizeEnd)
	statusLabel.AddCSSClass("chatot-account-row-status")
	statusLabel.AddCSSClass(accountStatusDot(vm.Status))
	statusRow.Append(statusLabel)
	textCol.Append(statusRow)

	row.Append(textCol)

	switch {
	case vm.Active:
		check := newCheckGlyph(14, true)
		check.AddCSSClass("chatot-account-row-check")
		row.Append(check)
	case vm.ShowUnread:
		badge := gtk.NewLabel(vm.UnreadText)
		badge.AddCSSClass("chatot-unread-badge")
		badge.SetVAlign(gtk.AlignCenter)
		row.Append(badge)
	}

	return row
}

// setMerged switches the sidebar between one account's chats and every
// account's merged into one list, then repaints the header and the list.
func (cl *ChatList) setMerged(on bool) {
	if cl.merged == on {
		return
	}
	cl.merged = on
	cl.refreshAccountHeader()
	cl.refresh()
}

// mergedSource is the account manager behind merged mode, nil when the
// switcher isn't one (a single-account build, or a test double).
func (cl *ChatList) mergedSource() *client.AccountManager {
	m, _ := cl.switcher.(*client.AccountManager)
	return m
}
