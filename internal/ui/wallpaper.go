package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/settings"
)

// wallpaperProvider is the sheet painting the chat wallpaper; nil while the
// thread shows its plain surface.
var wallpaperProvider *gtk.CSSProvider

// ApplyChatWallpaper paints the picture at path behind an open chat's
// messages (as WhatsApp Web's custom wallpaper does: filling the thread,
// centred, fixed while the messages scroll), or restores the plain surface
// for "". A file that is gone counts as "".
func ApplyChatWallpaper(path string) {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}
	if wallpaperProvider != nil {
		gtk.StyleContextRemoveProviderForDisplay(display, wallpaperProvider)
		wallpaperProvider = nil
	}
	if path == "" {
		return
	}
	if !fileExists(path) {
		log.Printf("chatot: chat wallpaper %s is missing; showing the plain surface", path)
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
