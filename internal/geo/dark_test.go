package geo

import (
	"image/color"
	"testing"
)

func TestDarkPixelKeepsHueInvertsLightness(t *testing.T) {
	// A light blue (water) must come out a dark blue, not yellow.
	got := darkPixel(color.RGBA{0xaa, 0xd3, 0xdf, 0xff})
	if !(got.B > got.R && got.B > got.G) {
		t.Errorf("light blue -> %+v, want a blue", got)
	}
	if int(got.R)+int(got.G)+int(got.B) > 3*0x70 {
		t.Errorf("light blue -> %+v, want dark", got)
	}
	// White ground becomes near-black, black text near-white.
	if w := darkPixel(color.RGBA{0xff, 0xff, 0xff, 0xff}); w.R > 0x10 {
		t.Errorf("white -> %+v", w)
	}
	if k := darkPixel(color.RGBA{0, 0, 0, 0xff}); k.R < 0xe0 {
		t.Errorf("black -> %+v", k)
	}
}

func TestDarkTileRoundTrip(t *testing.T) {
	out, err := DarkTile(solidTilePNG())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 || out[1] != 'P' {
		t.Errorf("DarkTile did not return a PNG")
	}
	if _, err := DarkTile([]byte("nope")); err == nil {
		t.Error("expected an error for junk input")
	}
}
