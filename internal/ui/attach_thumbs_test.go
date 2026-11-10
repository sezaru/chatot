package ui

import (
	"image"
	"image/color"
	"image/gif"
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

func TestFitSide(t *testing.T) {
	cases := []struct{ w, h, side, ww, wh int }{
		{1080, 1424, 512, 388, 512},
		{1424, 1080, 512, 512, 388},
		{100, 100, 40, 40, 40},
		{4000, 2, 40, 40, 1},
	}
	for _, c := range cases {
		if w, h := fitSide(c.w, c.h, c.side); w != c.ww || h != c.wh {
			t.Errorf("fitSide(%d, %d, %d) = %dx%d, want %dx%d", c.w, c.h, c.side, w, h, c.ww, c.wh)
		}
	}
}

// A GIF whose frames add up past gdk-pixbuf's animation limit cannot be
// loaded at a size; the thumbnail falls back to its first frame.
func TestImageThumbnailOfLargeGIF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.gif")
	palette := color.Palette{color.Black, color.White, color.Gray{Y: 128}}
	anim := &gif.GIF{}
	for i := range 40 {
		frame := image.NewPaletted(image.Rect(0, 0, 1080, 1424), palette)
		frame.Pix[i] = 1
		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, 4)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, anim); err != nil {
		t.Fatal(err)
	}
	f.Close()

	data, err := imageThumbnail(path)
	if err != nil {
		t.Fatalf("imageThumbnail: %v", err)
	}
	pb, err := pixbufFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if pb.Width() != 388 || pb.Height() != 512 {
		t.Errorf("thumbnail is %dx%d, want 388x512", pb.Width(), pb.Height())
	}
}
