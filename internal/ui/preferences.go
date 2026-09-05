package ui

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
	"chatot/internal/settings"
)

var themeOptions = []string{"System", "Light", "Dark"}

// themeSchemes maps a saved theme value to the AdwStyleManager scheme.
var themeSchemes = []adw.ColorScheme{
	adw.ColorSchemeDefault,
	adw.ColorSchemeForceLight,
	adw.ColorSchemeForceDark,
}

var themeValues = []string{"system", "light", "dark"}

// themeIndex maps a saved theme value to its index in themeOptions,
// defaulting to "System" for anything unrecognised.
func themeIndex(theme string) uint {
	for i, v := range themeValues {
		if v == theme {
			return uint(i)
		}
	}
	return 0
}

// ApplyTheme pushes the saved theme onto the AdwStyleManager.
func ApplyTheme(theme string) {
	adw.StyleManagerGetDefault().SetColorScheme(themeSchemes[themeIndex(theme)])
}

// PreferenceHooks are the app-level actions Preferences reaches for that
// live outside the ui package: main.go fills it in once the window exists.
type PreferenceHooks struct {
	// ManageAccounts opens the Accounts card (the Account rail row).
	ManageAccounts func()
	// RefreshChatList rebuilds the sidebar rows after a preference that
	// changes how they render.
	RefreshChatList func()
	// SetTrayVisible adds or removes the tray icon.
	SetTrayVisible func(show bool)
	// Toasts is the main window's toast overlay for short confirmations.
	Toasts *adw.ToastOverlay
	// LogFile is the capped log file's path, "" when none could be opened.
	LogFile string
}

// Prefs is the live PreferenceHooks.
var Prefs PreferenceHooks

func prefToast(text string) { showToast(Prefs.Toasts, text) }

// prefPages are the mockup's six Preferences tabs, in its order.
var prefPages = []struct{ ID, Icon, Label string }{
	{"appearance", "🎨", "Appearance"},
	{"notifications", "🔔", "Notifications"},
	{"privacy", "🔒", "Privacy"},
	{"network", "🌐", "Network"},
	{"shortcuts", "⌨", "Shortcuts"},
	{"advanced", "🛠", "Advanced"},
}

// ShowPreferences opens the mockup's Preferences card: a 720×445 sheet with a
// left icon nav and, on the right, captioned groups of bordered rows.
//
// This replaces an AdwPreferencesWindow with a top view-switcher and three
// pages, which the design never had. Building it out of the shared settings
// card also gave the three surfaces that had been parked in the ⋮ menu —
// Blocked contacts, Privacy settings and Keyboard shortcuts — the home the
// design intends for them.
func ShowPreferences(parent *gtk.Window, s *settings.Settings, c client.Client, onChange func(settings.Settings)) {
	dialog := newCardDialog()
	dialog.SetTitle("Preferences")
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(720, 447)

	stack := gtk.NewStack()
	stack.SetHExpand(true)
	stack.SetVExpand(true)
	// Pages differ in height; a homogeneous stack would size every page to
	// the tallest and leave the scroller nothing to do.
	stack.SetVhomogeneous(false)

	build := map[string]func() gtk.Widgetter{
		"appearance":    func() gtk.Widgetter { return prefAppearance(parent, s, c, onChange) },
		"notifications": func() gtk.Widgetter { return prefNotifications(dialog, s, onChange) },
		"privacy":       func() gtk.Widgetter { return prefPrivacy(dialog, s, c, onChange) },
		"network":       func() gtk.Widgetter { return prefNetwork(s, onChange) },
		"shortcuts":     func() gtk.Widgetter { return prefShortcuts() },
		"advanced":      func() gtk.Widgetter { return prefAdvanced(dialog, s, c, onChange) },
	}
	for _, page := range prefPages {
		scroller := gtk.NewScrolledWindow()
		scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
		scroller.SetChild(build[page.ID]())
		stack.AddNamed(scroller, page.ID)
	}

	cols := gtk.NewBox(gtk.OrientationHorizontal, 0)
	cols.Append(newPrefNav(stack))
	cols.Append(stack)

	if PreferencesInitialPage != "" {
		stack.SetVisibleChildName(PreferencesInitialPage)
		PreferencesInitialPage = ""
	}

	dialog.SetChild(cols)
	dialog.Present()
}

// PreferencesInitialPage, when set, is the page the next ShowPreferences
// opens on (a prefPages ID such as "privacy"); it is consumed on open. The
// ⋮ menu's former Privacy and Keyboard shortcuts rows land here.
var PreferencesInitialPage string

