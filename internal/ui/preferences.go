package ui

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
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
	dialog.SetDefaultSize(720, 445)

	stack := gtk.NewStack()
	stack.SetHExpand(true)
	stack.SetVExpand(true)

	build := map[string]func() gtk.Widgetter{
		"appearance":    func() gtk.Widgetter { return prefAppearance(s, onChange) },
		"notifications": func() gtk.Widgetter { return prefNotifications(s, onChange) },
		"privacy":       func() gtk.Widgetter { return prefPrivacy(dialog, s, c, onChange) },
		"network":       func() gtk.Widgetter { return prefNetwork(s, onChange) },
		"shortcuts":     func() gtk.Widgetter { return prefShortcuts() },
		"advanced":      func() gtk.Widgetter { return prefAdvanced() },
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

func prefAppearance(s *settings.Settings, onChange func(settings.Settings)) gtk.Widgetter {
	card := newSettingsCard()

	// A dropdown, not the mockup's click-to-cycle row: three states are more
	// than a cycling affordance can communicate.
	dropdown := gtk.NewDropDownFromStrings(themeOptions)
	dropdown.SetSelected(themeIndex(s.Theme))
	dropdown.NotifyProperty("selected", func() {
		i := dropdown.Selected()
		if int(i) >= len(themeValues) {
			return
		}
		s.Theme = themeValues[i]
		ApplyTheme(s.Theme)
		onChange(*s)
	})
	card.Add(dropdownRow("Colour scheme", dropdown))

	return prefPage(newSettingsGroup("THEME", card))
}

func prefNotifications(s *settings.Settings, onChange func(settings.Settings)) gtk.Widgetter {
	alerts := newSettingsCard()
	desktop, _ := newSwitchRow("Desktop notifications", "", s.ShowNotifications, func(on bool) {
		s.ShowNotifications = on
		NotificationsEnabled = on
		onChange(*s)
	})
	alerts.Add(desktop)
	sound, _ := newSwitchRow("Notification sound",
		"Plays a chime with each notification",
		s.NotificationSound, func(on bool) {
			s.NotificationSound = on
			NotificationSound = on
			onChange(*s)
		})
	alerts.Add(sound)
	perAccount, _ := newSwitchRow("Label notifications by account",
		"Prefixes each notification with the account it arrived on",
		s.NotificationsPerAccount, func(on bool) {
			s.NotificationsPerAccount = on
			NotificationsPerAccount = on
			onChange(*s)
		})
	alerts.Add(perAccount)

	return prefPage(newSettingsGroup("ALERTS", alerts))
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
	security.Add(newActionRow("Blocked contacts", "", "View", false, func() {
		showBlockedDialog(dialog.Window(), c)
	}))

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

func prefNetwork(s *settings.Settings, onChange func(settings.Settings)) gtk.Widgetter {
	proxy := newSettingsCard()

	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.AddCSSClass("chatot-card-row")
	row.Append(settingsRowBody("Proxy URL", "socks5:// or http:// — applied on next launch"))
	entry := gtk.NewEntry()
	entry.SetText(s.Proxy)
	entry.SetPlaceholderText("Direct connection")
	entry.SetVAlign(gtk.AlignCenter)
	entry.SetSizeRequest(220, -1)
	entry.ConnectChanged(func() {
		s.Proxy = entry.Text()
		onChange(*s)
	})
	row.Append(entry)
	proxy.Add(row)

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
		newSettingsGroup("ACCOUNTS", accounts),
	)
}

func prefShortcuts() gtk.Widgetter {
	card := newSettingsCard()
	for _, sc := range appShortcuts() {
		row := gtk.NewBox(gtk.OrientationHorizontal, 12)
		row.AddCSSClass("chatot-card-row")
		row.Append(settingsRowBody(sc.Action, ""))
		keys := gtk.NewLabel(sc.Keys)
		keys.AddCSSClass("chatot-menu-accel")
		keys.SetVAlign(gtk.AlignCenter)
		row.Append(keys)
		card.Add(row)
	}
	return prefPage(newSettingsGroup("KEYBOARD", card))
}

func prefAdvanced() gtk.Widgetter {
	card := newSettingsCard()

	stateDir := chatotStateDir()
	card.Add(newActionRow("Application data", stateDir, "Open", stateDir == "", func() {
		openFile(stateDir)
	}))
	card.Add(newActionRow("Settings file", filepath.Join(settings.Dir(), "settings.json"), "Open", false, func() {
		openFile(settings.Dir())
	}))

	return prefPage(newSettingsGroup("STORAGE", card))
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
