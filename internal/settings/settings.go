// Package settings persists chatot's user-facing preferences (read
// receipts, typing indicators, notifications, theme) as JSON under
// $XDG_CONFIG_HOME/chatot. Pure Go, no GTK or client dependencies, so it's
// usable from both the UI layer and tests without a display.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings holds every preference exposed by the Preferences window.
type Settings struct {
	SendReadReceipts     bool   `json:"sendReadReceipts"`
	SendTypingIndicators bool   `json:"sendTypingIndicators"`
	ShowNotifications    bool   `json:"showNotifications"`
	Theme                string `json:"theme"` // "system", "light", or "dark"
	// Proxy is a SOCKS5 or HTTP proxy URL (e.g. "socks5://host:port") applied
	// to the WhatsApp connection at startup; "" connects directly. Changing
	// it takes effect on the next launch, not the running session.
	Proxy string `json:"proxy"`
	// NotificationsPerAccount prefixes each toast title with the active
	// account's label (e.g. "Work · Sam Okafor") when more than one account is
	// linked, so it's clear which account a notification belongs to.
	NotificationsPerAccount bool `json:"notificationsPerAccount"`
	// KeepInactiveConnected keeps every linked account connected in the
	// background; when false only the account currently shown stays connected.
	KeepInactiveConnected bool `json:"keepInactiveConnected"`
	// LocationAccess lets the Send-location sheet ask the system (the XDG
	// location portal) for a position; off leaves only the map picker.
	LocationAccess bool `json:"locationAccess"`
	// NotificationSound plays a short chime with each desktop notification.
	// Notification daemons rarely ring on their own for app notifications, so
	// the app does it: the system sound theme's "message-new-instant" when a
	// theme is installed, otherwise a built-in chime.
	NotificationSound bool `json:"notificationSound"`
	// NotificationSoundFile is the audio file the chime plays instead of the
	// default; "" keeps the default (the CHATOT_NOTIFY_SOUND file a package
	// wires in, else the built-in chime).
	NotificationSoundFile string `json:"notificationSoundFile"`
	// NotificationText puts the message text in the notification body; off
	// shows only that a message arrived, for shared or locked screens.
	NotificationText bool `json:"notificationText"`
	// ShowTrayIcon keeps a StatusNotifierItem in the desktop's tray.
	ShowTrayIcon bool `json:"showTrayIcon"`
	// CloseToTray makes closing the window hide it (the tray brings it
	// back); off quits. Only honoured while the tray icon is shown.
	CloseToTray bool `json:"closeToTray"`
	// ShowWindowControls draws the minimize/maximize/close buttons the
	// desktop's gtk-decoration-layout asks for; off hides them all.
	ShowWindowControls bool `json:"showWindowControls"`
	// FontSize scales the chat list and message text: "small", "default"
	// or "large".
	FontSize string `json:"fontSize"`
	// ShowMessagePreviews puts the last message under each chat's name in
	// the list; off leaves the name alone.
	ShowMessagePreviews bool `json:"showMessagePreviews"`
	// AutoDownload picks which incoming media is fetched as soon as it is
	// shown: "always", "photos" (photos, stickers and voice notes) or
	// "never" (everything waits for a click).
	AutoDownload string `json:"autoDownload"`
	// VerboseLogging turns on whatsmeow's info and debug lines in the log.
	VerboseLogging bool `json:"verboseLogging"`
}

// FontSizes lists the FontSize values in display order.
var FontSizes = []string{"small", "default", "large"}

// AutoDownloadModes lists the AutoDownload values in display order.
var AutoDownloadModes = []string{"always", "photos", "never"}

// Default returns the preferences a fresh install starts with: chatot
// matches WhatsApp's own defaults (read receipts on, as the mockup's Privacy
// page shows them).
func Default() Settings {
	return Settings{
		SendReadReceipts:        true,
		SendTypingIndicators:    true,
		ShowNotifications:       true,
		Theme:                   "system",
		NotificationsPerAccount: true,
		KeepInactiveConnected:   true,
		LocationAccess:          true,
		NotificationSound:       true,
		NotificationText:        true,
		ShowTrayIcon:            true,
		CloseToTray:             true,
		ShowWindowControls:      true,
		FontSize:                "default",
		ShowMessagePreviews:     true,
		AutoDownload:            "photos",
	}
}

// fileName is the JSON file's name within the settings directory.
const fileName = "settings.json"

// Load reads settings.json from dir, returning Default() if the file
// doesn't exist. A malformed file also falls back to Default() rather than
// failing startup. Unknown JSON fields are ignored by encoding/json.
func Load(dir string) Settings {
	s := Default()
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Default()
	}
	return normalize(s)
}

// Save writes s to dir/settings.json as indented JSON, creating dir if
// needed.
func Save(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, fileName), data, 0o600)
}

// Dir resolves $XDG_CONFIG_HOME/chatot, falling back to ~/.config/chatot.
func Dir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "chatot")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "chatot")
	}
	return filepath.Join(home, ".config", "chatot")
}

// normalize replaces enum-valued fields that hold something the app does
// not know (an older file, a hand edit) with their defaults, so callers can
// switch on them without a fallback arm.
func normalize(s Settings) Settings {
	if !contains(FontSizes, s.FontSize) {
		s.FontSize = Default().FontSize
	}
	if !contains(AutoDownloadModes, s.AutoDownload) {
		s.AutoDownload = Default().AutoDownload
	}
	return s
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
