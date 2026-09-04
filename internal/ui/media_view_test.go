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
	if out.Chip != "📷 a sunset" {
		t.Errorf("Chip = %q, want %q", out.Chip, "📷 a sunset")
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

func TestMediaVM_IsGIF(t *testing.T) {
	m := client.Message{
		ID:         "1",
		Attachment: &client.Attachment{Kind: "video", IsGIF: true},
	}
	out := mediaVM(m)
	if !out.IsGIF {
		t.Error("expected IsGIF=true when the attachment is a GIF-playback video")
	}
}

func TestMediaVM_ViewOnce(t *testing.T) {
	m := client.Message{
		ID:         "1",
		Attachment: &client.Attachment{Kind: "image", ViewOnce: true},
	}
	out := mediaVM(m)
	if !out.ViewOnce || out.Viewed {
		t.Errorf("got ViewOnce=%v Viewed=%v, want ViewOnce=true Viewed=false", out.ViewOnce, out.Viewed)
	}
}

func TestViewOnceRenderState(t *testing.T) {
	cases := []struct {
		name               string
		isViewOnce, viewed bool
		wantTitle, wantSub string
		wantSpent          bool
	}{
		{"not view-once", false, false, "", "", false},
		{"unopened", true, false, "view once", "Click to open · closes after viewing", false},
		{"opened", true, true, "opened", "No longer available", true},
		// A viewed=true attachment that somehow isn't view-once shouldn't happen
		// in practice, but the selector should still treat isViewOnce as the gate.
		{"viewed but not view-once", false, true, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, sub, spent := viewOnceRenderState(tc.isViewOnce, tc.viewed)
			if title != tc.wantTitle || sub != tc.wantSub || spent != tc.wantSpent {
				t.Errorf("viewOnceRenderState(%v, %v) = (%q, %q, %v), want (%q, %q, %v)",
					tc.isViewOnce, tc.viewed, title, sub, spent, tc.wantTitle, tc.wantSub, tc.wantSpent)
			}
		})
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

func TestInlinePhotoHeightFollowsAspect(t *testing.T) {
	cases := []struct{ w, h, want int }{
		{1400, 400, 80}, // banner: 280*400/1400 = 80
		{1400, 300, 80}, // wider still: clamped to the floor
		{800, 600, 210}, // 4:3 lands at 210
		{600, 800, 360}, // portrait: 373 clamped to the ceiling
		{0, 0, 200},     // unknown: the old fixed box
	}
	for _, c := range cases {
		if got := inlinePhotoHeight(c.w, c.h); got != c.want {
			t.Errorf("inlinePhotoHeight(%d,%d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestNextZoomStepSnapsToDesignSteps(t *testing.T) {
	cases := []struct {
		z    float64
		d    int
		want float64
	}{
		{1, 1, 1.5},    // Fit → first step up
		{1.5, 1, 2},    // on a step → next step
		{1.7, 1, 2},    // between steps → the step above
		{1.7, -1, 1.5}, // between steps → the step below
		{5, 1, 5},      // at the ceiling stays
		{1, -1, 1},     // at Fit stays
		{1.004, -1, 1}, // rounding noise counts as Fit
		{2.004, 1, 3},  // rounding noise counts as the step
	}
	for _, c := range cases {
		if got := nextZoomStep(c.z, c.d); got != c.want {
			t.Errorf("nextZoomStep(%v,%d) = %v, want %v", c.z, c.d, got, c.want)
		}
	}
}
