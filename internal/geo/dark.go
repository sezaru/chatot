package geo

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// DarkTile re-renders a light OSM tile for the dark scheme the way the
// mockup's CSS filter does (invert, rotate hue back, slightly desaturate,
// slightly darken): every pixel keeps its hue and saturation while its
// lightness is inverted, so water stays blue and parks stay green on a
// dark ground. Returns PNG bytes.
func DarkTile(pngData []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			out.Set(x, y, darkPixel(color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)}))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// darkPixel is DarkTile for one pixel: HSL with L → 1-L, S × 0.86, then
// brightness × 0.94.
func darkPixel(c color.RGBA) color.RGBA {
	h, s, l := rgbToHSL(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	r, g, b := hslToRGB(h, s*0.86, 1-l)
	const bright = 0.94
	return color.RGBA{uint8(r*bright*255 + 0.5), uint8(g*bright*255 + 0.5), uint8(b*bright*255 + 0.5), c.A}
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return hueToRGB(p, q, h+1.0/3), hueToRGB(p, q, h), hueToRGB(p, q, h-1.0/3)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 0.5:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}
