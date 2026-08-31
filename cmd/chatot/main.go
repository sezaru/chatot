// Command chatot is a standalone GTK4/libadwaita WhatsApp client.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
	"chatot/internal/settings"
	"chatot/internal/ui"
)

const appID = "com.sezdm.chatot"

func main() {
	app := adw.NewApplication(appID, gio.ApplicationFlagsNone)

	c := buildClient()

	app.ConnectActivate(func() { activate(app, c) })

	os.Exit(app.Run(os.Args))
}

// buildClient returns an AccountManager holding one account — the real
// whatsmeow-backed client, or the seeded Fake when CHATOT_FAKE=1 (dev/mockup
// path with no live WhatsApp link). The manager is the multi-account seam
// (F58+); with a single account it behaves exactly like the bare client.
// Future accounts get $XDG_STATE_HOME/chatot/accounts/<id>/; the "default"
// account keeps the legacy $XDG_STATE_HOME/chatot path for back-compat.
func buildClient() client.Client {
	m := client.NewAccountManager()
	if os.Getenv("CHATOT_FAKE") == "1" {
		m.AddAccount("default", "Account 1", client.NewFake())
		return m
	}
	stateDir := stateDir()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		log.Fatalf("chatot: create state dir: %v", err)
	}
	c, err := client.NewWhatsmeow(stateDir)
	if err != nil {
		log.Fatalf("chatot: init whatsmeow client: %v", err)
	}
	name := "Account 1"
	if jid := c.OwnJID(); jid != "" {
		name = jid
	}
	m.AddAccount("default", name, c)
	return m
}

// stateDir resolves $XDG_STATE_HOME/chatot, falling back to
// ~/.local/state/chatot.
func stateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "chatot")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("chatot: resolve home dir: %v", err)
	}
	return filepath.Join(home, ".local", "state", "chatot")
}