// newPrefNav is the 170px left rail: one glyph+label button per page, the
// current one filled.
func newPrefNav(stack *gtk.Stack) gtk.Widgetter {
	nav := gtk.NewBox(gtk.OrientationVertical, 2)
	nav.AddCSSClass("chatot-pref-nav")
	nav.SetSizeRequest(170, -1)
	// The labels inside expand to fill their button; without pinning the
	// rail's own expand flag that propagates up and the rail takes half the
	// card.
	nav.SetHExpand(false)

	var buttons []*gtk.ToggleButton
	sync := func() {
		current := stack.VisibleChildName()
		for i, b := range buttons {
			b.SetActive(prefPages[i].ID == current)
		}
	}
	for _, page := range prefPages {
		id := page.ID
		btn := gtk.NewToggleButton()
		btn.SetChild(prefNavLabel(page.Icon, page.Label))
		btn.AddCSSClass("chatot-pref-navitem")
		btn.ConnectClicked(func() {
			stack.SetVisibleChildName(id)
			sync()
		})
		buttons = append(buttons, btn)
		nav.Append(btn)
	}
	stack.NotifyProperty("visible-child-name", sync)
	sync()
	return nav
}

func prefNavLabel(icon, label string) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 9)
	glyph := gtk.NewLabel(icon)
	glyph.SetSizeRequest(16, -1)
	row.Append(glyph)
	text := gtk.NewLabel(label)
	text.SetXAlign(0)
	text.SetHExpand(true)
	row.Append(text)
	return row
}

// prefPage is the padded right-hand column every page's groups go into.
func prefPage(groups ...gtk.Widgetter) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 14)
	box.AddCSSClass("chatot-pref-page")
	for _, g := range groups {
		box.Append(g)
	}
	return box
}

// choiceRow is the mockup's "value ▾" row: clicking it drops a menu of
// labels under the row and hands the pick's index to onPick, which
// returns the new value text.
func choiceRow(label, sub, value string, labels []string, onPick func(i int) string) gtk.Widgetter {
	var row gtk.Widgetter
	var valueLabel *gtk.Label
	row, valueLabel = newValueRow(label, sub, value, func() {
		items := make([]menuItem, 0, len(labels))
		for i, l := range labels {
			i := i
			items = append(items, menuItem{Label: l, OnActivate: func() {
				valueLabel.SetText(onPick(i) + " ▾")
			}})
		}
		popupMenuBelow(row, items)
	})
	return row
}

func prefAppearance(parent *gtk.Window, s *settings.Settings, c client.Client, onChange func(settings.Settings)) gtk.Widgetter {
	theme := newSettingsCard()
	theme.Add(choiceRow("Colour scheme", "", themeOptions[themeIndex(s.Theme)], themeOptions, func(i int) string {
		s.Theme = themeValues[i]
		ApplyTheme(s.Theme)
		onChange(*s)
		return themeOptions[i]
	}))
	controls, _ := newSwitchRow("Show window controls",
		"Hides minimize, maximize and close when off",
		s.ShowWindowControls, func(on bool) {
			s.ShowWindowControls = on
			ApplyWindowControls(on)
			onChange(*s)
		})
	theme.Add(controls)
	linked := "1 linked"
	if am, ok := c.(*client.AccountManager); ok {
		linked = fmt.Sprintf("%d linked", am.Count())
	}
	theme.Add(newActionRow("Account rail",
		"Shown automatically while more than one account is linked",
		linked, false, Prefs.ManageAccounts))

	list := newSettingsCard()
	list.Add(choiceRow("Font size", "", fontSizeLabel(s.FontSize), []string{"Small", "Default", "Large"}, func(i int) string {
		s.FontSize = settings.FontSizes[i]
		if parent != nil {
			ApplyFontSize(parent, s.FontSize)
		}
		onChange(*s)
		return fontSizeLabel(s.FontSize)
	}))
	previews, _ := newSwitchRow("Show message previews", "", s.ShowMessagePreviews, func(on bool) {
		s.ShowMessagePreviews = on
		ShowMessagePreviews = on
		if Prefs.RefreshChatList != nil {
			Prefs.RefreshChatList()
		}
		onChange(*s)
	})
	list.Add(previews)

	return prefPage(
		newSettingsGroup("THEME", theme),
		newSettingsGroup("CHAT LIST", list),
	)
}

