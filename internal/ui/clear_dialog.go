package ui

import (
	"context"
	"fmt"
	"log"
	"strings"

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

// ShowDeleteChatDialog opens the "Delete this chat?" confirmation for jid.
// Deleting goes further than clearing: it is WhatsApp's own delete, synced
// to the phone (see Client.DeleteChat), and the chat leaves the list. When
// jid is the open chat, onDeleted is called afterwards so the caller can
// show the empty pane in its place; the list refreshes off the
// EventChatUpdate DeleteChat pushes.
func ShowDeleteChatDialog(parent *gtk.Window, c client.Client, cv *ConversationView, jid, contactName string, onDeleted func()) {
	body := fmt.Sprintf("Messages with %s are deleted on this device and on your phone. The other side keeps theirs.", contactName)
	if strings.HasSuffix(jid, "@g.us") {
		body = "The group's messages are deleted on this device and on your phone. You stay a member: the next message brings the chat back. Leave the group first to be done with it."
	}
	dialog := adw.NewAlertDialog("Delete this chat?", body)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("delete", "Delete chat")
	dialog.SetResponseAppearance("delete", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response != "delete" {
			return
		}
		go func() {
			err := c.DeleteChat(context.Background(), jid)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: delete chat failed: %v", err)
					if cv.toastOverlay != nil {
						cv.toastOverlay.AddToast(adw.NewToast("Couldn't delete the chat: " + err.Error()))
					}
					return
				}
				if cv.CurrentJID() == jid && onDeleted != nil {
					onDeleted()
				}
				if cv.toastOverlay != nil {
					cv.toastOverlay.AddToast(adw.NewToast("Chat deleted"))
				}
			})
		}()
	})

	dialog.Present(parent)
}
