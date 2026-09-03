package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImageThumbnailEncodesJPEGFromSVG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mark.svg")
	if err := os.WriteFile(path, appMarkSVG, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := imageThumbnail(path)
	if err != nil {
		t.Skipf("gdk-pixbuf could not load the SVG here: %v", err)
	}
	if len(data) < 100 || data[0] != 0xff || data[1] != 0xd8 {
		t.Errorf("imageThumbnail did not return a JPEG (%d bytes)", len(data))
	}
	if _, err := imageThumbnail(filepath.Join(t.TempDir(), "missing.png")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestTrayKindOf(t *testing.T) {
	cases := map[string]trayKind{
		"a.JPG": trayImage, "b.svg": trayImage, "c.mp4": trayVideo, "d.mov": trayVideo,
		"e.mp3": trayAudio, "f.ogg": trayAudio, "g.pdf": trayPDF, "h.zip": trayOther, "noext": trayOther,
	}
	for path, want := range cases {
		if got := trayKindOf(path); got != want {
			t.Errorf("trayKindOf(%q) = %v, want %v", path, got, want)
		}
	}
}