// soundSourceText describes where the chime comes from, for the Sound
// file row's subtitle.
func soundSourceText(path, source string) string {
	switch source {
	case soundSourceCustom:
		return filepath.Base(path)
	case soundSourceDropIn:
		return "Drop-in file " + filepath.Base(path)
	case soundSourcePackage:
		return "Packaged default: " + filepath.Base(path)
	}
	return "Built-in chime"
}

func prefNotifications(dialog *cardDialog, s *settings.Settings, onChange func(settings.Settings)) gtk.Widgetter {
	alerts := newSettingsCard()
	sound, _ := newSwitchRow("Play sound", "", s.NotificationSound, func(on bool) {
		s.NotificationSound = on
		NotificationSound = on
		onChange(*s)
	})
	alerts.Add(sound)
	desktop, _ := newSwitchRow("Desktop notifications", "", s.ShowNotifications, func(on bool) {
		s.ShowNotifications = on
		NotificationsEnabled = on
		onChange(*s)
	})
	alerts.Add(desktop)
	text, _ := newSwitchRow("Show message text in notifications",
		"Off shows only that a message arrived",
		s.NotificationText, func(on bool) {
			s.NotificationText = on
			NotificationText = on
			onChange(*s)
		})
	alerts.Add(text)
	perAccount, _ := newSwitchRow("Label notifications by account",
		"Prefixes each notification with the account it arrived on",
		s.NotificationsPerAccount, func(on bool) {
			s.NotificationsPerAccount = on
			NotificationsPerAccount = on
			onChange(*s)
		})
	alerts.Add(perAccount)

	// The chime itself: pick any audio file (MP3 included; it is transcoded
	// before GTK sees it), hear it, or go back to the default.
	chime := newSettingsCard()
	var sourceSub *gtk.Label
	var resetRow gtk.Widgetter
	refreshSource := func() {
		path, source := currentNotificationSound()
		sourceSub.SetText(soundSourceText(path, source))
		gtk.BaseWidget(resetRow).SetSensitive(s.NotificationSoundFile != "")
	}
	setSound := func(path string) {
		s.NotificationSoundFile = path
		NotificationSoundFile = path
		onChange(*s)
		refreshSource()
	}
	fileRow, _ := newActionRowLabel("Sound file", "", "Choose…", false, func() {
		pickSoundFile(dialog.Window(), func(path string) {
			setSound(path)
			playSoundFile(path, func(err error) {
				prefToast("Couldn't play that file: " + err.Error())
				setSound("")
			})
		})
	})
	// The subtitle is filled by refreshSource; reach it through the row.
	sourceSub = rowSubLabel(fileRow)
	chime.Add(fileRow)
	chime.Add(newActionRow("Preview", "", "Play", false, func() { playNotificationSound() }))
	resetRow = newActionRow("Use the default sound", "", "Reset", false, func() { setSound("") })
	chime.Add(resetRow)
	refreshSource()

	tray := newSettingsCard()
	show, _ := newSwitchRow("Show tray icon", "Tooltip shows the unread count", s.ShowTrayIcon, func(on bool) {
		s.ShowTrayIcon = on
		if Prefs.SetTrayVisible != nil {
			Prefs.SetTrayVisible(on)
		}
		onChange(*s)
	})
	tray.Add(show)
	closeTo, _ := newSwitchRow("Close to tray", "Closing the window keeps chatot running", s.CloseToTray, func(on bool) {
		s.CloseToTray = on
		onChange(*s)
	})
	tray.Add(closeTo)

	return prefPage(
		newSettingsGroup("ALERTS", alerts),
		newSettingsGroup("SOUND", chime),
		newSettingsGroup("TRAY", tray),
	)
}

// rowSubLabel adds an (initially empty) subtitle to a row built without one
// and returns it. Rows are a button around a box whose first child is the
// label column.
func rowSubLabel(row gtk.Widgetter) *gtk.Label {
	sub := gtk.NewLabel("")
	sub.SetXAlign(0)
	sub.SetWrap(true)
	sub.AddCSSClass("chatot-card-sub")
	if btn, ok := row.(*gtk.Button); ok {
		if box, ok := btn.Child().(*gtk.Box); ok {
			if col, ok := box.FirstChild().(*gtk.Box); ok {
				col.Append(sub)
			}
		}
	}
	return sub
}

