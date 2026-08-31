package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got := Load(t.TempDir())
	if want := Default(); got != want {
		t.Errorf("Load(missing) = %+v, want defaults %+v", got, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Settings{
		SendReadReceipts:     true,
		SendTypingIndicators: false,
		ShowNotifications:    false,
		Theme:                "dark",
		Proxy:                "socks5://localhost:9050",
	}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load(dir)
	if got != want {
		t.Errorf("Load after Save = %+v, want %+v", got, want)
	}
}

func TestLoadUnknownFieldsTolerated(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"sendReadReceipts": true, "futureFeature": {"nested": 1}}`)
	if err := os.WriteFile(filepath.Join(dir, fileName), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := Load(dir)
	want := Default()
	want.SendReadReceipts = true
	if got != want {
		t.Errorf("Load(unknown fields) = %+v, want %+v", got, want)
	}
}

func TestLoadMalformedFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := Load(dir)
	if want := Default(); got != want {
		t.Errorf("Load(malformed) = %+v, want defaults %+v", got, want)
	}
}
