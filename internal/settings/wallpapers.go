package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PlainWallpaper is the per-chat wallpaper value that puts the plain
// surface behind a chat even while a default wallpaper is set.
const PlainWallpaper = "none"

// wallpapersFileName holds the per-chat wallpaper overrides, beside
// settings.json: chat JID to the path of the app's copy of the picture, or
// PlainWallpaper. A chat not in the file uses the default wallpaper.
const wallpapersFileName = "chat-wallpapers.json"

// LoadChatWallpapers reads the per-chat overrides from dir; a missing or
// malformed file is an empty map.
func LoadChatWallpapers(dir string) map[string]string {
	m := map[string]string{}
	data, err := os.ReadFile(filepath.Join(dir, wallpapersFileName))
	if err != nil {
		return m
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]string{}
	}
	return m
}

// SaveChatWallpapers writes the per-chat overrides to dir, creating dir if
// needed; an empty map removes the file.
func SaveChatWallpapers(dir string, m map[string]string) error {
	path := filepath.Join(dir, wallpapersFileName)
	if len(m) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