func activate(app *adw.Application, c client.Client) {
	loadCSS()

	prefs := settings.Load(settings.Dir())
	applySettings(prefs)

	chatList := ui.NewChatList(c)
	sidebar := adw.NewNavigationPage(chatList, "Chats")

	chatList.OnNewCommunityRequested(func() {
		log.Printf("chatot: new community requested (F48 not yet implemented)")
	})

	conversation := ui.NewConversationView(c)
	composer := ui.NewComposer(c)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.SetVExpand(true)
	contentBox.SetHExpand(true)
	contentBox.Append(conversation)
	contentBox.Append(composer)

	toastOverlay := adw.NewToastOverlay()
	toastOverlay.SetChild(contentBox)
	content := adw.NewNavigationPage(toastOverlay, "Conversation")

	composer.OnSent(func(msg client.Message) {
		conversation.AppendSentMessage(msg)
	})
	conversation.OnReplyRequested(composer.StartReply)
	conversation.OnEditRequested(composer.StartEdit)
	conversation.OnDeleteRequested(func(msg client.Message) {
		go func() {
			if err := c.DeleteMessage(context.Background(), msg.ChatJID, msg.ID); err != nil {
				log.Printf("chatot: delete message failed: %v", err)
			}
		}()
	})
	conversation.OnReactRequested(func(msg client.Message, emoji string) {
		go func() {
			if err := c.React(context.Background(), msg.ChatJID, msg.ID, emoji); err != nil {
				log.Printf("chatot: react failed: %v", err)
				return
			}
			glib.IdleAdd(func() { conversation.ApplyOwnReaction(msg.ChatJID) })
		}()
	})
	conversation.OnVoteRequested(func(msg client.Message, options []string) {
		go func() {
			if err := c.VotePoll(context.Background(), msg.ChatJID, msg.ID, options); err != nil {
				log.Printf("chatot: vote poll failed: %v", err)
			}
		}()
	})
	conversation.OnStarRequested(func(msg client.Message) {
		go func() {
			if err := c.StarMessage(context.Background(), msg.ChatJID, msg.ID, !msg.Starred); err != nil {
				log.Printf("chatot: star message failed: %v", err)
			}
		}()
	})
	// openChat is the single "show this chat" path: the chat-list click and
	// the notification's click-to-open action both funnel through it.
	openChat := func(jid string) {
		conversation.Load(jid)
		composer.SetChat(jid)
		go markReadOnOpen(c, jid, conversation.Messages())
	}
	chatList.OnChatSelected(openChat)

	split := adw.NewNavigationSplitView()
	split.SetSidebar(sidebar)
	split.SetContent(content)

	// The window flips between the QR pairing screen and the main UI via a
	// stack keyed on login state; whatsmeow pairing/connection events switch it.
	linking := ui.NewLinkingView()
	stack := gtk.NewStack()
	stack.AddNamed(linking, "linking")
	stack.AddNamed(split, "main")
	if c.LoggedIn() {
		stack.SetVisibleChildName("main")
	} else {
		stack.SetVisibleChildName("linking")
	}

	win := adw.NewApplicationWindow(&app.Application)
	win.SetTitle("chatot")
	win.SetDefaultSize(1000, 700)
	win.SetContent(stack)
	composer.SetWindow(&win.Window)
	chatList.SetWindow(&win.Window)
	conversation.SetWindow(&win.Window)
	conversation.SetToastOverlay(toastOverlay)

	conversation.OnForwardRequested(func(msg client.Message) {
		ui.ShowForwardDialog(&win.Window, c, msg, toastOverlay)
	})
	conversation.OnShowMediaRequested(func(jid string) {
		ui.ShowMediaPage(&win.Window, c, jid, chatNameFor(c, jid))
	})
	conversation.OnSearchAllChatsRequested(func(query string) {
		split.SetShowContent(false) // reveal the sidebar when the split view is collapsed
		chatList.OpenGlobalSearch(query)
	})
	conversation.OnExportRequested(func(jid string) {
		ui.ShowExportDialog(&win.Window, c, jid, chatNameFor(c, jid), toastOverlay)
	})
	conversation.OnClearRequested(func(jid string) {
		ui.ShowClearChatDialog(&win.Window, c, conversation, jid, chatNameFor(c, jid))
	})

	// "is-active" tracks OS-level window focus; report available/unavailable
	// so contacts see accurate presence rather than a permanent "online".
	win.NotifyProperty("is-active", func() {
		go sendPresence(c, win.IsActive())
	})

	openChatAction := gio.NewSimpleAction("open-chat", glib.NewVariantType("s"))
	openChatAction.ConnectActivate(func(param *glib.Variant) {
		jid := param.String()
		win.Present()
		openChat(jid)
	})
	app.AddAction(openChatAction)

	rejectCallAction := gio.NewSimpleAction("reject-call", glib.NewVariantType("s"))
	rejectCallAction.ConnectActivate(func(param *glib.Variant) {
		chatJID, callID, ok := ui.DecodeCallActionParam(param.String())
		if !ok {
			return
		}
		go func() {
			if err := c.RejectCall(context.Background(), chatJID, callID); err != nil {
				log.Printf("chatot: reject call failed: %v", err)
			}
		}()
	})
	app.AddAction(rejectCallAction)

	preferencesAction := gio.NewSimpleAction("preferences", nil)
	preferencesAction.ConnectActivate(func(_ *glib.Variant) {
		ui.ShowPreferences(&win.Window, &prefs, func(updated settings.Settings) {
			prefs = updated
			if err := settings.Save(settings.Dir(), prefs); err != nil {
				log.Printf("chatot: save settings failed: %v", err)
			}
		})
	})
	app.AddAction(preferencesAction)
	app.SetAccelsForAction("app.preferences", []string{"<Ctrl>comma"})

	ui.NewNotifier(c, &app.Application.Application, func() (bool, string) {
		return win.IsActive(), conversation.CurrentJID()
	})

	// System-tray StatusNotifierItem: click to raise, Open/Quit menu, unread
	// tooltip. Degrades to a no-op with no StatusNotifierWatcher on the bus.
	tray := ui.SetupTray(func() { win.Present() }, func() { app.Quit() })
	go watchTrayUnread(c, tray)

	// Feed pairing QR codes into the linking screen.
	go func() {
		for code := range c.QRCodes() {
			code := code
			glib.IdleAdd(func() {
				linking.SetQR(code)
				linking.SetStatus("Waiting for you to scan…")
			})
		}
	}()

	// Alternative to QR: pair with a phone-number code, entered on the phone.
	linking.OnPhonePairRequested(func(phone string) {
		go func() {
			code, err := c.PairPhone(context.Background(), phone)
			if err != nil {
				glib.IdleAdd(func() { linking.SetPairError(err.Error()) })
				return
			}
			glib.IdleAdd(func() { linking.SetPairCode(code) })
		}()
	})

	// Watch login state to flip the stack between the pairing screen and the
	// main UI. This is its own fan-out subscription — the chat list,
	// conversation and notifier each have their own (see the eventBus).
	// Subscribe synchronously (before the goroutine and before Start) so no
	// early event is dropped.
	loginCh := c.Events()
	go func() {
		for ev := range loginCh {
			switch {
			case ev.Kind == client.EventPairSuccess,
				ev.Kind == client.EventConnection && ev.Connection != nil && ev.Connection.Connected:
				glib.IdleAdd(func() { stack.SetVisibleChildName("main") })
			case ev.Kind == client.EventLoggedOut:
				glib.IdleAdd(func() {
					stack.SetVisibleChildName("linking")
					linking.SetStatus("Disconnected — scan to relink")
				})
			}
		}
	}()

	// Start last: the constructors above (NewChatList/NewConversationView/
	// NewNotifier) and loginCh all Subscribe synchronously before this runs,
	// so every subscription happens-before Start and no initial QR/pairing/
	// connection/history event can be dropped by an unscheduled goroutine.
	go func() {
		if err := c.Start(context.Background()); err != nil {
			log.Printf("chatot: client start failed: %v", err)
		}
	}()

	win.Present()
	go sendPresence(c, true)

	// Dev/screenshot hooks: CHATOT_SHOT_CHAT=<jid> opens that chat on launch;
	// CHATOT_SHOT_MEDIA=1 then opens its Media/Links/Docs page. Both no-op unset.
	if jid := os.Getenv("CHATOT_SHOT_CHAT"); jid != "" {
		openChat(jid)
		if os.Getenv("CHATOT_SHOT_MEDIA") == "1" {
			ui.ShowMediaPage(&win.Window, c, jid, chatNameFor(c, jid))
		}
	}
	if os.Getenv("CHATOT_SHOT_PREFS") == "1" {
		ui.ShowPreferences(&win.Window, &prefs, func(updated settings.Settings) { prefs = updated })
	}
}

