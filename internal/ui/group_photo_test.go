package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestEncodeGroupPhotoFitsTheCap(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1000, 500))
	for y := 0; y < 500; y++ {
		for x := 0; x < 1000; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x / 4), G: uint8(y / 2), B: 90, A: 255})
		}
	}
	data, err := encodeGroupPhoto(src, 640)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// The longer side lands on the cap and the aspect ratio survives.
	if cfg.Width != 640 || cfg.Height != 320 {
		t.Errorf("encoded %dx%d, want 640x320", cfg.Width, cfg.Height)
	}
}

func TestShrinkToLeavesSmallImagesAlone(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 300, 200))
	if got := shrinkTo(src, 640); got != src {
		t.Error("a photo already within the cap was rescaled")
	}
	// A box average of a flat colour is that colour, not a darker one
	// (the classic off-by-one when the sum is divided by the wrong count).
	flat := image.NewRGBA(image.Rect(0, 0, 900, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 900; x++ {
			flat.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	small := shrinkTo(flat, 300)
	if b := small.Bounds(); b.Dx() != 300 || b.Dy() != 300 {
		t.Fatalf("shrunk to %v", b)
	}
	r, g, b, _ := small.At(150, 150).RGBA()
	if r>>8 != 200 || g>>8 != 100 || b>>8 != 50 {
		t.Errorf("averaged colour = %d,%d,%d, want 200,100,50", r>>8, g>>8, b>>8)
	}
}

func TestEncodeGroupPhotoFlattensTransparencyOntoWhite(t *testing.T) {
	// A fully transparent image must come out white, not the black that
	// premultiplied zeros encode as.
	src := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	data, err := encodeGroupPhoto(src, 640)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(20, 20).RGBA()
	if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Errorf("transparent pixel encoded as %d,%d,%d, want white", r>>8, g>>8, b>>8)
	}
}

func TestShrinkToHonoursANonZeroOrigin(t *testing.T) {
	// A SubImage keeps its parent's coordinates; sampling must start at
	// Bounds().Min, not at (0,0), or the crop shows the wrong region.
	parent := image.NewRGBA(image.Rect(0, 0, 1200, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1200; x++ {
			c := color.RGBA{R: 20, G: 20, B: 20, A: 255}
			if x >= 600 && y >= 600 {
				c = color.RGBA{R: 240, G: 240, B: 240, A: 255}
			}
			parent.Set(x, y, c)
		}
	}
	sub := parent.SubImage(image.Rect(600, 600, 1200, 1200))
	small := shrinkTo(sub, 300)
	r, _, _, _ := small.At(150, 150).RGBA()
	if r>>8 != 240 {
		t.Errorf("sub-image sampled from the wrong origin: red = %d, want 240", r>>8)
	}
}
