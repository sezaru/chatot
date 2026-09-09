package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/settings"
)

// wallpaperProvider is the sheet painting the chat wallpaper; nil while the
// thread shows its plain surface. wallpaperApplied is the path it shows.
var (
	wallpaperProvider *gtk.CSSProvider
	wallpaperApplied  string
)

// ChatWallpaper is the default wallpaper's path ("" for the plain
// surface), mirrored from settings by main.go and Preferences.
var ChatWallpaper string

// chatWallpaperOverrides is each chat's own choice, when it made one: the
// path of the app's copy of its picture, or settings.PlainWallpaper.
var chatWallpaperOverrides = map[string]string{}

// wallpaperChat is the chat the thread shows, so a change to the default
// or to this chat's own wallpaper repaints at once.
var wallpaperChat string

// SetChatWallpaperOverrides installs the per-chat choices loaded at start.
func SetChatWallpaperOverrides(m map[string]string) {
	if m == nil {
		m = map[string]string{}
	}
	chatWallpaperOverrides = m
}

// effectiveWallpaper is the picture behind jid: its own choice when it has
// one (plain counts), else the default.
func effectiveWallpaper(jid, def string, overrides map[string]string) string {
	switch v, ok := overrides[jid]; {
	case !ok:
		return def
	case v == settings.PlainWallpaper:
		return ""
	default:
		return v
	}
}

// setWallpaperChat records the open chat and paints its wallpaper.
func setWallpaperChat(jid string) {
	wallpaperChat = jid
	refreshChatWallpaper()
}

// refreshChatWallpaper repaints the open chat's wallpaper after the
// default or its own choice changed.
func refreshChatWallpaper() {
	ApplyChatWallpaper(effectiveWallpaper(wallpaperChat, ChatWallpaper, chatWallpaperOverrides))
}

// ApplyChatWallpaper paints the picture at path behind an open chat's
// messages (as WhatsApp Web's custom wallpaper does: filling the thread,
// centred, fixed while the messages scroll), or restores the plain surface
// for "". A file that is gone counts as "". The same path again is a no-op,
// so a chat reload does not reparse the sheet.
func ApplyChatWallpaper(path string) {
	if path != "" && !fileExists(path) {
		log.Printf("chatot: chat wallpaper %s is missing; showing the plain surface", path)
		path = ""
	}
	if path == wallpaperApplied && (path == "" || wallpaperProvider != nil) {
		return
	}
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}
	wallpaperApplied = path
	if wallpaperProvider != nil {
		gtk.StyleContextRemoveProviderForDisplay(display, wallpaperProvider)
		wallpaperProvider = nil
	}
	if path == "" {
		return
	}
	p := gtk.NewCSSProvider()
	p.LoadFromString(chatWallpaperCSS(path))
	// Above the app sheet and its dark override (one step up), so the
	// thread's plain fill gives way in either scheme.
	gtk.StyleContextAddProviderForDisplay(display, p, widgetPriority(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION+2))
	wallpaperProvider = p
}

// chatWallpaperCSS is the sheet that shows the picture at path through the
// thread: the scroller carries it, the list over it goes clear, and the
// washes that read as tints on the plain surface (an incoming bubble, the
// day and unread pills) become the same colours mixed opaque, so text stays
// legible whatever is behind it.
func chatWallpaperCSS(path string) string {
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	return ".chatot-conv-thread {\n" +
		"\tbackground-image: url(\"" + uri + "\");\n" +
		"\tbackground-size: cover;\n" +
		"\tbackground-position: center;\n" +
		"\tbackground-repeat: no-repeat;\n" +
		"}\n" +
		".chatot-conv-list {\n" +
		"\tbackground-color: transparent;\n" +
		"}\n" +
		".chatot-bubble-in, .chatot-typing-bubble, .chatot-day-separator {\n" +
		"\tbackground-color: mix(@chatot_thread, currentColor, 0.06);\n" +
		"}\n" +
		".chatot-day-separator {\n" +
		"\topacity: 0.9;\n" +
		"}\n" +
		".chatot-unread-separator {\n" +
		"\tbackground-color: mix(@chatot_thread, #1b8c72, 0.12);\n" +
		"}\n"
}

