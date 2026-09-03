package ui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// forwardDialogAvatarSize matches chatRowAvatarSize; the forward picker's
// rows are a smaller version of the chat-list rows.
const forwardDialogAvatarSize = 32

// ForwardInitialPick pre-ticks these chat JIDs when the dialog opens; a
// screenshot hook, so the ticked state can be captured without clicks.
var ForwardInitialPick []string

// forwardSelectionLabel renders the dialog's footer for n selected chats.
func forwardSelectionLabel(n int) string {
	if n == 0 {
		return "Pick chats to forward to"
	}
	return fmt.Sprintf("%d selected", n)
}

// filterForwardChats returns the chats whose name contains query
// case-insensitively (query "" matches everything), preserving order.
func filterForwardChats(chats []client.Chat, query string) []client.Chat {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return chats
	}
	out := make([]client.Chat, 0, len(chats))
	for _, c := range chats {
		if strings.Contains(strings.ToLower(c.Name), query) {
			out = append(out, c)
		}
	}
	return out
}

// ShowForwardDialog opens the "Forward to" picker for msg: a preview of the
// source message, a searchable multi-select list of the user's chats, and a
// Forward button that dispatches ForwardMessage to every checked chat in the
// background, reporting the outcome via toastOverlay (may be nil).
func ShowForwardDialog(parent *gtk.Window, c client.Client, msg client.Message, toastOverlay *adw.ToastOverlay) {
	chats, err := c.Chats(0)
	if err != nil {
		log.Printf("chatot: forward: load chats failed: %v", err)
		return
	}

	dialog := newCardDialog()
	dialog.SetTitle("Forward to…")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(360, 480)

	box := gtk.NewBox(gtk.OrientationVertical, 0)

	// The mockup drops the quoted-message preview: the ⋯ menu you came from
	// already showed which message this is, and the row list needs the height.
	search := sidebarSearchEntry("Search chats")
	searchRow := gtk.NewBox(gtk.OrientationVertical, 0)
	searchRow.AddCSSClass("chatot-forward-search")
	searchRow.Append(search)
	box.Append(searchRow)

	list := gtk.NewBox(gtk.OrientationVertical, 0)
	list.AddCSSClass("chatot-forward-list")

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetMinContentHeight(0)
	scroller.SetSizeRequest(-1, 120)
	scroller.SetChild(list)
	box.Append(scroller)

	// A single footer bar, per the mockup: the count at the left, one green
	// Send at the right. Closing is the title row's ✕.
	btnRow := gtk.NewBox(gtk.OrientationHorizontal, 10)
	btnRow.AddCSSClass("chatot-dialog-footer")
	footer := gtk.NewLabel(forwardSelectionLabel(0))
	footer.SetXAlign(0)
	footer.SetHExpand(true)
	footer.SetVAlign(gtk.AlignCenter)
	footer.AddCSSClass("chatot-card-value")
	btnRow.Append(footer)
	forwardBtn := gtk.NewButtonWithLabel("Send")
	forwardBtn.AddCSSClass("chatot-primary-btn")
	forwardBtn.SetSensitive(false)
	btnRow.Append(forwardBtn)
	box.Append(btnRow)

	selected := make(map[string]bool)
	for _, jid := range ForwardInitialPick {
		selected[jid] = true
	}
	cache := newAvatarCache()

	updateFooter := func() {
		footer.SetText(forwardSelectionLabel(len(selected)))
		forwardBtn.SetSensitive(len(selected) > 0)
	}

	var rebuild func(string)
	rebuild = func(query string) {
		removeAllChildren(list)
		for _, chat := range filterForwardChats(chats, query) {
			vm := chatRowVM(chat, time.Now())
			jid := chat.JID

			row := gtk.NewBox(gtk.OrientationHorizontal, 10)
			row.Append(buildAvatar(c, cache, jid, vm.Initial, forwardDialogAvatarSize))

			nameLabel := gtk.NewLabel(vm.Name)
			nameLabel.SetXAlign(0)
			nameLabel.SetHExpand(true)
			nameLabel.SetEllipsize(pango.EllipsizeEnd)
			nameLabel.AddCSSClass("chatot-forward-name")
			row.Append(nameLabel)

			// A round tick disc, not a square GtkCheckButton: the design's
			// forward list uses the same 19px check as its other pickers.
			check := newCheckGlyph(19, selected[jid])
			check.AddCSSClass("chatot-forward-check")
			if selected[jid] {
				check.AddCSSClass("chatot-forward-check-on")
			}
			row.Append(check)

			btn := gtk.NewButton()
			btn.SetChild(row)
			btn.AddCSSClass("flat")
			btn.AddCSSClass("chatot-people-row")
			btn.ConnectClicked(func() {
				if selected[jid] {
					delete(selected, jid)
				} else {
					selected[jid] = true
				}
				updateFooter()
				rebuild(search.Text())
			})
			list.Append(btn)
		}
	}
	rebuild("")
	updateFooter()
	search.ConnectSearchChanged(func() { rebuild(search.Text()) })

	forwardBtn.ConnectClicked(func() {
		targets := make([]string, 0, len(selected))
		for jid := range selected {
			targets = append(targets, jid)
		}
		dialog.Close()
		if len(targets) == 0 {
			return
		}
		go func() {
			ok := 0
			for _, jid := range targets {
				if _, err := c.ForwardMessage(context.Background(), msg, jid); err != nil {
					log.Printf("chatot: forward to %s failed: %v", jid, err)
					continue
				}
				ok++
			}
			if toastOverlay == nil {
				return
			}
			glib.IdleAdd(func() {
				toastOverlay.AddToast(adw.NewToast(fmt.Sprintf("Forwarded to %d chat(s)", ok)))
			})
		}()
	})

	dialog.SetChild(box)
	dialog.SetDefaultWidget(forwardBtn)
	dialog.Present()
}