// pickSoundFile opens an audio chooser and hands back the pick.
func pickSoundFile(parent *gtk.Window, onPicked func(path string)) {
	fd := gtk.NewFileDialog()
	fd.SetTitle("Choose a notification sound")
	filter := gtk.NewFileFilter()
	filter.SetName("Audio")
	filter.AddMIMEType("audio/*")
	filters := gio.NewListStore(glib.TypeObject)
	filters.Append(filter.Object)
	fd.SetFilters(filters)
	fd.Open(context.Background(), parent, func(res gio.AsyncResulter) {
		file, err := fd.OpenFinish(res)
		if err != nil || file == nil {
			return
		}
		onPicked(file.Path())
	})
}

func prefPrivacy(dialog *cardDialog, s *settings.Settings, c client.Client, onChange func(settings.Settings)) gtk.Widgetter {
	receipts := newSettingsCard()
	read, _ := newSwitchRow("Send read receipts",
		"When off, senders never learn you opened a chat",
		s.SendReadReceipts, func(on bool) {
			s.SendReadReceipts = on
			SendReadReceipts = on
			onChange(*s)
		})
	receipts.Add(read)
	typing, _ := newSwitchRow("Share typing status", "", s.SendTypingIndicators, func(on bool) {
		s.SendTypingIndicators = on
		SendTypingIndicators = on
		onChange(*s)
	})
	receipts.Add(typing)

	location := newSettingsCard()
	locRow, _ := newSwitchRow("Location access",
		"Ask the system for a position when you send a location. Off leaves only the map picker.",
		s.LocationAccess, func(on bool) {
			s.LocationAccess = on
			LocationAccess = on
			onChange(*s)
		})
	location.Add(locRow)

	security := newSettingsCard()
	blockedRow, blockedWord := newActionRowLabel("Blocked contacts", "", "View", false, func() {
		showBlockedDialog(dialog.Window(), c)
	})
	security.Add(blockedRow)
	go func() {
		list, err := c.Blocklist(context.Background())
		glib.IdleAdd(func() {
			if err != nil || blockedWord.Root() == nil {
				return
			}
			blockedWord.SetText(fmt.Sprintf("%d blocked", len(list)))
		})
	}()

	// The account's WhatsApp privacy settings (last seen, profile photo…)
	// come from the phone, so the group fills in once they arrive rather
	// than opening a second dialog.
	account := gtk.NewBox(gtk.OrientationVertical, 0)
	loading := gtk.NewLabel("Loading account privacy…")
	loading.SetXAlign(0)
	loading.AddCSSClass("chatot-card-sub")
	account.Append(loading)
	go func() {
		settings, err := c.PrivacySettings(context.Background())
		glib.IdleAdd(func() {
			// Preferences may have been closed while the phone answered.
			if account.Root() == nil {
				return
			}
			clearBox(account)
			card := newSettingsCard()
			if err != nil {
				loading.SetText("Couldn't load account privacy")
				account.Append(loading)
				return
			}
			for _, row := range privacySettingsRows(settings) {
				card.Add(newPrivacyRow(c, row.Name, row.Value))
			}
			account.Append(newSettingsGroup("ACCOUNT PRIVACY", card))
		})
	}()

	return prefPage(
		newSettingsGroup("RECEIPTS", receipts),
		newSettingsGroup("LOCATION", location),
		newSettingsGroup("SECURITY", security),
		account,
	)
}

// entryRow is a card row whose trailing control is a text field.
func entryRow(label, sub string, entry *gtk.Entry, width int) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.AddCSSClass("chatot-card-row")
	row.Append(settingsRowBody(label, sub))
	entry.AddCSSClass("chatot-pref-entry")
	entry.SetVAlign(gtk.AlignCenter)
	entry.SetSizeRequest(width, -1)
	row.Append(entry)
	return row
}

