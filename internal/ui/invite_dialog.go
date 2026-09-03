package ui

import (
	"context"
	"log"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// communityInviteBody is the invite dialog's explanation for a community.
const communityInviteBody = "Anyone with this link can join the community and see its announcement group. Reset it to revoke every link you have shared."

// showInviteLinkDialog is the mockup's "Invite to <name>" card, shared by
// groups and communities: an explanation, the link in a copy box, and a
// footer with Reset link (behind a confirmation), Cancel and Send to a chat.
// toast and forward may be nil (the copy/reset toasts and the send button
// then do nothing visible).
func showInviteLinkDialog(parent *gtk.Window, c client.Client, jid, title, body string, toast func(string), forward func(client.Message)) {
	if toast == nil {
		toast = func(string) {}
	}
	dialog := newCardDialog()
	dialog.SetTitle(title)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetDefaultSize(420, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	content := gtk.NewBox(gtk.OrientationVertical, 10)
	content.AddCSSClass("chatot-dialog-body")
	content.Append(newDialogBodyText(body))
	linkSlot := gtk.NewBox(gtk.OrientationVertical, 0)
	loading := gtk.NewLabel("Fetching the link…")
	loading.SetXAlign(0)
	loading.AddCSSClass("chatot-dialog-hint")
	linkSlot.Append(loading)
	content.Append(linkSlot)
	box.Append(content)

	link := ""
	setLink := func(l string) {
		link = l
		removeAllChildren(linkSlot)
		linkSlot.Append(newLinkBox(l, func() { toast("Invite link copied to the clipboard") }))
	}
	fetch := func(reset bool) {
		go func() {
			l, err := c.GroupInviteLink(context.Background(), jid, reset)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: invite link for %s: %v", jid, err)
					removeAllChildren(linkSlot)
					fail := gtk.NewLabel("Couldn't fetch the invite link")
					fail.SetXAlign(0)
					fail.AddCSSClass("chatot-dialog-hint")
					linkSlot.Append(fail)
					return
				}
				setLink(l)
				if reset {
					toast("Invite link reset")
				}
			})
		}()
	}
	fetch(false)

	footer := newDialogFooter()
	reset := newChipButton("Reset link", func() {
		alert := adw.NewAlertDialog("Reset the invite link?", "The current link stops working immediately. Anyone you already shared it with will not be able to join.")
		alert.AddResponse("cancel", "Cancel")
		alert.AddResponse("reset", "Reset")
		alert.SetResponseAppearance("reset", adw.ResponseDestructive)
		alert.SetCloseResponse("cancel")
		alert.ConnectResponse(func(response string) {
			if response == "reset" {
				fetch(true)
			}
		})
		presentAlert(alert, parent)
	})
	reset.AddCSSClass("chatot-chip-btn-danger")
	reset.SetHAlign(gtk.AlignStart)
	reset.SetHExpand(true)
	footer.Append(reset)
	footer.Append(newChipButton("Cancel", func() { dialog.Close() }))
	footer.Append(newPrimaryButton("Send to a chat", func() {
		if link == "" || forward == nil {
			return
		}
		dialog.Close()
		forward(client.Message{Text: link})
	}))
	box.Append(footer)
	dialog.SetChild(box)
	dialog.Present()
}
