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

func TestNotificationSoundSourceOrder(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "mine.mp3")
	packaged := filepath.Join(dir, "brand.oga")
	cfg := filepath.Join(dir, "cfg")
	os.MkdirAll(cfg, 0o700)
	dropIn := filepath.Join(cfg, "notify.ogg")
	for _, p := range []string{custom, packaged, dropIn} {
		os.WriteFile(p, []byte("OggS"), 0o600)
	}

	if p, s := notificationSoundSource(custom, cfg, packaged); p != custom || s != soundSourceCustom {
		t.Errorf("all present: %q %q, want the custom file", p, s)
	}
	if p, s := notificationSoundSource("", cfg, packaged); p != dropIn || s != soundSourceDropIn {
		t.Errorf("no custom: %q %q, want the drop-in", p, s)
	}
	if p, s := notificationSoundSource("", t.TempDir(), packaged); p != packaged || s != soundSourcePackage {
		t.Errorf("no drop-in: %q %q, want the packaged file", p, s)
	}
	if p, s := notificationSoundSource("", t.TempDir(), ""); p != "" || s != soundSourceBuiltIn {
		t.Errorf("nothing configured: %q %q, want the built-in chime", p, s)
	}
	// A pick that no longer exists falls through instead of failing.
	if p, s := notificationSoundSource(filepath.Join(dir, "gone.wav"), t.TempDir(), packaged); p != packaged || s != soundSourcePackage {
		t.Errorf("vanished custom: %q %q, want the packaged file", p, s)
	}
}