func prefNetwork(s *settings.Settings, onChange func(settings.Settings)) gtk.Widgetter {
	// The proxy is edited as type, host and port (the mockup's rows) but
	// stored as the one URL whatsmeow reads at startup.
	scheme, host, port := proxyParts(s.Proxy)
	hostEntry := gtk.NewEntry()
	hostEntry.SetText(host)
	hostEntry.SetPlaceholderText("127.0.0.1")
	portEntry := gtk.NewEntry()
	portEntry.SetText(port)
	portEntry.SetPlaceholderText("9050")
	portEntry.SetInputPurpose(gtk.InputPurposeDigits)
	setSensitive := func() {
		hostEntry.SetSensitive(scheme != "")
		portEntry.SetSensitive(scheme != "")
	}
	store := func() {
		s.Proxy = proxyURL(scheme, hostEntry.Text(), portEntry.Text())
		onChange(*s)
	}
	labels := make([]string, len(proxyTypes))
	for i, t := range proxyTypes {
		labels[i] = t.Label
	}
	proxy := newSettingsCard()
	proxy.Add(choiceRow("Proxy type", "Applied on the next launch", proxyTypeLabel(scheme), labels, func(i int) string {
		scheme = proxyTypes[i].Scheme
		setSensitive()
		store()
		return proxyTypes[i].Label
	}))
	proxy.Add(entryRow("Host", "", hostEntry, 200))
	proxy.Add(entryRow("Port", "", portEntry, 90))
	hostEntry.ConnectChanged(store)
	portEntry.ConnectChanged(store)
	setSensitive()

	var testWord *gtk.Label
	testRow, word := newActionRowLabel("Connection",
		"Reaches the proxy, or WhatsApp directly with none", "Test", false, func() {
			testWord.SetText("Testing…")
			url := proxyURL(scheme, hostEntry.Text(), portEntry.Text())
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				d, err := testProxy(ctx, url)
				glib.IdleAdd(func() {
					if testWord.Root() == nil {
						return
					}
					if err != nil {
						testWord.SetText("Unreachable · Test again")
						prefToast("Connection test failed: " + err.Error())
						return
					}
					testWord.SetText(fmt.Sprintf("Reachable · %d ms", d.Milliseconds()))
				})
			}()
		})
	testWord = word
	proxy.Add(testRow)

	media := newSettingsCard()
	modes := make([]string, len(settings.AutoDownloadModes))
	for i, m := range settings.AutoDownloadModes {
		modes[i] = autoDownloadLabel(m)
	}
	media.Add(choiceRow("Auto-download", "Media from the last 7 days; older waits for a click",
		autoDownloadLabel(s.AutoDownload), modes, func(i int) string {
			s.AutoDownload = settings.AutoDownloadModes[i]
			AutoDownload = s.AutoDownload
			onChange(*s)
			return autoDownloadLabel(s.AutoDownload)
		}))

	gifs := newSettingsCard()
	services := make([]string, len(settings.GIFServices))
	for i, id := range settings.GIFServices {
		services[i] = gifServiceLabel(id)
	}
	gifs.Add(choiceRow("GIF search", "Giphy gives out free app keys at developers.giphy.com",
		gifServiceLabel(s.GIFService), services, func(i int) string {
			s.GIFService = settings.GIFServices[i]
			GIFService = s.GIFService
			onChange(*s)
			return gifServiceLabel(s.GIFService)
		}))
	keyEntry := gtk.NewEntry()
	keyEntry.SetText(s.GIFAPIKey)
	keyEntry.SetPlaceholderText("API key")
	keyEntry.ConnectChanged(func() {
		s.GIFAPIKey = strings.TrimSpace(keyEntry.Text())
		GIFAPIKey = s.GIFAPIKey
		onChange(*s)
	})
	gifs.Add(entryRow("API key", "", keyEntry, 220))

	accounts := newSettingsCard()
	keep, _ := newSwitchRow("Keep inactive accounts connected",
		"When off, only the account you're viewing stays online",
		s.KeepInactiveConnected, func(on bool) {
			s.KeepInactiveConnected = on
			onChange(*s)
		})
	accounts.Add(keep)

	return prefPage(
		newSettingsGroup("PROXY", proxy),
		newSettingsGroup("MEDIA", media),
		newSettingsGroup("GIFS", gifs),
		newSettingsGroup("ACCOUNTS", accounts),
	)
}

func prefShortcuts() gtk.Widgetter {
	var groups []gtk.Widgetter
	for i, rows := range shortcutsByGroup() {
		card := newSettingsCard()
		for _, sc := range rows {
			row := gtk.NewBox(gtk.OrientationHorizontal, 12)
			row.AddCSSClass("chatot-card-row")
			row.Append(settingsRowBody(sc.Action, ""))
			keys := gtk.NewLabel(sc.Keys)
			keys.AddCSSClass("chatot-menu-accel")
			keys.SetVAlign(gtk.AlignCenter)
			row.Append(keys)
			card.Add(row)
		}
		groups = append(groups, newSettingsGroup(strings.ToUpper(shortcutGroups[i]), card))
	}
	return prefPage(groups...)
}

