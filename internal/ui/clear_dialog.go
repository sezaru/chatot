package ui

import (
	"context"
	"fmt"
	"log"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// ShowClearChatDialog opens the "Clear this chat?" confirmation for jid.
// Clearing is LOCAL-only (see Client.ClearChat): it never reaches the phone
// or the other party. On confirm it reloads cv if jid is the open chat and
// shows a toast; the chat list itself refreshes off the EventChatUpdate
// ClearChat pushes, no explicit call needed here.
func ShowClearChatDialog(parent *gtk.Window, c client.Client, cv *ConversationView, jid, contactName string) {
	body := fmt.Sprintf(
		"Messages are removed from chatot on this device. They stay on your phone and on %s's device.",
		contactName,
	)
	dialog := adw.NewAlertDialog("Clear this chat?", body)

	toggleLabel := gtk.NewLabel("Also delete downloaded media")
	toggleLabel.SetXAlign(0)
	toggleLabel.SetHExpand(true)
	alsoMedia := gtk.NewSwitch()
	alsoMedia.SetVAlign(gtk.AlignCenter)
	toggleRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	toggleRow.SetMarginTop(8)
	toggleRow.Append(toggleLabel)
	toggleRow.Append(alsoMedia)
	dialog.SetExtraChild(toggleRow)

	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("clear", "Clear chat")
	dialog.SetResponseAppearance("clear", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response != "clear" {
			return
		}
		withMedia := alsoMedia.Active()
		go func() {
			if err := c.ClearChat(context.Background(), jid, withMedia); err != nil {
				log.Printf("chatot: clear chat failed: %v", err)
				return
			}
			glib.IdleAdd(func() {
				if cv.CurrentJID() == jid {
					cv.Load(jid)
				}
				if cv.toastOverlay != nil {
					cv.toastOverlay.AddToast(adw.NewToast("Chat cleared"))
				}
			})
		}()
	})

	dialog.Present(parent)
}
