package ui

import (
	"log"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

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

// buildAccountPopover builds the switcher popover: one row per account (active
// one checked), a divider, then the Add/Manage account items. Rebuilt on each
// open via the header MenuButton's create-popup func, so status/unread stay
// fresh and switching the active account re-checks the right row.
func (cl *ChatList) buildAccountPopover() *gtk.Popover {
	pop := gtk.NewPopover()
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-account-switcher")

	activeID := cl.switcher.ActiveID()
	for _, m := range cl.switcher.Accounts() {
		vm := accountRowVM(m, activeID)
		id := vm.ID
		btn := gtk.NewButton()
		btn.AddCSSClass("flat")
		btn.SetChild(buildAccountRow(vm))
		btn.ConnectClicked(func() {
			pop.Popdown()
			if id == cl.switcher.ActiveID() {
				return
			}
			if err := cl.switcher.SetActive(id); err != nil {
				log.Printf("chatot: switch account %q failed: %v", id, err)
				return
			}
			cl.refreshAccountHeader()
		})
		box.Append(btn)
	}

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))
	box.Append(cl.buildAccountMenuItem("Add account…", func() {
		if cl.onAddAccount != nil {
			cl.onAddAccount()
		}
	}, pop))
	box.Append(cl.buildAccountMenuItem("Manage accounts…", func() {
		if cl.onManageAccounts != nil {
			cl.onManageAccounts()
		}
	}, pop))

	pop.SetChild(box)
	return pop
}

// buildAccountMenuItem builds a flat, left-aligned popover action button that
// pops the switcher down before running onClick.
func (cl *ChatList) buildAccountMenuItem(label string, onClick func(), pop *gtk.Popover) *gtk.Button {
	item := gtk.NewButtonWithLabel(label)
	item.AddCSSClass("flat")
	item.SetHAlign(gtk.AlignFill)
	item.Child().(*gtk.Label).SetXAlign(0)
	item.ConnectClicked(func() {
		pop.Popdown()
		onClick()
	})
	return item
}

// buildAccountRow constructs the widget tree for one switcher row: colored
// initial avatar, name over a dim status line, an optional unread badge, and a
// check on the active account.
func buildAccountRow(vm accountRowView) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)

	row.Append(newAvatarInitial(vm.ID, vm.Initial, 32))

	textCol := gtk.NewBox(gtk.OrientationVertical, 0)
	textCol.SetHExpand(true)
	textCol.SetVAlign(gtk.AlignCenter)

	nameLabel := gtk.NewLabel(vm.Name)
	nameLabel.SetXAlign(0)
	nameLabel.AddCSSClass("chatot-chat-name")
	textCol.Append(nameLabel)

	statusLabel := gtk.NewLabel(vm.Status)
	statusLabel.SetXAlign(0)
	statusLabel.AddCSSClass("chatot-account-status")
	textCol.Append(statusLabel)

	row.Append(textCol)

	if vm.ShowUnread {
		badge := gtk.NewLabel(vm.UnreadText)
		badge.AddCSSClass("chatot-unread-badge")
		badge.SetVAlign(gtk.AlignCenter)
		row.Append(badge)
	}

	if vm.Active {
		check := gtk.NewImageFromIconName("object-select-symbolic")
		check.SetVAlign(gtk.AlignCenter)
		row.Append(check)
	}

	return row
}
