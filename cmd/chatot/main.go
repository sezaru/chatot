// Command chatot is a standalone GTK4/libadwaita WhatsApp client.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
	"chatot/internal/logfile"
	"chatot/internal/settings"
	"chatot/internal/ui"
)

const appID = "com.sezdm.chatot"

// logFileLimit caps the log file Preferences › Advanced points at.
const logFileLimit = 8 << 20

// mainWin is the one main window; a second activation (the launcher while
// the app is running, or a hidden-to-tray window) presents it instead of
// building another.
var mainWin *adw.ApplicationWindow

func main() {
	ensureComposeInput()
	// Every log line also lands in a capped file under the state dir, the
	// one Preferences › Advanced › Log file opens.
	if w, err := logfile.Open(filepath.Join(stateDir(), "chatot.log"), logFileLimit); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, w))
		ui.Prefs.LogFile = w.Path()
	} else {
		log.Printf("chatot: log file: %v", err)
	}
	// GTK 4.16+ defaults to its Vulkan renderer, which logs a
	// VK_SUBOPTIMAL_KHR warning whenever a popover surface first presents.
	// The GL renderer ("gl"; "ngl" before 4.22) draws the same and stays quiet; an explicit
	// GSK_RENDERER still wins.
	if os.Getenv("GSK_RENDERER") == "" {
		os.Setenv("GSK_RENDERER", "gl")
	}

	app := adw.NewApplication(appID, gio.ApplicationFlagsNone)

	if err := installDesktopEntry(); err != nil {
		log.Printf("chatot: install desktop entry: %v", err)
	}

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
		// Seed three fake accounts so the switcher is populated for
		// screenshots/dev; the first is active. Each is its own Fake client.
		m.AddAccount("personal", "Sezar (personal)", client.NewFake())
		m.AddAccount("work", "Work", client.NewFake())
		m.AddAccount("business", "Bakery (business)", client.NewFake())
		return m
	}
	// CHATOT_OFFLINE renders a COPY of the real local store with no network
	// connection (dev seam for reproducing the real UI against real data). It
	// never touches the live session or the phone link.
	if os.Getenv("CHATOT_OFFLINE") == "1" {
		dir, err := copyOfflineState(stateDir())
		if err != nil {
			log.Fatalf("chatot: copy offline state: %v", err)
		}
		c, err := client.NewWhatsmeow(dir)
		if err != nil {
			log.Fatalf("chatot: init offline client: %v", err)
		}
		c.SetOffline(true)
		m.SetBaseDir(dir)
		m.AddAccount("default", "", c)
		return m
	}

	stateDir := stateDir()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		log.Fatalf("chatot: create state dir: %v", err)
	}
	m.SetBaseDir(stateDir)
	c, err := client.NewWhatsmeow(stateDir)
	if err != nil {
		log.Fatalf("chatot: init whatsmeow client: %v", err)
	}
	// No label: the manager shows the profile name (or number) instead.
	m.AddAccount("default", "", c)
	// Re-create every persisted pairing account (no-op for a fresh, single-
	// account install, so behavior there is unchanged).
	if err := m.LoadRoster(); err != nil {
		log.Printf("chatot: load account roster: %v", err)
	}
	return m
}

