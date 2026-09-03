package geo

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// solidTilePNG encodes a TileSize square filled with one colour.
func solidTilePNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, TileSize, TileSize))
	for y := 0; y < TileSize; y++ {
		for x := 0; x < TileSize; x++ {
			img.Set(x, y, color.RGBA{0x40, 0x80, 0xc0, 0xff})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
