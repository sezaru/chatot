package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The wallpaper sheet points at the file as a URI (spaces and the like
// escaped), fills the thread with it, and lets the list show it through.
func TestChatWallpaperCSS(t *testing.T) {
	css := chatWallpaperCSS("/home/me/My Pictures/wall#1.jpg")
	for _, want := range []string{
		`url("file:///home/me/My%20Pictures/wall%231.jpg")`,
		"background-size: cover",
		".chatot-conv-list {\n\tbackground-color: transparent;",
		".chatot-bubble-in, .chatot-typing-bubble, .chatot-day-separator, .chatot-join-banner {\n\tbackground-color: mix(@chatot_thread, currentColor, 0.06);",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("chatWallpaperCSS lacks %q:\n%s", want, css)
		}
	}
}

// Choosing a picture copies it under the app's dir by content and drops the
// previous copy; choosing the same picture again keeps the one copy.
func TestInstallChatWallpaperReplacesTheOldCopy(t *testing.T) {
	src := t.TempDir()
	a := filepath.Join(src, "A.JPG")
	b := filepath.Join(src, "b.png")
	os.WriteFile(a, []byte("picture a"), 0o600)
	os.WriteFile(b, []byte("picture b"), 0o600)
	dir := filepath.Join(t.TempDir(), "wallpaper")

	first, err := installChatWallpaper(a, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first) != dir || !strings.HasPrefix(filepath.Base(first), "wallpaper-") || filepath.Ext(first) != ".jpg" {
		t.Errorf("copy = %q, want dir/wallpaper-<hash>.jpg", first)
	}
	if data, _ := os.ReadFile(first); string(data) != "picture a" {
		t.Errorf("copy holds %q", data)
	}
	again, err := installChatWallpaper(a, dir)
	if err != nil || again != first {
		t.Errorf("same picture again = %q, %v; want %q", again, err, first)
	}
	second, err := installChatWallpaper(b, dir)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("a different picture got the same name %q", second)
	}
	if fileExists(first) {
		t.Error("the earlier copy was kept")
	}
	if !fileExists(second) {
		t.Error("the new copy is missing")
	}
	clearChatWallpaper(dir, "")
	if fileExists(second) {
		t.Error("clearChatWallpaper left the copy")
	}
}

// A chat's own choice wins over the default: a picture shows, "plain"
// shows nothing, and a chat without a choice follows the default.
func TestEffectiveWallpaper(t *testing.T) {
	overrides := map[string]string{"a@s.whatsapp.net": "/w/a.jpg", "b@g.us": "none"}
	cases := []struct{ jid, def, want string }{
		{"a@s.whatsapp.net", "/w/default.jpg", "/w/a.jpg"},
		{"b@g.us", "/w/default.jpg", ""},
		{"c@s.whatsapp.net", "/w/default.jpg", "/w/default.jpg"},
		{"c@s.whatsapp.net", "", ""},
	}
	for _, c := range cases {
		if got := effectiveWallpaper(c.jid, c.def, overrides); got != c.want {
			t.Errorf("effectiveWallpaper(%q, %q) = %q, want %q", c.jid, c.def, got, c.want)
		}
	}
}

// The per-chat dialog ticks the chat's current choice, one row at a time.
func TestChatWallpaperChoicesTickTheCurrent(t *testing.T) {
	for override, want := range map[string]string{"": "Default wallpaper", "none": "Plain background", "/w/a.jpg": "Choose a picture…"} {
		var ticked []string
		for _, o := range chatWallpaperChoices(override) {
			if o.Current {
				ticked = append(ticked, o.Label)
			}
		}
		if len(ticked) != 1 || ticked[0] != want {
			t.Errorf("choices(%q) tick %v, want [%s]", override, ticked, want)
		}
	}
}

// Each chat's picture lives in its own folder, named safely from the JID.
func TestChatWallpaperChatDirIsPerChat(t *testing.T) {
	a := chatWallpaperChatDir("1112223333@s.whatsapp.net")
	b := chatWallpaperChatDir("123-456@g.us")
	if a == b || filepath.Dir(a) != filepath.Dir(b) {
		t.Errorf("dirs %q and %q should be siblings", a, b)
	}
	if strings.ContainsAny(filepath.Base(a), "@/") {
		t.Errorf("dir name %q keeps unsafe characters", filepath.Base(a))
	}
}
