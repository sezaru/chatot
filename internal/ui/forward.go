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

	"chatot/internal/client"
)

// forwardDialogAvatarSize matches chatRowAvatarSize; the forward picker's
// rows are a smaller version of the chat-list rows.
const forwardDialogAvatarSize = 32

// forwardSelectionLabel renders the dialog's footer for n selected chats.
func forwardSelectionLabel(n int) string {
	if n == 1 {
		return "1 chat selected"
	}
	return fmt.Sprintf("%d chats selected", n)
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

	dialog := gtk.NewWindow()
	dialog.SetTitle("Forward to")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(360, 480)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	preview := gtk.NewLabel(starredSnippet(msg))
	preview.AddCSSClass("chatot-bubble-quote")
	preview.SetXAlign(0)
	preview.SetWrap(true)
	box.Append(preview)

	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search chats")
	box.Append(search)

	list := gtk.NewListBox()
	list.SetSelectionMode(gtk.SelectionNone)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetChild(list)
	box.Append(scroller)

	footer := gtk.NewLabel(forwardSelectionLabel(0))
	footer.SetXAlign(0)
	box.Append(footer)

	btnRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnRow.SetHAlign(gtk.AlignEnd)
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	forwardBtn := gtk.NewButtonWithLabel("Forward")
	forwardBtn.AddCSSClass("suggested-action")
	forwardBtn.SetSensitive(false)
	btnRow.Append(cancelBtn)
	btnRow.Append(forwardBtn)
	box.Append(btnRow)

	selected := make(map[string]bool)
	cache := newAvatarCache()

	updateFooter := func() {
		footer.SetText(forwardSelectionLabel(len(selected)))
		forwardBtn.SetSensitive(len(selected) > 0)
	}

	rebuild := func(query string) {
		for child := list.FirstChild(); child != nil; {
			next := gtk.BaseWidget(child).NextSibling()
			list.Remove(child)
			child = next
		}
		for _, chat := range filterForwardChats(chats, query) {
			vm := chatRowVM(chat, time.Now())

			row := gtk.NewBox(gtk.OrientationHorizontal, 8)
			row.SetMarginTop(4)
			row.SetMarginBottom(4)
			row.SetMarginStart(4)
			row.SetMarginEnd(4)

			check := gtk.NewCheckButton()
			check.SetActive(selected[chat.JID])
			row.Append(check)

			row.Append(buildAvatar(c, cache, chat.JID, vm.Initial, forwardDialogAvatarSize))

			nameLabel := gtk.NewLabel(vm.Name)
			nameLabel.SetXAlign(0)
			nameLabel.SetHExpand(true)
			row.Append(nameLabel)

			jid := chat.JID
			check.ConnectToggled(func() {
				if check.Active() {
					selected[jid] = true
				} else {
					delete(selected, jid)
				}
				updateFooter()
			})

			list.Append(row)
		}
	}
	rebuild("")
	search.ConnectSearchChanged(func() { rebuild(search.Text()) })

	cancelBtn.ConnectClicked(func() { dialog.Close() })
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
