package ui

import (
	"log"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// pictureTextures memoises decoded files by path and side. Bubbles are
// rebuilt whenever the list recycles a row, and the same photo must not be
// decoded again each time. Main loop only; LRU past pictureTexturesBudget
// bytes, enough for every avatar of a large account plus recent media.
var pictureTextures = newTextureCache(pictureTexturesBudget)

const pictureTexturesBudget = 64 << 20

// pictureLoads holds the setters waiting on a decode in flight, so a file
// bound to several widgets at once is decoded once. Main loop only.
var pictureLoads = map[string][]func(gdk.Paintabler){}

// newAsyncPicture is a picture of the file at path, decoded off the main
// loop and scaled to fit side px. GtkPicture's own file loading decodes the
// whole image on the spot, on the main loop.
func newAsyncPicture(path string, side int) *gtk.Picture {
	p := gtk.NewPicture()
	loadPictureAsync(path, side, p.SetPaintable)
	return p
}

// loadPictureAsync hands set the texture for path (scaled to fit side px)
// on the main loop, at once when it is memoised, else after decoding it in
// the background. Nothing happens when the file cannot be decoded.
func loadPictureAsync(path string, side int, set func(gdk.Paintabler)) {
	key := path + "|" + strconv.Itoa(side)
	if t, ok := pictureTextures.get(key); ok {
		set(t)
		return
	}
	if waiting, ok := pictureLoads[key]; ok {
		pictureLoads[key] = append(waiting, set)
		return
	}
	pictureLoads[key] = []func(gdk.Paintabler){set}
	go func() {
		pb, err := gdkpixbufAtScale(path, side)
		glib.IdleAdd(func() {
			waiting := pictureLoads[key]
			delete(pictureLoads, key)
			if err != nil {
				log.Printf("chatot: decode %s: %v", path, err)
				return
			}
			t := gdk.NewTextureForPixbuf(pb)
			pictureTextures.put(key, t, textureBytes(t))
			for _, f := range waiting {
				f(t)
			}
		})
	}()
}