// chatWallpaperDir is where the app keeps its copy of the wallpaper.
func chatWallpaperDir() string { return filepath.Join(settings.Dir(), "wallpaper") }

// installChatWallpaper copies the picture at src into dir, named by its
// content, and drops any earlier wallpaper there. The copy is what the
// setting points at: a file chosen through the portal is only readable
// for this session, and one in the user's folders may move.
func installChatWallpaper(src, dir string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	ext := strings.ToLower(filepath.Ext(src))
	dest := filepath.Join(dir, "wallpaper-"+hex.EncodeToString(sum[:4])+ext)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", err
	}
	clearChatWallpaper(dir, dest)
	return dest, nil
}

// clearChatWallpaper removes every wallpaper copy under dir except keep
// ("" keeps none).
func clearChatWallpaper(dir, keep string) {
	matches, _ := filepath.Glob(filepath.Join(dir, "wallpaper-*"))
	for _, m := range matches {
		if m != keep {
			os.Remove(m)
		}
	}
}

// Per-chat wallpaper: the header ⋮ menu's "Chat wallpaper…" offers the
// default, the plain surface or a picture of the chat's own, kept under
// its own folder so replacing it drops the previous copy.

// Choice codes for chatWallpaperChoices, carried in choiceOption.Seconds.
const (
	wallpaperChoiceDefault int64 = iota
	wallpaperChoicePlain
	wallpaperChoicePicture
)

// chatWallpaperChoices are the per-chat dialog's rows, the chat's current
// choice ticked: override is its entry ("" when it has none).
func chatWallpaperChoices(override string) []choiceOption {
	return []choiceOption{
		{Label: "Default wallpaper", Seconds: wallpaperChoiceDefault, Current: override == ""},
		{Label: "Plain background", Seconds: wallpaperChoicePlain, Current: override == settings.PlainWallpaper},
		{Label: "Choose a picture…", Seconds: wallpaperChoicePicture, Current: override != "" && override != settings.PlainWallpaper},
	}
}

// chatWallpaperChatDir is where jid's own picture is kept.
func chatWallpaperChatDir(jid string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			return r
		}
		return '_'
	}, jid)
	return filepath.Join(chatWallpaperDir(), "chats", safe)
}

// showChatWallpaperDialog lets the user pick what sits behind jid's
// messages; toasts hears the outcome.
func showChatWallpaperDialog(parent *gtk.Window, jid string, toasts *adw.ToastOverlay) {
	showChoiceDialog(parent, "Chat wallpaper",
		"What sits behind this chat's messages. The default is set in Preferences › Appearance.",
		chatWallpaperChoices(chatWallpaperOverrides[jid]),
		func(opt choiceOption) {
			switch opt.Seconds {
			case wallpaperChoiceDefault:
				setChatWallpaperOverride(jid, "")
				showToast(toasts, "This chat uses the default wallpaper")
			case wallpaperChoicePlain:
				setChatWallpaperOverride(jid, settings.PlainWallpaper)
				showToast(toasts, "This chat shows the plain background")
			case wallpaperChoicePicture:
				pickImageFile(parent, func(src string) {
					if _, err := gdk.NewTextureFromFilename(src); err != nil {
						showToast(toasts, "Couldn't read that picture")
						return
					}
					dest, err := installChatWallpaper(src, chatWallpaperChatDir(jid))
					if err != nil {
						showToast(toasts, "Couldn't keep that picture: "+err.Error())
						return
					}
					setChatWallpaperOverride(jid, dest)
					showToast(toasts, "Wallpaper set for this chat")
				})
			}
		})
}

// setChatWallpaperOverride records jid's choice ("" for the default),
// saves the overrides, drops a picture no longer used and repaints.
func setChatWallpaperOverride(jid, value string) {
	if value == "" {
		delete(chatWallpaperOverrides, jid)
	} else {
		chatWallpaperOverrides[jid] = value
	}
	if value == "" || value == settings.PlainWallpaper {
		os.RemoveAll(chatWallpaperChatDir(jid))
	}
	if err := settings.SaveChatWallpapers(settings.Dir(), chatWallpaperOverrides); err != nil {
		log.Printf("chatot: save chat wallpapers: %v", err)
	}
	refreshChatWallpaper()
}