// applySettings pushes a loaded Settings into the live package vars/state
// the rest of the app actually reads: Composer gates read receipts and
// typing indicators on ui.SendReadReceipts/ui.SendTypingIndicators, and the
// AdwStyleManager owns the color scheme. It must run before c.Start so the
// proxy (below) is in place before whatsmeow connects.
func applySettings(s settings.Settings) {
	ui.SendReadReceipts = s.SendReadReceipts
	ui.SendTypingIndicators = s.SendTypingIndicators
	ui.ApplyTheme(s.Theme)

	// Whatsmeow.Start reads CHATOT_PROXY itself (see internal/client), so
	// seeding it here reuses that exact path rather than adding a new one.
	// An operator-set CHATOT_PROXY always wins over the saved setting.
	if s.Proxy != "" && os.Getenv("CHATOT_PROXY") == "" {
		os.Setenv("CHATOT_PROXY", s.Proxy)
	}
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

// watchTrayUnread seeds the tray's unread tooltip and refreshes it on the
// events that move an unread count (new message, receipt, or a synced
// pin/mute/unread change). Its own Events() subscription is independent of the
// chat list's and the notifier's (fan-out bus).
func watchTrayUnread(c client.Client, tray *ui.Tray) {
	tray.SetUnread(totalUnread(c))
	for ev := range c.Events() {
		switch ev.Kind {
		case client.EventMessage, client.EventReceipt, client.EventChatUpdate, client.EventHistorySync:
			tray.SetUnread(totalUnread(c))
		}
	}
}

// totalUnread sums the unread counts across all chats.
func totalUnread(c client.Client) int {
	chats, err := c.Chats(0)
	if err != nil {
		return 0
	}
	total := 0
	for _, chat := range chats {
		total += chat.UnreadCount
	}
	return total
}

// chatNameFor looks up jid's display name from the chat list, for dialogs
// (the media/links/docs page) that need it but aren't handed it directly.
// Falls back to jid itself if the chat isn't found.
func chatNameFor(c client.Client, jid string) string {
	chats, err := c.Chats(0)
	if err != nil {
		return jid
	}
	for _, chat := range chats {
		if chat.JID == jid {
			return chat.Name
		}
	}
	return jid
}

func loadCSS() {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(ui.StyleCSS)
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