// copyOfflineState copies the store, session, and avatar cache from src into a
// fresh temp dir so CHATOT_OFFLINE can render real data read-only without ever
// writing the real store. Media isn't copied (chips render without it).
func copyOfflineState(src string) (string, error) {
	dst, err := os.MkdirTemp("", "chatot-offline-")
	if err != nil {
		return "", err
	}
	for _, name := range []string{"session.db", "chatot.db", "chatot.db-wal", "chatot.db-shm"} {
		if err := copyFile(filepath.Join(src, name), filepath.Join(dst, name)); err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	if err := copyDir(filepath.Join(src, "avatars"), filepath.Join(dst, "avatars")); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	log.Printf("chatot: offline render from copy of %s at %s", src, dst)
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
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
	if mainWin != nil {
		mainWin.Present()
		return
	}
	loadCSS()
	if os.Getenv("CHATOT_SHOT") != "" {
		ui.EnableShotHooks()
	}

	prefs := settings.Load(settings.Dir())
	applySettings(prefs)
	saveSettings := func() {
		if err := settings.Save(settings.Dir(), prefs); err != nil {
			log.Printf("chatot: save settings failed: %v", err)
		}
	}

	chatList := ui.NewChatList(c)
	sidebar := adw.NewNavigationPage(chatList, "Chats")

	chatList.OnNewCommunityRequested(func() {
		log.Printf("chatot: new community requested (F48 not yet implemented)")
	})

	// The account manager is the multi-account switcher seam; hand it to the
	// header's switcher popover without disturbing the client.Client wiring.
	// The add/manage-account callbacks need the window, so they're wired below
	// once it exists.
	am, hasAccounts := c.(*client.AccountManager)
	if hasAccounts {
		chatList.SetAccountSwitcher(am)
		// Apply the keep-inactive-connected preference before Start so it
		// governs which accounts connect on launch.
		am.SetKeepInactiveConnected(prefs.KeepInactiveConnected)
	}

	conversation := ui.NewConversationView(c)
	composer := ui.NewComposer(c)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.SetVExpand(true)
	contentBox.SetHExpand(true)
	contentBox.Append(conversation)
	contentBox.Append(composer)
	// Files dropped anywhere on the chat pane queue as attachments.
	contentBox.AddController(composer.DropTarget())

	// Declared up here because the starred page needs it, and it in turn needs
	// the conversation and composer that are built above.
	var openChat func(jid string)

	// Starred and Media/Links/Docs are pages of the content pane in the
	// mockup, not separate windows, so the right side is a stack whose default
	// page is the conversation.
	rightPane := gtk.NewStack()
	rightPane.SetVExpand(true)
	rightPane.SetHExpand(true)
	rightPane.AddNamed(contentBox, "chat")
	showChat := func() { rightPane.SetVisibleChildName("chat") }

	starredPage := ui.NewStarredPage(c, showChat, func(jid string) {
		showChat()
		openChat(jid)
	})
	rightPane.AddNamed(starredPage, "starred")

	mediaPage := ui.NewMediaPage(c, showChat)
	rightPane.AddNamed(mediaPage, "media")

	// The attachment viewer is a page of the same pane (the mockup's
	// pane === 'viewer'): every picture, clip, voice note, file and location
	// in the open chat navigates as one sequence there.
	viewer := ui.NewAttachmentViewer(c, showChat)
	rightPane.AddNamed(viewer, "viewer")
	conversation.OnOpenViewerRequested(func(msg client.Message) {
		viewer.Open(chatByJIDOrEmpty(c, msg.ChatJID), conversation.Messages(), msg.ID)
		rightPane.SetVisibleChildName("viewer")
	})
	viewer.OnMenu(conversation.MessageMenuItems)
	viewer.OnReply(composer.StartReply)
	viewer.OnDownloaded(conversation.SetLocalPath)

	chatList.OnStarredRequested(func() {
		starredPage.Reload()
		rightPane.SetVisibleChildName("starred")
	})

	// The attachment tray covers the whole conversation pane (thread and
	// composer both), per the mockup, so it overlays contentBox rather than
	// sitting inside the composer.
	attachTray := ui.NewAttachTray(composer.SendTrayItems, composer.ReopenFilePicker)
	composer.SetTray(attachTray)
	paneOverlay := gtk.NewOverlay()
	paneOverlay.SetChild(rightPane)
	paneOverlay.AddOverlay(attachTray)

	toastOverlay := adw.NewToastOverlay()
	// The mockup's toast is a black pill with a green action, not Adwaita's
	// default bar; the class is on the overlay so every toast picks it up.
	toastOverlay.AddCSSClass("chatot-toast")
	toastOverlay.SetChild(paneOverlay)
	toastOverlay.SetVExpand(true)

	// The conversation header (with the window controls) sits above the pane
	// stack, so the media and starred pages open beneath it instead of
	// replacing it — the mockup keeps the chat identity visible over both.
	// On the Status, Channels and Communities tabs the mockup drops the chat
	// identity and its ⋮ from that strip, leaving the window controls; the
	// tabs' own panes carry their headers below it.
	contentHeader := gtk.NewStack()
	contentHeader.AddNamed(conversation.Header(), "chat")
	contentHeader.AddNamed(ui.NewPlainHeader(), "plain")
	contentCol := gtk.NewBox(gtk.OrientationVertical, 0)
	contentCol.Append(contentHeader)
	contentCol.Append(toastOverlay)
	content := adw.NewNavigationPage(contentCol, "Conversation")

	// The tabs' content panes: the per-tab placeholder, the status viewer,
	// the channel reader and the community page.
	panes := chatList.Panes()
	rightPane.AddNamed(panes.Empty, "tabempty")
	rightPane.AddNamed(panes.Status, "status")
	rightPane.AddNamed(panes.Channel, "channel")
	rightPane.AddNamed(panes.Community, "community")
	chatList.OnPaneRequested(func(name string) {
		rightPane.SetVisibleChildName(name)
		if name == "chat" {
			contentHeader.SetVisibleChildName("chat")
		} else {
			contentHeader.SetVisibleChildName("plain")
		}
	})

	composer.OnSent(func(msg client.Message) {
		conversation.AppendSentMessage(msg)
	})
	conversation.OnReplyRequested(composer.StartReply)
	conversation.OnEditRequested(composer.StartEdit)
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
	openChat = func(jid string) {
		// A chat click lands on the thread: the viewer, starred and media
		// pages belong to the chat that was open before.
		if name := rightPane.VisibleChildName(); name != "chat" {
			if name == "viewer" {
				viewer.Close()
			}
			showChat()
		}
		chatList.SetSelected(jid)
		conversation.Load(jid)
		composer.SetChat(jid)
		composer.SetChatName(chatNameFor(c, jid))
		composer.FocusInput()
		go markReadOnOpen(c, jid, conversation.Messages())
	}
	chatList.OnChatSelected(openChat)
	conversation.OnStopLiveRequested(composer.StopLiveLocation)

	split := adw.NewNavigationSplitView()
	split.SetSidebar(sidebar)
	split.SetContent(content)
	// Mockup proportions: the sidebar is the 54px account rail plus a 334px
	// list column = 388px of the 1240px design canvas (31.3%). The clamps keep
	// that usable at other window sizes without letting the sidebar drift far
	// from the design width.
	split.SetSidebarWidthFraction(388.0 / 1240.0)
	split.SetMinSidebarWidth(340)
	split.SetMaxSidebarWidth(420)

	// The window flips between the QR pairing screen and the main UI via a
	// stack keyed on login state; whatsmeow pairing/connection events switch it.
	linking := ui.NewLinkingView()
	syncView := ui.NewSyncView()
	stack := gtk.NewStack()
	stack.AddNamed(ui.NewLoadingView(), "loading")
	stack.AddNamed(linking, "linking")
	stack.AddNamed(syncView, "syncing")
	stack.AddNamed(split, "main")
	// The main view fades in over the loading mark once the socket is up.
	stack.SetTransitionType(gtk.StackTransitionTypeCrossfade)
	stack.SetTransitionDuration(350)
	switch {
	case c.LoggedIn():
		stack.SetVisibleChildName("main")
	case c.Paired():
		// Linked, but the socket only comes up once Start runs below: show
		// the mark rather than flashing the QR page at someone who has
		// nothing to pair.
		stack.SetVisibleChildName("loading")
	default:
		stack.SetVisibleChildName("linking")
	}
	// leaveLoading moves off the loading mark to the main view (the local
	// store is usable offline) unless something already did.
	leaveLoading := func() {
		if stack.VisibleChildName() == "loading" {
			stack.SetVisibleChildName("main")
		}
	}

	win := adw.NewApplicationWindow(&app.Application)
	mainWin = win
	ui.ApplyFontSize(win, prefs.FontSize)
	win.SetTitle("chatot")
	// The icon name resolves through the desktop entry installDesktopEntry
	// keeps in place; on X11 GTK reads it from the window directly.
	win.SetIconName(appID)
	win.SetDefaultSize(1000, 700)
	win.SetContent(stack)
	composer.SetWindow(&win.Window)
	chatList.SetWindow(&win.Window)
	conversation.SetWindow(&win.Window)
	// Seeing the pill is reading the chat: the badge clears and the sender
	// gets the tick, as on open.
	conversation.OnUnreadSeen(func(jid string, msgs []client.Message) {
		go markReadOnOpen(c, jid, msgs)
	})
	conversation.OnDeleteRequested(func(msg client.Message) {
		ui.ShowDeleteMessageDialog(&win.Window, c, msg)
	})
	conversation.SetToastOverlay(toastOverlay)
	viewer.SetWindow(&win.Window)
	viewer.SetToastOverlay(toastOverlay)
	viewer.OnForward(func(msg client.Message) {
		ui.ShowForwardDialog(&win.Window, c, msg, toastOverlay)
	})
	viewer.OnStar(func(msg client.Message) {
		go func() {
			if err := c.StarMessage(context.Background(), msg.ChatJID, msg.ID, !msg.Starred); err != nil {
				log.Printf("chatot: star message failed: %v", err)
			}
		}()
	})
	// A chat row's right-click menu is the conversation's ⋮ menu for that
	// chat; rows that act on the open thread open the chat first.
	chatList.SetRowMenu(func(chat client.Chat) []ui.MenuItem {
		jid := chat.JID
		return conversation.MenuItemsForChat(jid, func() {
			if conversation.CurrentJID() != jid {
				openChat(jid)
			}
		})
	})
	chatList.SetToastOverlay(toastOverlay)
	chatList.OnForwardRequested(func(msg client.Message) {
		ui.ShowForwardDialog(&win.Window, c, msg, toastOverlay)
	})

	if hasAccounts {
		refresh := func() { chatList.RefreshAccounts() }
		chatList.OnAddAccountRequested(func() {
			ui.ShowAddAccountDialog(&win.Window, am, refresh)
		})
		chatList.OnManageAccountsRequested(func() {
			ui.ShowManageAccountsDialog(&win.Window, am, &prefs, refresh, saveSettings)
		})
	}

	conversation.OnForwardRequested(func(msg client.Message) {
		ui.ShowForwardDialog(&win.Window, c, msg, toastOverlay)
	})
	conversation.OnShowMediaRequested(func(jid string) {
		mediaPage.Load(jid)
		rightPane.SetVisibleChildName("media")
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
		ui.ShowPreferences(&win.Window, &prefs, c, func(updated settings.Settings) {
			prefs = updated
			if err := settings.Save(settings.Dir(), prefs); err != nil {
				log.Printf("chatot: save settings failed: %v", err)
			}
		})
	})
	app.AddAction(preferencesAction)
	app.SetAccelsForAction("app.preferences", []string{"<Ctrl>comma"})

	// The mockup's ⋮ menu ends in Quit (Ctrl+Q); GApplication provides no
	// quit action of its own.
	quitAction := gio.NewSimpleAction("quit", nil)
	quitAction.ConnectActivate(func(_ *glib.Variant) { app.Quit() })
	app.AddAction(quitAction)
	app.SetAccelsForAction("app.quit", []string{"<Ctrl>q"})

	// The Shortcuts page's Window and Conversation bindings.
	searchChatsAction := gio.NewSimpleAction("search-chats", nil)
	searchChatsAction.ConnectActivate(func(_ *glib.Variant) { chatList.FocusSearch() })
	app.AddAction(searchChatsAction)
	app.SetAccelsForAction("app.search-chats", []string{"<Ctrl>k"})
	searchInChatAction := gio.NewSimpleAction("search-in-chat", nil)
	searchInChatAction.ConnectActivate(func(_ *glib.Variant) { conversation.OpenSearch("") })
	app.AddAction(searchInChatAction)
	app.SetAccelsForAction("app.search-in-chat", []string{"<Ctrl>f"})
	closeWindowAction := gio.NewSimpleAction("close-window", nil)
	closeWindowAction.ConnectActivate(func(_ *glib.Variant) { win.Close() })
	app.AddAction(closeWindowAction)
	app.SetAccelsForAction("app.close-window", []string{"<Ctrl>w"})

	var accountInfo func() (string, int)
	if hasAccounts {
		accountInfo = func() (string, int) { return am.ActiveName(), am.Count() }
	}
	ui.NewNotifier(c, &app.Application.Application, func() (bool, string) {
		return win.IsActive(), conversation.CurrentJID()
	}, accountInfo)

	// System-tray StatusNotifierItem: click to raise, Open/Quit menu, unread
	// tooltip. Degrades to a no-op with no StatusNotifierWatcher on the bus.
	// Preferences › Notifications › Show tray icon adds and removes it.
	var trayMu sync.Mutex
	var tray *ui.Tray
	setTray := func(show bool) {
		trayMu.Lock()
		defer trayMu.Unlock()
		switch {
		case show && tray == nil:
			tray = ui.SetupTray(func() { win.Present() }, func() { app.Quit() })
			tray.SetUnread(totalUnread(c))
		case !show && tray != nil:
			tray.Teardown()
			tray = nil
		}
	}
	setTray(prefs.ShowTrayIcon)
	go watchTrayUnread(c, func(n int) {
		trayMu.Lock()
		defer trayMu.Unlock()
		if tray != nil {
			tray.SetUnread(n)
		}
	})
	// Close to tray: with a tray icon to bring it back, closing hides the
	// window and the app keeps running; without one (the switch off, or no
	// tray host on this desktop), closing quits.
	win.ConnectCloseRequest(func() bool {
		trayMu.Lock()
		hasTray := tray != nil
		trayMu.Unlock()
		if prefs.CloseToTray && hasTray && ui.TrayAvailable() {
			win.SetVisible(false)
			return true
		}
		return false
	})

	ui.Prefs.RefreshChatList = chatList.RebuildRows
	ui.Prefs.SetTrayVisible = setTray
	ui.Prefs.Toasts = toastOverlay
	ui.Prefs.ManageAccounts = func() {
		if hasAccounts {
			ui.ShowManageAccountsDialog(&win.Window, am, &prefs, func() { chatList.RefreshAccounts() }, saveSettings)
		}
	}

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
	// The post-link backfill: a fresh pairing parks the window on the
	// syncing screen until the chat list and recent messages have landed,
	// then the older history streams behind a sidebar banner. sync is only
	// ever touched on the main loop.
	var sync ui.SyncTracker
	applySync := func() {
		if sync.Blocking() {
			syncView.SetCounts(sync.Counts())
			if stack.VisibleChildName() != "syncing" {
				stack.SetVisibleChildName("syncing")
			}
			return
		}
		if stack.VisibleChildName() == "syncing" {
			stack.SetVisibleChildName("main")
			chatList.RefreshAccounts()
		}
		if sync.Background() {
			chatList.SetSyncProgress(sync.BannerText(), sync.Fraction())
		} else {
			chatList.HideSyncProgress()
		}
	}
	glib.TimeoutAdd(1000, func() bool {
		if sync.Tick(time.Now()) || sync.Blocking() || sync.Background() {
			applySync()
		}
		return true
	})

	loginCh := c.Events()
	go func() {
		for ev := range loginCh {
			switch {
			case ev.Kind == client.EventPairSuccess:
				glib.IdleAdd(func() {
					sync.Pair(time.Now())
					applySync()
				})
			case ev.Kind == client.EventConnection && ev.Connection != nil && ev.Connection.Connected:
				glib.IdleAdd(func() {
					if !sync.Blocking() {
						stack.SetVisibleChildName("main")
					}
				})
			case ev.Kind == client.EventHistorySync && ev.HistorySync != nil:
				h := ev.HistorySync
				glib.IdleAdd(func() {
					sync.Chunk(h, time.Now())
					applySync()
				})
			case ev.Kind == client.EventLoggedOut:
				glib.IdleAdd(func() {
					// Every account's events share this bus; a background
					// account signing out must not hide the one in view.
					if c.LoggedIn() {
						return
					}
					stack.SetVisibleChildName("linking")
					linking.SetStatus("Disconnected — scan to relink")
				})
			case ev.Kind == client.EventMessage && ev.Message != nil && !ev.Message.FromMe && !ev.Synced:
				// A message landing in the chat on screen while the window
				// has focus is read as it arrives: no badge, and a receipt
				// if the user sends them.
				msg := *ev.Message
				glib.IdleAdd(func() {
					if win.IsActive() && conversation.CurrentJID() == msg.ChatJID {
						go ui.MarkReadOnArrival(context.Background(), c, msg)
					}
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
			// Offline or refused: the cached chats are still worth showing.
			glib.IdleAdd(func() {
				if c.Paired() {
					leaveLoading()
				} else {
					stack.SetVisibleChildName("linking")
				}
			})
		}
	}()
	// A connection that takes unusually long must not hold the whole app
	// behind the mark; the header reports the connection state anyway.
	glib.TimeoutAdd(loadingFallbackMS, func() bool {
		leaveLoading()
		return false
	})

	win.Present()
	go sendPresence(c, true)
	chatList.PreloadCommunities()

	// Dev/screenshot hooks: CHATOT_SHOT_CHAT=<jid> opens that chat on launch;
	// CHATOT_SHOT_MEDIA=1 then opens its Media/Links/Docs page. Both no-op unset.
	if jid := os.Getenv("CHATOT_SHOT_CHAT"); jid != "" {
		openChat(jid)
		if os.Getenv("CHATOT_SHOT_MEDIA") == "1" {
			mediaPage.Load(jid)
			rightPane.SetVisibleChildName("media")
		}
	}
	// CHATOT_SHOT=<state> reaches one named mockup state (see shotHook); most
	// need the chat opened first via CHATOT_SHOT_CHAT and a realized thread,
	// hence the delay. CHATOT_SHOT_MSG picks the message index (default -1 =
	// newest) for the per-bubble states.
	if state := os.Getenv("CHATOT_SHOT"); state != "" {
		msgIdx := -1
		if v := os.Getenv("CHATOT_SHOT_MSG"); v != "" {
			fmt.Sscanf(v, "%d", &msgIdx)
		}
		// CHATOT_SHOT_DELAY (ms) waits longer for a thread whose pictures
		// re-measure the rows after they decode.
		delay := uint(1200)
		if v := os.Getenv("CHATOT_SHOT_DELAY"); v != "" {
			fmt.Sscanf(v, "%d", &delay)
		}
		glib.TimeoutAdd(delay, func() bool {
			shotHook(state, msgIdx, shotDeps{chatList: chatList, conversation: conversation, composer: composer, viewer: viewer, stack: stack, linking: linking, sync: syncView, win: &win.Window, c: c, am: am, prefs: &prefs, toasts: toastOverlay, saveSettings: saveSettings})
			return false
		})
	}

	if os.Getenv("CHATOT_SHOT_ATTACH") == "1" {
		glib.TimeoutAdd(900, func() bool { composer.PopAttach(); return false })
	}
	if os.Getenv("CHATOT_SHOT_PREFS") == "1" {
		// CHATOT_SHOT_ARG names the page to open on (a prefPages ID).
		ui.PreferencesInitialPage = os.Getenv("CHATOT_SHOT_ARG")
		ui.ShowPreferences(&win.Window, &prefs, c, func(updated settings.Settings) { prefs = updated })
	}
	if os.Getenv("CHATOT_SHOT_SWITCHER") == "1" {
		glib.IdleAdd(func() { chatList.PopupAccountSwitcher() })
	}
	if os.Getenv("CHATOT_SHOT_MANAGE") == "1" && hasAccounts {
		ui.ShowManageAccountsDialog(&win.Window, am, &prefs, func() { chatList.RefreshAccounts() }, saveSettings)
	}
	if os.Getenv("CHATOT_SHOT_ADDACCOUNT") == "1" && hasAccounts {
		ui.ShowAddAccountDialog(&win.Window, am, func() { chatList.RefreshAccounts() })
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
	ui.LocationAccess = s.LocationAccess
	ui.NotificationsEnabled = s.ShowNotifications
	ui.NotificationsPerAccount = s.NotificationsPerAccount
	ui.NotificationSound = s.NotificationSound
	ui.NotificationSoundFile = s.NotificationSoundFile
	ui.NotificationText = s.NotificationText
	ui.ShowWindowControls = s.ShowWindowControls
	ui.ShowMessagePreviews = s.ShowMessagePreviews
	ui.AutoDownload = s.AutoDownload
	ui.GIFService = s.GIFService
	ui.GIFAPIKey = s.GIFAPIKey
	client.SetVerboseLogging(s.VerboseLogging)
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

// markReadOnOpen looks up jid's unread count from the chat list and marks
// that many of its newest inbound messages read.
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
func watchTrayUnread(c client.Client, setUnread func(int)) {
	setUnread(totalUnread(c))
	for ev := range c.Events() {
		switch ev.Kind {
		case client.EventMessage, client.EventReceipt, client.EventChatUpdate, client.EventHistorySync:
			setUnread(totalUnread(c))
		}
	}
}

// totalUnread sums the unread counts across all chats outside the archive,
// which the phone's badge leaves out too.
func totalUnread(c client.Client) int {
	chats, err := c.Chats(0)
	if err != nil {
		return 0
	}
	total := 0
	for _, chat := range chats {
		if !chat.Archived {
			total += chat.UnreadCount
		}
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

func loadCSS() { ui.InstallStyles() }

// shotDeps bundles everything shotHook can poke; all dev/screenshot only.
type shotDeps struct {
	viewer       *ui.AttachmentViewer
	chatList     *ui.ChatList
	conversation *ui.ConversationView
	composer     *ui.Composer
	stack        *gtk.Stack
	linking      *ui.LinkingView
	sync         *ui.SyncView
	win          *gtk.Window
	c            client.Client
	am           *client.AccountManager
	prefs        *settings.Settings
	toasts       *adw.ToastOverlay
	saveSettings func()
}

// shotHook drives the window into one named mockup state for screenshots.
// loadingFallbackMS bounds the startup mark: past this the main view shows
// with whatever the local store holds, connected or not.
const loadingFallbackMS = 12000

func shotHook(state string, msgIdx int, d shotDeps) {
	refresh := func() { d.chatList.RefreshAccounts() }
	jid := d.conversation.CurrentJID()
	name := chatNameFor(d.c, jid)
	arg := os.Getenv("CHATOT_SHOT_ARG")
	switch state {
	case "refreshbench":
		d.chatList.RefreshBench(10)
		d.chatList.RefreshBreakdown()
		d.chatList.RefreshBreakdown()
	case "reconcilecheck":
		d.chatList.ReconcileCheck()
	case "anchorcheck":
		d.conversation.AnchorCheck()
	case "flingcheck":
		d.conversation.FlingCheck(arg == "abs")
	case "scrollbench":
		// CHATOT_SHOT_ARG=chats scrolls the chat list, anything else the
		// open thread; frame stats land in the log after 6 s.
		if arg == "chats" {
			s := d.chatList.ListScroller()
			ui.ScrollBench(s, s.VAdjustment(), "chats", 6*time.Second, 900)
		} else {
			s := d.conversation.Scroller()
			ui.ScrollBench(s, s.VAdjustment(), "thread", 6*time.Second, 900)
		}
	case "plusmenu":
		d.chatList.PopupPlusMenu()
	// The bottom tabs: CHATOT_SHOT_ARG names a tab, a status poster, a
	// channel or a community for the states that need one.
	case "tab":
		d.chatList.SelectTab(arg)
	case "tabplus":
		d.chatList.SelectTab(arg)
		glib.TimeoutAdd(300, func() bool { d.chatList.PopupPlusMenu(); return false })
	case "statusopen":
		d.chatList.SelectTab("status")
		d.chatList.ShowStatus(arg)
	case "statuspause":
		d.chatList.SelectTab("status")
		d.chatList.ShowStatus(arg)
		glib.TimeoutAdd(1500, func() bool { d.chatList.PauseStatus(); return false })
	case "chanreactn":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
		glib.TimeoutAdd(400, func() bool {
			d.chatList.ReactChannelFirst(os.Getenv("CHATOT_SHOT_TEXT"), msgIdx)
			return false
		})
	case "chanunfollow":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
		glib.TimeoutAdd(300, func() bool { d.chatList.ConfirmUnfollow(arg); return false })
	case "statusrowmenu":
		d.chatList.SelectTab("status")
		d.chatList.PopupStatusRowMenu(arg)
	case "statusmute":
		// Mute, then open the muted poster: the viewer must still reach
		// the Muted updates section.
		d.chatList.SelectTab("status")
		d.chatList.MuteStatusNow(arg)
		glib.TimeoutAdd(600, func() bool { d.chatList.ShowStatus(arg); return false })
	case "statusviewers":
		d.chatList.SelectTab("status")
		d.chatList.PostTextStatusNow("Shipping 2.1 tonight. Channels and Communities are in.")
		glib.TimeoutAdd(400, func() bool { d.chatList.ShowStatusViewersNow(); return false })
	case "statusviewmenu":
		d.chatList.SelectTab("status")
		d.chatList.ShowStatus(arg)
		glib.TimeoutAdd(300, func() bool { d.chatList.PopupStatusViewMenu(); return false })
	case "mystatus":
		d.chatList.SelectTab("status")
		d.chatList.PostTextStatusNow("Shipping 2.1 tonight. Channels and Communities are in.")
	case "mystatusopen":
		d.chatList.SelectTab("status")
		d.chatList.PostTextStatusNow("Shipping 2.1 tonight. Channels and Communities are in.")
		glib.TimeoutAdd(400, func() bool { d.chatList.ShowStatus("me"); return false })
	case "mystatusmenu":
		d.chatList.SelectTab("status")
		d.chatList.PostTextStatusNow("Shipping 2.1 tonight. Channels and Communities are in.")
		glib.TimeoutAdd(400, func() bool { d.chatList.PopupMyStatusMenu(); return false })
	case "textstatus":
		d.chatList.SelectTab("status")
		d.chatList.ShowTextStatus()
	case "channelopen":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
	case "chanmenu":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
		glib.TimeoutAdd(300, func() bool { d.chatList.PopupChannelMenu(); return false })
	case "chanrowmenu":
		d.chatList.SelectTab("channels")
		d.chatList.PopupChannelRowMenu(arg)
	case "chaninfo":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
		d.chatList.ShowChannelInfo(arg)
	case "chanlink":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
		d.chatList.ShowShareChannel(arg)
	case "chanreport":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
		d.chatList.ShowReportChannel(arg)
	case "chanreact":
		d.chatList.SelectTab("channels")
		d.chatList.OpenChannelJID(arg)
		glib.TimeoutAdd(400, func() bool { d.chatList.PopupChannelReactionPicker(); return false })
	case "discover":
		d.chatList.SelectTab("channels")
		d.chatList.ShowDiscover(arg, os.Getenv("CHATOT_SHOT_CAT"))
	case "followlink":
		d.chatList.SelectTab("channels")
		d.chatList.ShowFollowLink()
	case "communityopen":
		d.chatList.SelectTab("communities")
		d.chatList.OpenCommunityJID(arg)
	case "commgroup":
		// The community CHATOT_SHOT_ARG, then its first group row (the
		// announcement group) opened as a click would.
		d.chatList.SelectTab("communities")
		d.chatList.OpenCommunityJID(arg)
		glib.TimeoutAdd(2500, func() bool { d.chatList.OpenCommunityGroupAt(0); return false })
	case "commmenu":
		d.chatList.SelectTab("communities")
		d.chatList.OpenCommunityJID(arg)
		glib.TimeoutAdd(300, func() bool { d.chatList.PopupCommunityMenu(); return false })
	case "commrowmenu":
		d.chatList.SelectTab("communities")
		d.chatList.PopupCommunityRowMenu(arg)
	case "comminfo":
		d.chatList.SelectTab("communities")
		d.chatList.OpenCommunityJID(arg)
		d.chatList.ShowCommunityInfo(arg, os.Getenv("CHATOT_SHOT_CAT"))
	case "comminvite":
		d.chatList.SelectTab("communities")
		d.chatList.OpenCommunityJID(arg)
		d.chatList.ShowCommunityInvite(arg)
	case "commaddgroup":
		d.chatList.SelectTab("communities")
		d.chatList.OpenCommunityJID(arg)
		d.chatList.ShowAddGroup(arg)
	case "joinlink":
		d.chatList.SelectTab("communities")
		d.chatList.ShowJoinLink(os.Getenv("CHATOT_SHOT_TEXT"))
	case "joingroup":
		d.chatList.SelectTab("communities")
		d.chatList.OpenCommunityJID(arg)
		d.chatList.ConfirmFirstJoin(arg)
	case "appmenu":
		d.chatList.PopupAppMenu()
	case "chatmenu":
		d.conversation.PopupHeaderMenu()
	case "hover":
		d.conversation.ShowHoverActions(msgIdx)
	case "tray":
		// Two files so the thumbnail strip, its ✕ and the ＋ tile all render;
		// CHATOT_SHOT_ARG=a.mp4:b.pdf queues those instead.
		paths := []string{"internal/ui/tray_icon.png", "go.mod"}
		if arg != "" {
			paths = strings.Split(arg, ":")
		}
		d.composer.ShowTray(paths)
	case "scrolltop":
		// CHATOT_SHOT_MSG doubles as a percent here (0..100) so a capture can
		// reach any point in a thread the newest-message auto-scroll hides.
		pct := msgIdx
		if pct < 0 {
			pct = 0
		}
		d.conversation.ScrollThreadTo(float64(pct) / 100)
	case "scrollpx":
		// CHATOT_SHOT_MSG is an absolute offset in CSS px here.
		d.conversation.ScrollThreadPx(float64(msgIdx))
	case "msgmenu":
		d.conversation.PopupMessageMenu(msgIdx)
	case "reactpill":
		d.conversation.PopupReactPill(msgIdx)
	case "reactions":
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			go func() {
				if err := d.composer.ReactTo(m, "👍"); err != nil {
					log.Printf("chatot: shot react: %v", err)
				}
				glib.IdleAdd(func() { d.conversation.ApplyOwnReaction(m.ChatJID) })
			}()
		}
	case "reply":
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			d.composer.StartReply(m)
		}
	case "emoji":
		d.composer.PopEmoji()
	case "gif":
		d.composer.PopPicker("gif")
	case "stickers":
		d.composer.PopPicker("stickers")
	case "recording":
		d.composer.ShowRecordingUI()
	case "draft":
		d.composer.SetDraft("On my way, see you at noon")
	case "search":
		// In-chat search for CHATOT_SHOT_TEXT (default "relay"), landing on
		// its first hit.
		q := os.Getenv("CHATOT_SHOT_TEXT")
		if q == "" {
			q = "relay"
		}
		d.conversation.OpenSearch(q)
	case "listsearch":
		// The chat list's search box with CHATOT_SHOT_TEXT typed in.
		d.chatList.SearchList(os.Getenv("CHATOT_SHOT_TEXT"))
	case "starred":
		d.chatList.ShowStarred()
	case "archived":
		d.chatList.ShowArchived()
	case "newchat":
		d.chatList.ShowNewChat()
	case "newgroup":
		d.chatList.ShowNewGroup()
	case "newcommunity":
		d.chatList.ShowNewCommunity()
	case "joininvite":
		d.chatList.ShowJoinInvite()
	case "labelspop":
		d.chatList.PopupLabelOverflow()
	case "labelfilter":
		d.chatList.FilterByLabel(arg)
	case "compose":
		// A draft in the pill; "\n" in CHATOT_SHOT_TEXT breaks the line.
		d.composer.SetDraft(strings.ReplaceAll(os.Getenv("CHATOT_SHOT_TEXT"), `\n`, "\n"))
	case "rowmenu":
		d.chatList.PopupRowMenu(jid)
	case "empty":
		d.chatList.SetSearchText("zzz")
	case "typing":
		// The open chat's peer starts composing (fake only): the header
		// subtitle, the chat-list row and the thread's dotted bubble.
		var active client.Client = d.c
		if d.am != nil {
			active = d.am.ActiveClient()
		}
		if f, ok := active.(*client.Fake); ok && jid != "" {
			f.PushEvent(client.Event{Kind: client.EventChatPresence, ChatPresence: &client.ChatPresence{ChatJID: jid, State: "composing"}})
		}
	case "arrive":
		// ARG messages, 1.5 s apart, land in a chat other than the open one
		// (fake only): each raises a desktop notification and its chime.
		var active client.Client = d.c
		if d.am != nil {
			active = d.am.ActiveClient()
		}
		if f, ok := active.(*client.Fake); ok {
			n, _ := strconv.Atoi(arg)
			if n < 1 {
				n = 1
			}
			left := n
			glib.TimeoutAdd(1500, func() bool {
				f.Receive("1112223333@s.whatsapp.net", "1112223333@s.whatsapp.net", "Ping "+strconv.Itoa(n-left+1))
				left--
				return left > 0
			})
		}
	case "bgarrive":
		// The open chat's peer sends a message once the window has lost
		// focus (fake only): the pill and the row's badge appear, and both
		// must go when the window comes back and the pill has been seen.
		var active client.Client = d.c
		if d.am != nil {
			active = d.am.ActiveClient()
		}
		f, ok := active.(*client.Fake)
		if !ok || jid == "" {
			return
		}
		deliver := func() { f.Receive(jid, "4445556666@s.whatsapp.net", "Arrived while you were away") }
		if !d.win.IsActive() {
			deliver()
			return
		}
		var once bool
		d.win.NotifyProperty("is-active", func() {
			if once || d.win.IsActive() {
				return
			}
			once = true
			deliver()
		})
	case "vote":
		// Scrolls the thread to its top, then votes CHATOT_SHOT_ARG on the
		// poll at CHATOT_SHOT_MSG: the tally must land without the thread
		// moving.
		d.conversation.ScrollThreadTo(0.3)
		glib.TimeoutAdd(600, func() bool { d.conversation.VoteAt(msgIdx, arg); return false })
	case "mention":
		// The composer holds "@" + CHATOT_SHOT_TEXT with the cursor after
		// it, so the @ picker is up.
		d.composer.ShowMentionPicker(os.Getenv("CHATOT_SHOT_TEXT"))
	case "fullscreen":
		// The viewer on CHATOT_SHOT_MSG, then its ⤢: the fullscreen clip.
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			d.conversation.OpenViewer(m)
			glib.TimeoutAdd(400, func() bool { d.viewer.Fullscreen(); return false })
		}
	case "dlopen":
		// The bubble's own download then a click on the picture: the viewer
		// must open on the file, not ask to fetch it again. CHATOT_SHOT_TEXT
		// names a file whose bytes stand in for the fake's empty download.
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			path, err := d.c.DownloadMedia(context.Background(), m.ID)
			if err != nil {
				log.Printf("chatot: shot dlopen: %v", err)
				return
			}
			if src := os.Getenv("CHATOT_SHOT_TEXT"); src != "" {
				if b, err := os.ReadFile(src); err == nil {
					_ = os.WriteFile(path, b, 0o600)
				}
			}
			d.conversation.OpenDownloaded(m, path)
		}
	case "wheelzoom":
		// The viewer on CHATOT_SHOT_MSG, then CHATOT_SHOT_ARG wheel notches
		// (negative = out) over the picture's top-left quarter.
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			d.conversation.OpenViewer(m)
			n, _ := strconv.Atoi(arg)
			glib.TimeoutAdd(600, func() bool {
				step := 1
				if n < 0 {
					step, n = -1, -n
				}
				for i := 0; i < n; i++ {
					d.viewer.WheelZoom(math.Pow(1.12, float64(step)), 200, 150)
				}
				return false
			})
		}
	case "viewerswitch":
		// The viewer on CHATOT_SHOT_MSG, then the chat CHATOT_SHOT_ARG is
		// opened as a list click would: the pane must land on the thread.
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			d.conversation.OpenViewer(m)
			glib.TimeoutAdd(400, func() bool {
				d.win.Application().ActivateAction("open-chat", glib.NewVariantString(arg))
				return false
			})
		}
	case "viewer":
		// Opens the attachment viewer on the message at CHATOT_SHOT_MSG;
		// CHATOT_SHOT_ARG=info also opens the details sidebar.
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			d.conversation.OpenViewer(m)
			if arg == "info" {
				glib.TimeoutAdd(300, func() bool { d.viewer.ToggleDetails(); return false })
			}
		}
	case "deletedialog":
		// WhatsApp's delete prompt for the message at CHATOT_SHOT_MSG.
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			ui.ShowDeleteMessageDialog(d.win, d.c, m)
		}
	case "imageviewer":
		// CHATOT_SHOT_TEXT is the image file to show for the message at
		// CHATOT_SHOT_MSG (the fake has no real picture bytes).
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			ui.ShowImageViewerNow(d.win, m, os.Getenv("CHATOT_SHOT_TEXT"))
		}
	case "forward":
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			if pick := os.Getenv("CHATOT_SHOT_TEXT"); pick != "" {
				ui.ForwardInitialPick = strings.Split(pick, ",")
			}
			ui.ShowForwardDialog(d.win, d.c, m, d.toasts)
		}
	case "export":
		ui.ShowExportDialog(d.win, d.c, jid, name, d.toasts)
	case "clear":
		ui.ShowClearChatDialog(d.win, d.c, d.conversation, jid, name)
	case "groupinfo":
		d.conversation.ShowGroupInfo()
	case "groupinvite":
		d.conversation.ShowGroupInvite()
	case "timer":
		d.conversation.ShowDisappearing()
	case "contactinfo":
		d.conversation.ShowContactInfo()
	case "joinrequests":
		d.conversation.ShowJoinRequests()
	case "composersize":
		d.composer.LogButtonSizes()
	case "mute":
		d.conversation.ShowMute()
	case "blockconfirm":
		d.conversation.ShowBlockConfirm(name)
	case "about":
		d.chatList.ShowAbout()
	case "shortcuts":
		d.chatList.ShowShortcuts()
	case "blocked":
		d.chatList.ShowBlocked()
	case "privacy":
		d.chatList.ShowPrivacy()
	case "loading":
		d.stack.SetVisibleChildName("loading")
	case "location":
		// CHATOT_SHOT_ARG=manual|denied picks the sheet's state (neither
		// asks the system for a position); CHATOT_SHOT_TEXT=lat,lon drops
		// a pin there on the manual tab.
		d.composer.ShowLocationPicker(arg, os.Getenv("CHATOT_SHOT_TEXT"))
	case "poll":
		d.composer.ShowPollDialog()
	case "contactpick":
		d.composer.ShowContactPicker()
	case "linking":
		d.linking.SetQR("chatot-demo-pairing-code")
		d.linking.SetStatus("Waiting for you to scan…")
		d.stack.SetVisibleChildName("linking")
	case "syncing":
		// CHATOT_SHOT_ARG=empty shows the pre-first-chunk prompt.
		if arg == "empty" {
			d.sync.SetCounts("")
		} else {
			d.sync.SetCounts("128 chats · 4,213 messages")
		}
		d.stack.SetVisibleChildName("syncing")
	case "syncbanner":
		d.chatList.SetSyncProgress("Syncing older messages · 43%", 0.43)
	case "toast":
		d.toasts.AddToast(adw.NewToast("Message copied to clipboard"))
	case "reactpick":
		d.conversation.PopupReactionPicker(msgIdx)
	case "groupname":
		d.chatList.ShowGroupName()
	case "newgrouppicked":
		d.chatList.ShowNewGroupPicked()
	case "groupphoto":
		d.chatList.ShowGroupName()
		d.chatList.PickGroupPhotoNow(os.Getenv("CHATOT_SHOT_PHOTO"))
	case "groupcreate":
		// Drives the whole create path (the report was a crash on the
		// Create click), landing in the new group's thread.
		d.chatList.ShowGroupName()
		d.chatList.CreateGroupNow("Shot group")
	case "relink":
		if d.am != nil {
			d.chatList.ShowRelink(d.am)
		}
	case "join":
		d.chatList.ShowJoinInvite()
	case "msginfo":
		if m, ok := d.conversation.MessageAt(msgIdx); ok {
			ui.ShowMessageInfo(d.win, m)
		}
	case "merged":
		d.chatList.ShowMerged()
	case "switcher":
		d.chatList.PopupAccountSwitcher()
	case "addaccount":
		if d.am != nil {
			ui.ShowAddAccountDialog(d.win, d.am, refresh)
		}
	case "manage":
		if d.am != nil {
			ui.ShowManageAccountsDialog(d.win, d.am, d.prefs, refresh, d.saveSettings)
		}
	default:
		log.Printf("chatot: unknown CHATOT_SHOT state %q", state)
	}
}

