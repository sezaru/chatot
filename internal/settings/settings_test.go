package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got := Load(t.TempDir())
	if want := Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("Load(missing) = %+v, want defaults %+v", got, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Settings{
		SendReadReceipts:        true,
		SendTypingIndicators:    false,
		ShowNotifications:       false,
		Theme:                   "dark",
		Proxy:                   "socks5://localhost:9050",
		NotificationsPerAccount: false,
		KeepInactiveConnected:   false,
		ThemeSource:             "auto",
		VoiceSpeed:              1.5,
		FontSize:                "default",
		AutoDownload:            "photos",
		AutoTranscribe:          true,
		TranscriptsExpanded:     true,
		TranscribeModel:         "turbo",
		ChatWallpaper:           "/home/me/.config/chatot/wallpaper/wallpaper-0badc0de.jpg",
	}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load(dir)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load after Save = %+v, want %+v", got, want)
	}
}

func TestDefaultMultiAccountTogglesOn(t *testing.T) {
	d := Default()
	if !d.NotificationsPerAccount {
		t.Error("NotificationsPerAccount should default true")
	}
	if !d.KeepInactiveConnected {
		t.Error("KeepInactiveConnected should default true")
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
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load(unknown fields) = %+v, want %+v", got, want)
	}
}

func TestLoadMalformedFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := Load(dir)
	if want := Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("Load(malformed) = %+v, want defaults %+v", got, want)
	}
}

func TestLoadKeepsNotificationSoundWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"showNotifications":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := Load(dir); !s.NotificationSound {
		t.Fatal("settings written before the sound toggle existed should keep it on")
	}
}

func TestLoadNormalizesUnknownEnumValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"fontSize":"huge","autoDownload":"wifi","transcribeModel":"base"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(dir)
	if s.FontSize != "default" || s.AutoDownload != "photos" || s.TranscribeModel != "small" {
		t.Errorf("Load = fontSize %q autoDownload %q transcribeModel %q, want defaults", s.FontSize, s.AutoDownload, s.TranscribeModel)
	}
	// A file from before there was a model choice keeps the model it
	// already downloaded.
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"autoTranscribe":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := Load(dir); s.TranscribeModel != "small" {
		t.Errorf("Load without transcribeModel = %q, want small", s.TranscribeModel)
	}
}

func TestLoadKeepsTrayAndPreviewDefaultsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(dir)
	if !s.ShowTrayIcon || !s.CloseToTray || !s.ShowWindowControls || !s.ShowMessagePreviews || !s.NotificationText {
		t.Errorf("absent fields lost their on defaults: %+v", s)
	}
}

func TestSoundFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Default()
	s.NotificationSoundFile = "/tmp/ding.mp3"
	s.VerboseLogging = true
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir); got.NotificationSoundFile != s.NotificationSoundFile || !got.VerboseLogging {
		t.Errorf("Load = %+v, want %+v", got, s)
	}
}

func TestLoadNormalizesThemeSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"themeSource":"bogus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir).ThemeSource; got != "auto" {
		t.Errorf("unknown themeSource loaded as %q, want auto", got)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"themeSource":"none"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir).ThemeSource; got != "none" {
		t.Errorf("themeSource none loaded as %q", got)
	}
}

func TestLoadNormalizesVoiceSpeed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"voiceSpeed": 3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir).VoiceSpeed; got != 1 {
		t.Fatalf("VoiceSpeed = %v, want 1 for an unknown value", got)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir).VoiceSpeed; got != 1 {
		t.Fatalf("VoiceSpeed = %v, want 1 for an older file", got)
	}
	for _, c := range []struct{ in, want float64 }{{1, 1.5}, {1.5, 2}, {2, 1}, {7, 1}} {
		if got := NextVoiceSpeed(c.in); got != c.want {
			t.Errorf("NextVoiceSpeed(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
