package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// groupPhotoMaxSide caps a group picture's longer side; WhatsApp wants a
// modest square-ish JPEG, not the camera original.
const groupPhotoMaxSide = 640

const groupPhotoQuality = 88

// pickGroupPhoto opens an image chooser and hands back the pick as a JPEG
// (WhatsApp accepts nothing else for a group picture), scaled to at most
// groupPhotoMaxSide on a side.
func pickGroupPhoto(parent *gtk.Window, onPicked func(jpeg []byte)) {
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Choose a group photo")
	filter := gtk.NewFileFilter()
	filter.SetName("Images")
	filter.AddMIMEType("image/*")
	filters := gio.NewListStore(glib.TypeObject)
	filters.Append(filter.Object)
	dialog.SetFilters(filters)
	dialog.Open(context.Background(), parent, func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil || file == nil {
			return
		}
		jpeg, err := jpegForUpload(file.Path())
		if err != nil {
			log.Printf("chatot: group photo %q: %v", file.Path(), err)
			return
		}
		onPicked(jpeg)
	})
}

// jpegForUpload decodes path and re-encodes it as an upload-sized JPEG.
//
// GDK's own loaders rather than gdk-pixbuf: GTK 4 decodes PNG and JPEG
// itself, whereas the pixbuf loader set a packaged GTK resolves can be as
// thin as librsvg alone (this development environment's is), which turned
// every picked photo into "unrecognized image file format" and left the
// disc unchanged.
func jpegForUpload(path string) ([]byte, error) {
	texture, err := gdk.NewTextureFromFilename(path)
	if err != nil {
		return nil, err
	}
	var raw []byte
	texture.SaveToPNGBytes().Use(func(b []byte) { raw = append([]byte(nil), b...) })
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return encodeGroupPhoto(img, groupPhotoMaxSide)
}

// encodeGroupPhoto re-encodes img as a JPEG no larger than maxSide on a
// side. Transparency is flattened onto white first: JPEG has no alpha, and
// encoding premultiplied pixels as if opaque darkens every soft edge.
func encodeGroupPhoto(img image.Image, maxSide int) ([]byte, error) {
	img = shrinkTo(img, maxSide)
	flat := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: groupPhotoQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// shrinkTo box-averages img down so its longer side is at most maxSide; an
// image already within the cap is returned as is.
func shrinkTo(img image.Image, maxSide int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxSide && h <= maxSide {
		return img
	}
	scale := float64(maxSide) / float64(max(w, h))
	nw := max(1, int(float64(w)*scale+0.5))
	nh := max(1, int(float64(h)*scale+0.5))
	out := image.NewRGBA64(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy0, sy1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < nw; x++ {
			sx0, sx1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := img.At(sx, sy).RGBA()
					r += uint64(cr)
					g += uint64(cg)
					bl += uint64(cb)
					a += uint64(ca)
					n++
				}
			}
			out.SetRGBA64(x, y, color.RGBA64{R: uint16(r / n), G: uint16(g / n), B: uint16(bl / n), A: uint16(a / n)})
		}
	}
	return out
}

// groupPhotoPreview shows a picked photo in the step's 72px disc.
func groupPhotoPreview(jpeg []byte) gtk.Widgetter {
	avatar := adw.NewAvatar(72, "", false)
	if texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(jpeg)); err == nil {
		avatar.SetCustomImage(texture)
	}
	return avatar
}
