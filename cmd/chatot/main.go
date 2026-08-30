// Command chatot is a standalone GTK4/libadwaita WhatsApp client.
package main

import (
	"context"
	"log"
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
	"chatot/internal/ui"
)

const appID = "com.sezdm.chatot"

func main() {
	app := adw.NewApplication(appID, gio.ApplicationFlagsNone)

	// Fake stands in for the whatsmeow-backed Client until F2; the UI is
	// built against the Client interface either way.
	c := client.NewFake()

	app.ConnectActivate(func() { activate(app, c) })

	os.Exit(app.Run(os.Args))
}

func activate(app *adw.Application, c client.Client) {
	loadCSS()

	chatList := ui.NewChatList(c)
	sidebar := adw.NewNavigationPage(chatList, "Chats")

	conversation := ui.NewConversationView(c)
	composer := ui.NewComposer(c)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.SetVExpand(true)
	contentBox.SetHExpand(true)
	contentBox.Append(conversation)
	contentBox.Append(composer)
	content := adw.NewNavigationPage(contentBox, "Conversation")

	composer.OnSent(func(msg client.Message) {
		conversation.AppendSentMessage(msg)
	})
	conversation.OnReplyRequested(composer.StartReply)
	conversation.OnReactRequested(func(msg client.Message, emoji string) {
		go func() {
			if err := c.React(context.Background(), msg.ChatJID, msg.ID, emoji); err != nil {
				log.Printf("chatot: react failed: %v", err)
				return
			}
			glib.IdleAdd(func() { conversation.ApplyOwnReaction(msg.ChatJID) })
		}()
	})

	chatList.OnChatSelected(func(jid string) {
		conversation.Load(jid)
		composer.SetChat(jid)
		go markReadOnOpen(c, jid, conversation.Messages())
	})

	split := adw.NewNavigationSplitView()
	split.SetSidebar(sidebar)
	split.SetContent(content)

	win := adw.NewApplicationWindow(&app.Application)
	win.SetTitle("chatot")
	win.SetDefaultSize(1000, 700)
	win.SetContent(split)
	composer.SetWindow(&win.Window)

	// "is-active" tracks OS-level window focus; report available/unavailable
	// so contacts see accurate presence rather than a permanent "online".
	win.NotifyProperty("is-active", func() {
		go sendPresence(c, win.IsActive())
	})

	win.Present()
	go sendPresence(c, true)
}

func sendPresence(c client.Client, available bool) {
	if err := c.SendPresence(available); err != nil {
		log.Printf("chatot: send presence failed: %v", err)
	}
}

// markReadOnOpen looks up jid's unread count from the chat list and, if
// ui.SendReadReceipts is enabled, marks the corresponding messages read.
func markReadOnOpen(c client.Client, jid string, msgs []client.Message) {
	chats, err := c.Chats(0)
	if err != nil {
		return
	}
	for _, chat := range chats {
		if chat.JID == jid {
			ui.MarkReadOnOpen(context.Background(), c, jid, msgs, chat.UnreadCount)
			return
		}
	}
}

func loadCSS() {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(ui.StyleCSS)
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
