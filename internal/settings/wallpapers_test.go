package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestChatWallpapersRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := LoadChatWallpapers(dir); len(got) != 0 {
		t.Errorf("LoadChatWallpapers(missing) = %v, want empty", got)
	}
	want := map[string]string{
		"1112223333@s.whatsapp.net": "/home/me/.config/chatot/wallpaper/chats/x/wallpaper-0badc0de.jpg",
		"123-456@g.us":              PlainWallpaper,
	}
	if err := SaveChatWallpapers(dir, want); err != nil {
		t.Fatal(err)
	}
	if got := LoadChatWallpapers(dir); !reflect.DeepEqual(got, want) {
		t.Errorf("Load after Save = %v, want %v", got, want)
	}
	// An empty map removes the file rather than leaving "{}" behind.
	if err := SaveChatWallpapers(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, wallpapersFileName)); !os.IsNotExist(err) {
		t.Errorf("file left after saving no overrides: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, wallpapersFileName), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadChatWallpapers(dir); len(got) != 0 {
		t.Errorf("LoadChatWallpapers(malformed) = %v, want empty", got)
	}
}
