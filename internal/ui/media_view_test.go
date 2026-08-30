package ui

import (
	"os"
	"testing"

	"chatot/internal/client"
)

func TestMediaVM_NotMedia(t *testing.T) {
	out := mediaVM(client.Message{ID: "1", Text: "plain"})
	if out.IsMedia {
		t.Error("expected IsMedia=false for a text message")
	}
}

func TestMediaVM_NoLocalPath(t *testing.T) {
	m := client.Message{
		ID:         "1",
		Attachment: &client.Attachment{Kind: "image", Caption: "a sunset"},
	}
	out := mediaVM(m)
	if !out.IsMedia {
		t.Fatal("expected IsMedia=true")
	}
	if out.HasLocal {
		t.Error("expected HasLocal=false when local_path is empty")
	}
	if out.Chip != "[image] a sunset" {
		t.Errorf("Chip = %q, want %q", out.Chip, "[image] a sunset")
	}
}

func TestMediaVM_LocalPathMissingFile(t *testing.T) {
	m := client.Message{
		ID:         "1",
		Attachment: &client.Attachment{Kind: "image", LocalPath: "/nonexistent/path/does-not-exist.jpg"},
	}
	out := mediaVM(m)
	if out.HasLocal {
		t.Error("expected HasLocal=false when local_path points at a missing file")
	}
}

func TestMediaVM_LocalPathExists(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "chatot-media-*.jpg")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	m := client.Message{
		ID:         "1",
		Attachment: &client.Attachment{Kind: "image", LocalPath: f.Name()},
	}
	out := mediaVM(m)
	if !out.HasLocal {
		t.Fatal("expected HasLocal=true when local_path exists on disk")
	}
	if out.LocalPath != f.Name() {
		t.Errorf("LocalPath = %q, want %q", out.LocalPath, f.Name())
	}
}

func TestMediaVM_ThumbnailNotDownloaded(t *testing.T) {
	m := client.Message{
		ID:         "1",
		Attachment: &client.Attachment{Kind: "image", Thumbnail: []byte{0xFF, 0xD8, 0xFF}},
	}
	out := mediaVM(m)
	if !out.HasThumbnail {
		t.Error("expected HasThumbnail=true when Thumbnail is set and not downloaded")
	}
	if out.HasLocal {
		t.Error("expected HasLocal=false with no local_path")
	}
	if string(out.Thumbnail) != string(m.Attachment.Thumbnail) {
		t.Errorf("Thumbnail = %v, want %v", out.Thumbnail, m.Attachment.Thumbnail)
	}
}

func TestMediaVM_ThumbnailButDownloaded(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "chatot-media-*.jpg")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	m := client.Message{
		ID: "1",
		Attachment: &client.Attachment{
			Kind: "image", LocalPath: f.Name(), Thumbnail: []byte{0xFF, 0xD8, 0xFF},
		},
	}
	out := mediaVM(m)
	if !out.HasLocal {
		t.Fatal("expected HasLocal=true when local_path exists on disk")
	}
	if out.HasThumbnail {
		t.Error("expected HasThumbnail=false once the full media is downloaded")
	}
}

func TestMediaVM_NoThumbnail(t *testing.T) {
	m := client.Message{
		ID:         "1",
		Attachment: &client.Attachment{Kind: "image"},
	}
	out := mediaVM(m)
	if out.HasThumbnail {
		t.Error("expected HasThumbnail=false when no thumbnail bytes are set")
	}
}

func TestInlineable(t *testing.T) {
	for _, kind := range []string{"image", "video", "sticker", "audio"} {
		if !inlineable(kind) {
			t.Errorf("inlineable(%q) = false, want true", kind)
		}
	}
	for _, kind := range []string{"document", ""} {
		if inlineable(kind) {
			t.Errorf("inlineable(%q) = true, want false", kind)
		}
	}
}