// ensureComposeInput keeps dead keys and the Compose key working on Wayland.
// GTK's Wayland input-method context hands key composition to the
// compositor's input method; on a compositor with none attached (niri, sway,
// ...) a Compose or dead-key sequence then types its raw characters
// ("n~ao"). The simple context composes in-process. It is only forced when
// nothing suggests a real input method (fcitx/ibus export XMODIFIERS or set
// GTK_IM_MODULE themselves), so an IME user keeps theirs.
func ensureComposeInput() {
	if os.Getenv("WAYLAND_DISPLAY") == "" || os.Getenv("GTK_IM_MODULE") != "" {
		return
	}
	for _, v := range []string{"XMODIFIERS", "INPUT_METHOD", "QT_IM_MODULE"} {
		if os.Getenv(v) != "" {
			return
		}
	}
	os.Setenv("GTK_IM_MODULE", "gtk-im-context-simple")
}

// chatByJIDOrEmpty is the chat row for jid from the current chat list, or a
// bare Chat carrying only the JID when it isn't listed.
func chatByJIDOrEmpty(c client.Client, jid string) client.Chat {
	chats, err := c.Chats(0)
	if err == nil {
		for _, ch := range chats {
			if ch.JID == jid {
				return ch
			}
		}
	}
	return client.Chat{JID: jid}
}
