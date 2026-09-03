package geo

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
)

// Snapshot composes the tiles around (lat, lon) into a w×h JPEG: the map
// preview a sent location carries so the bubble (ours and the recipient's)
// shows streets rather than a placeholder. Tiles not yet cached are fetched
// first; any that fail stay blank. A red pin is drawn at the centre.
func Snapshot(ctx context.Context, cache *TileCache, lat, lon float64, zoom, w, h int) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{0xe9, 0xe5, 0xdd, 0xff}), image.Point{}, draw.Src)
	for _, p := range TilesFor(lat, lon, zoom, w, h) {
		data, err := cache.Fetch(ctx, p.Tile)
		if err != nil {
			continue
		}
		tile, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}
		at := image.Pt(int(math.Round(p.DX)), int(math.Round(p.DY)))
		draw.Draw(img, image.Rectangle{Min: at, Max: at.Add(image.Pt(TileSize, TileSize))}, tile, image.Point{}, draw.Over)
	}
	drawPin(img, w/2, h/2)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawPin paints the bubble's red dot with its translucent halo at (cx, cy).
func drawPin(img *image.RGBA, cx, cy int) {
	fill := func(r float64, c color.RGBA) {
		rr := int(math.Ceil(r))
		for y := -rr; y <= rr; y++ {
			for x := -rr; x <= rr; x++ {
				if float64(x*x+y*y) <= r*r {
					px, py := cx+x, cy+y
					if image.Pt(px, py).In(img.Bounds()) {
						img.Set(px, py, blend(img.RGBAAt(px, py), c))
					}
				}
			}
		}
	}
	fill(10.5, color.RGBA{0xc0, 0x1c, 0x28, 0x38})
	fill(6.5, color.RGBA{0xc0, 0x1c, 0x28, 0xff})
}

// blend composites src over dst (straight alpha).
func blend(dst, src color.RGBA) color.RGBA {
	a := float64(src.A) / 255
	mix := func(d, s uint8) uint8 { return uint8(float64(d)*(1-a) + float64(s)*a + 0.5) }
	return color.RGBA{mix(dst.R, src.R), mix(dst.G, src.G), mix(dst.B, src.B), 0xff}
}
