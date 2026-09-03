package ui

import (
	"context"
	"log"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// ShowDeleteMessageDialog is WhatsApp's delete prompt (the mockup has no
// design for it): any message can go for this account only; an own message
// that isn't already a tombstone can also go for everyone.
func ShowDeleteMessageDialog(parent *gtk.Window, c client.Client, msg client.Message) {
	d := adw.NewAlertDialog("Delete message?", "")
	d.AddResponse("cancel", "Cancel")
	d.AddResponse("me", "Delete for me")
	everyone := msg.FromMe && !msg.Deleted
	if everyone {
		d.AddResponse("everyone", "Delete for everyone")
		d.SetResponseAppearance("everyone", adw.ResponseDestructive)
	} else {
		d.SetResponseAppearance("me", adw.ResponseDestructive)
	}
	d.SetDefaultResponse("cancel")
	d.SetCloseResponse("cancel")
	d.ConnectResponse(func(response string) {
		switch response {
		case "me":
			go func() {
				if err := c.DeleteMessageForMe(context.Background(), msg.ChatJID, msg.ID); err != nil {
					log.Printf("chatot: delete message for me failed: %v", err)
				}
			}()
		case "everyone":
			go func() {
				if err := c.DeleteMessage(context.Background(), msg.ChatJID, msg.ID); err != nil {
					log.Printf("chatot: delete message failed: %v", err)
				}
			}()
		}
	})
	d.Present(parent)
}
