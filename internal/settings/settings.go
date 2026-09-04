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
}

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
	return s
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
