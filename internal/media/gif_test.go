package media

import (
	"context"
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsGIF(t *testing.T) {
	cases := []struct {
		path, mime string
		want       bool
	}{
		{"loop.gif", "", true},
		{"LOOP.GIF", "", true},
		{"blob", "image/gif", true},
		{"loop.gif", "video/mp4", false},
		{"photo.jpg", "", false},
		{"blob", "image/png", false},
	}
	for _, c := range cases {
		if got := IsGIF(c.path, c.mime); got != c.want {
			t.Errorf("IsGIF(%q, %q) = %v, want %v", c.path, c.mime, got, c.want)
		}
	}
}

func TestGIFVideo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "loop.gif")
	// Odd sides: the encoder needs them rounded to even.
	anim := &gif.GIF{}
	palette := color.Palette{color.Black, color.White}
	for i := range 4 {
		frame := image.NewPaletted(image.Rect(0, 0, 33, 21), palette)
		frame.Set(i, i, color.White)
		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, 10)
	}
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, anim); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cache := filepath.Join(dir, "cache")
	out, err := GIFVideo(context.Background(), cache, src)
	if err != nil {
		t.Fatalf("GIFVideo: %v", err)
	}
	info, err := ProbeVideo(context.Background(), out)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if info.Width != 32 || info.Height != 20 {
		t.Fatalf("mp4 is %dx%d, want 32x20", info.Width, info.Height)
	}
	again, err := GIFVideo(context.Background(), cache, src)
	if err != nil || again != out {
		t.Fatalf("second call = %q, %v; want the cached %q", again, err, out)
	}
}