func prefAdvanced(dialog *cardDialog, s *settings.Settings, c client.Client, onChange func(settings.Settings)) gtk.Widgetter {
	storage := newSettingsCard()

	cache := cacheDir()
	var cacheWord *gtk.Label
	cacheRow, word := newActionRowLabel("Media cache", cache, "", false, func() {
		if err := clearDir(cache); err != nil {
			prefToast("Couldn't clear the cache: " + err.Error())
		}
		cacheWord.SetText(humanSize(dirSize(cache)) + " · Clear")
	})
	cacheWord = word
	cacheWord.SetText(humanSize(dirSize(cache)) + " · Clear")
	storage.Add(cacheRow)

	if si, ok := c.(client.StorageInfo); ok {
		if dir := si.MediaDir(); dir != "" {
			storage.Add(newActionRow("Downloaded media", dir, humanSize(dirSize(dir))+" · Open", false, func() {
				openFile(dir)
			}))
		}
		storage.Add(newActionRow("Message database", humanSize(si.DatabaseSize()), "Export", false, func() {
			exportDatabase(dialog.Window(), si)
		}))
	}

	debug := newSettingsCard()
	verbose, _ := newSwitchRow("Verbose logging", "WhatsApp protocol detail in the log", s.VerboseLogging, func(on bool) {
		s.VerboseLogging = on
		client.SetVerboseLogging(on)
		onChange(*s)
	})
	debug.Add(verbose)
	var openLog func()
	if Prefs.LogFile != "" {
		openLog = func() { openFile(Prefs.LogFile) }
	}
	debug.Add(newActionRow("Log file", Prefs.LogFile, "Open", false, openLog))

	files := newSettingsCard()
	stateDir := chatotStateDir()
	files.Add(newActionRow("Application data", stateDir, "Open", stateDir == "", func() {
		openFile(stateDir)
	}))
	files.Add(newActionRow("Settings file", filepath.Join(settings.Dir(), "settings.json"), "Open", false, func() {
		openFile(settings.Dir())
	}))

	return prefPage(
		newSettingsGroup("STORAGE", storage),
		newSettingsGroup("DEBUG", debug),
		newSettingsGroup("FILES", files),
	)
}

// exportDatabase asks where to save a copy of the message database and
// writes it there off the main loop.
func exportDatabase(parent *gtk.Window, si client.StorageInfo) {
	fd := gtk.NewFileDialog()
	fd.SetTitle("Export message database")
	fd.SetInitialName("chatot-" + time.Now().Format("2006-01-02") + ".db")
	fd.Save(context.Background(), parent, func(res gio.AsyncResulter) {
		file, err := fd.SaveFinish(res)
		if err != nil || file == nil {
			return
		}
		path := file.Path()
		go func() {
			// VACUUM INTO refuses to overwrite; the chooser already asked.
			os.Remove(path)
			err := si.BackupDatabase(context.Background(), path)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: export database: %v", err)
					prefToast("Export failed: " + err.Error())
					return
				}
				prefToast("Database exported to " + path)
			})
		}()
	})
}

// chatotStateDir mirrors main.go's state-dir resolution so the Advanced page
// can show (and open) where chatot keeps its database and session.
func chatotStateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "chatot")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "chatot")
}

// prefPagesLabels is the page titles in order, for tests asserting the nav
// matches the design.
func prefPagesLabels() []string {
	out := make([]string, 0, len(prefPages))
	for _, p := range prefPages {
		out = append(out, p.Label)
	}
	return out
}

// newPrivacyRow is one account privacy setting. Clicking it offers the
// values WhatsApp accepts for that setting and pushes the pick to the
// server; the row keeps its old value if the server refuses.
func newPrivacyRow(c client.Client, name, value string) gtk.Widgetter {
	options := client.PrivacySettingOptions(name)
	var row gtk.Widgetter
	var valueLabel *gtk.Label
	var onClick func()
	if len(options) > 0 {
		onClick = func() {
			items := make([]menuItem, 0, len(options))
			for _, opt := range options {
				o := opt
				items = append(items, menuItem{Label: client.PrivacySettingLabel(o), OnActivate: func() {
					go func() {
						err := c.SetPrivacySetting(context.Background(), name, o)
						glib.IdleAdd(func() {
							if err != nil {
								log.Printf("chatot: set privacy %s: %v", name, err)
								return
							}
							valueLabel.SetText(client.PrivacySettingLabel(o))
						})
					}()
				}})
			}
			popupMenuBelow(row, items)
		}
	}
	row, valueLabel = newValueRow(name, "", client.PrivacySettingLabel(value), onClick)
	return row
}

// gifServiceLabel names a settings.GIFServices value for the choice row.
func gifServiceLabel(id string) string {
	if id == "tenor" {
		return "Tenor"
	}
	return "Giphy"
}
