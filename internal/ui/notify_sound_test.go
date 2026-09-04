package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserSoundPathFindsConfigFile(t *testing.T) {
	dir := t.TempDir()
	if got := userSoundPath(dir); got != "" {
		t.Errorf("empty dir: userSoundPath = %q, want empty", got)
	}
	want := filepath.Join(dir, "notify.ogg")
	if err := os.WriteFile(want, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := userSoundPath(dir); got != want {
		t.Errorf("userSoundPath = %q, want %q", got, want)
	}
}

func TestNotifyChimeEmbedded(t *testing.T) {
	if len(notifyChime) < 1000 || string(notifyChime[:4]) != "OggS" {
		t.Fatalf("embedded chime is not an Ogg stream (%d bytes)", len(notifyChime))
	}
}
