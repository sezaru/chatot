package ui

import (
	"context"
	"sort"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// blockedContactRow is the display view-model for one row of the blocked
// contacts dialog: a resolved name (falling back to the raw JID) plus the
// JID itself for the Unblock action.
type blockedContactRow struct {
	JID  string
	Name string
}

// blockedContactRows joins the blocked JIDs against the known chat names,
// sorted by display name for a stable listing.
func blockedContactRows(blocked []string, chats []client.Chat) []blockedContactRow {
	names := make(map[string]string, len(chats))
	for _, c := range chats {
		if c.Name != "" {
			names[c.JID] = c.Name
		}
	}

	rows := make([]blockedContactRow, 0, len(blocked))
	for _, jid := range blocked {
		name := names[jid]
		if name == "" {
			name = jid
		}
		rows = append(rows, blockedContactRow{JID: jid, Name: name})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// showBlockedDialog opens a modal listing blocked contacts with an Unblock
// button per row, refreshing the list after each unblock. Fetching happens
// off the main thread; UI updates are marshalled back via glib.IdleAdd.
func showBlockedDialog(parent *gtk.Window, c client.Client) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Blocked contacts")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(360, 400)

	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	status := gtk.NewLabel("Loading…")
	status.SetXAlign(0)
	box.Append(status)

	dialog.SetChild(box)
	dialog.Present()

	var reload func()
	reload = func() {
		go func() {
			blocked, err := c.Blocklist(context.Background())
			var chats []client.Chat
			if err == nil {
				chats, _ = c.Chats(1000)
			}
			glib.IdleAdd(func() {
				for child := box.FirstChild(); child != nil; {
					next := gtk.BaseWidget(child).NextSibling()
					box.Remove(child)
					child = next
				}
				if err != nil {
					box.Append(gtk.NewLabel("Couldn't load blocked contacts"))
					return
				}
				rows := blockedContactRows(blocked, chats)
				if len(rows) == 0 {
					box.Append(gtk.NewLabel("No blocked contacts"))
					return
				}
				for _, row := range rows {
					box.Append(newBlockedContactRowWidget(c, row, reload))
				}
			})
		}()
	}
	reload()
}

// newBlockedContactRowWidget builds a single name/JID row with an Unblock
// button that calls c.SetBlocked and then reload to refresh the list.
func newBlockedContactRowWidget(c client.Client, row blockedContactRow, reload func()) *gtk.Box {
	line := gtk.NewBox(gtk.OrientationHorizontal, 8)

	label := gtk.NewLabel(row.Name)
	label.SetXAlign(0)
	label.SetHExpand(true)
	line.Append(label)

	unblockBtn := gtk.NewButtonWithLabel("Unblock")
	unblockBtn.ConnectClicked(func() {
		unblockBtn.SetSensitive(false)
		go func() {
			c.SetBlocked(context.Background(), row.JID, false)
			glib.IdleAdd(reload)
		}()
	})
	line.Append(unblockBtn)

	return line
}
