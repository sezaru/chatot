package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/settings"
)

// themeOptions is the Theme combo row's fixed choice list, in the order
// shown; index 0 is "system" (inherit the desktop's color scheme).
var themeOptions = []string{"System", "Light", "Dark"}

// themeSchemes maps themeOptions positions to the adw.ColorScheme
// StyleManager.SetColorScheme expects.
var themeSchemes = []adw.ColorScheme{
	adw.ColorSchemeDefault,
	adw.ColorSchemeForceLight,
	adw.ColorSchemeForceDark,
}

// themeValues maps themeOptions positions to the Settings.Theme string.
var themeValues = []string{"system", "light", "dark"}

// themeIndex returns the combo row position for a stored theme value,
// defaulting to "system" for anything unrecognized.
func themeIndex(theme string) uint {
	for i, v := range themeValues {
		if v == theme {
			return uint(i)
		}
	}
	return 0
}

// ApplyTheme sets the default AdwStyleManager's color scheme from a stored
// Settings.Theme value. Exported so main.go can apply the saved theme at
// startup, before the Preferences window (which owns the combo row driving
// this same call) is ever opened.
func ApplyTheme(theme string) {
	adw.StyleManagerGetDefault().SetColorScheme(themeSchemes[themeIndex(theme)])
}

// ShowPreferences builds and presents an adw.PreferencesWindow with
// General and Privacy pages, seeded from s. Every row applies its change
// immediately (live) and calls onChange with the updated Settings so the
// caller can persist it.
func ShowPreferences(parent *gtk.Window, s *settings.Settings, onChange func(settings.Settings)) {
	win := adw.NewPreferencesWindow()
	win.SetTransientFor(parent)
	win.SetModal(true)
	win.SetSearchEnabled(false)

	win.Add(generalPage(s, onChange))
	win.Add(privacyPage(s, onChange))

	win.Present()
}

// generalPage builds the Appearance + Notifications page.
func generalPage(s *settings.Settings, onChange func(settings.Settings)) *adw.PreferencesPage {
	page := adw.NewPreferencesPage()
	page.SetTitle("General")
	page.SetIconName("preferences-system-symbolic")

	appearance := adw.NewPreferencesGroup()
	appearance.SetTitle("Appearance")

	themeRow := adw.NewComboRow()
	themeRow.SetTitle("Theme")
	themeRow.SetModel(gtk.NewStringList(themeOptions))
	themeRow.SetSelected(themeIndex(s.Theme))
	themeRow.NotifyProperty("selected", func() {
		i := themeRow.Selected()
		if int(i) >= len(themeValues) {
			return
		}
		s.Theme = themeValues[i]
		ApplyTheme(s.Theme)
		onChange(*s)
	})
	appearance.Add(themeRow)

	notifications := adw.NewPreferencesGroup()
	notifications.SetTitle("Notifications")

	notifyRow := adw.NewSwitchRow()
	notifyRow.SetTitle("Show notifications")
	notifyRow.SetSubtitle("Desktop notifications for new messages")
	notifyRow.SetActive(s.ShowNotifications)
	notifyRow.NotifyProperty("active", func() {
		s.ShowNotifications = notifyRow.Active()
		onChange(*s)
	})
	notifications.Add(notifyRow)

	page.Add(appearance)
	page.Add(notifications)
	return page
}

// privacyPage builds the read-receipts/typing-indicators page. These two
// rows are the only ones that also flip a live package var (ui.
// SendReadReceipts, ui.SendTypingIndicators) in addition to the Settings
// struct, since the composer reads those vars directly on every send.
func privacyPage(s *settings.Settings, onChange func(settings.Settings)) *adw.PreferencesPage {
	page := adw.NewPreferencesPage()
	page.SetTitle("Privacy")
	page.SetIconName("channel-secure-symbolic")

	group := adw.NewPreferencesGroup()
	group.SetTitle("Messaging")

	readReceiptsRow := adw.NewSwitchRow()
	readReceiptsRow.SetTitle("Read receipts")
	readReceiptsRow.SetSubtitle("Let contacts see when you've read their messages")
	readReceiptsRow.SetActive(s.SendReadReceipts)
	readReceiptsRow.NotifyProperty("active", func() {
		active := readReceiptsRow.Active()
		s.SendReadReceipts = active
		SendReadReceipts = active
		onChange(*s)
	})
	group.Add(readReceiptsRow)

	typingRow := adw.NewSwitchRow()
	typingRow.SetTitle("Typing indicators")
	typingRow.SetSubtitle("Let contacts see when you're typing")
	typingRow.SetActive(s.SendTypingIndicators)
	typingRow.NotifyProperty("active", func() {
		active := typingRow.Active()
		s.SendTypingIndicators = active
		SendTypingIndicators = active
		onChange(*s)
	})
	group.Add(typingRow)

	page.Add(group)
	return page
}
