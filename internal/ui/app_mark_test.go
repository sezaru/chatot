package ui

import (
	"bytes"
	"image/png"
	"testing"
)

// The app draws the rasterised mark (the SVG needs a pixbuf loader that
// is not everywhere); it must be a real 512px PNG so it never falls back
// to the placeholder glyph.
func TestAppMarkPNGDecodes(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(appMarkPNG))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 512 || b.Dy() != 512 {
		t.Fatalf("size = %dx%d, want 512x512", b.Dx(), b.Dy())
	}
	if !bytes.Contains(appMarkSVG, []byte("#e8a34a")) {
		t.Fatal("embedded SVG is not the 2c mark (no amber beak)")
	}
}

// Both tray marks must be square PNGs: StatusNotifierItem pixmaps are
// scaled to a square slot, and the menu row shows them at 11px.
func TestTrayIconPNGsAreSquare(t *testing.T) {
	for name, data := range map[string][]byte{"light": trayIconLightPNG, "dark": trayIconDarkPNG} {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if b := img.Bounds(); b.Dx() != 128 || b.Dy() != 128 {
			t.Fatalf("%s: size = %dx%d, want 128x128", name, b.Dx(), b.Dy())
		}
	}
	if trayIconPNG(true) == nil || &trayIconPNG(true)[0] != &trayIconDarkPNG[0] || &trayIconPNG(false)[0] != &trayIconLightPNG[0] {
		t.Fatal("trayIconPNG picks the wrong variant")
	}
}
