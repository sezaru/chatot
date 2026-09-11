package ui

import (
	"context"
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
	"chatot/internal/media"
)

// buildAlbumContent is the album bubble's grid: a tile over each of the
// run's first pictures, the last carrying "+N" for the rest. A tile shows
// its picture once the file is cached, else the sender's preview under a
// download disc. A click opens the viewer on that picture (every picture
// of the run is a step of it) or fetches it; a right-click offers the
// message menu for that one picture.
func buildAlbumContent(alb *albumView, h bubbleHooks) gtk.Widgetter {
	grid := gtk.NewBox(gtk.OrientationVertical, albumTileGap)
	grid.AddCSSClass("chatot-album")
	grid.SetOverflow(gtk.OverflowHidden)
	grid.SetHAlign(gtk.AlignStart)
	i := 0
	for _, row := range albumRows(len(alb.Tiles)) {
		line := gtk.NewBox(gtk.OrientationHorizontal, albumTileGap)
		for _, size := range row {
			if i >= len(alb.Tiles) {
				break
			}
			more := 0
			if i == len(alb.Tiles)-1 {
				more = alb.More
			}
			line.Append(newAlbumTile(alb.Msgs[i], alb.Tiles[i], size, more, h))
			i++
		}
		grid.Append(line)
	}
	return grid
}

// albumTileWidget is one tile of the grid. The base child is the preview
// (or a hatch) at exactly the tile's size; the picture, the disc, the play
// glyph and the count float over it, so the tile never measures past its
// footprint whatever the picture's own size is.
type albumTileWidget struct {
	overlay *gtk.Overlay
	msg     client.Message
	mv      mediaView
	w, h    int
	hooks   bubbleHooks
	// disc is the download disc while the file is not cached; busy while a
	// fetch is out.
	disc *gtk.Label
	busy bool
}

func newAlbumTile(msg client.Message, mv mediaView, size albumTile, more int, h bubbleHooks) gtk.Widgetter {
	t := &albumTileWidget{overlay: gtk.NewOverlay(), msg: msg, mv: mv, w: size.W, h: size.H, hooks: h}
	t.overlay.AddCSSClass("chatot-album-tile")
	t.overlay.SetOverflow(gtk.OverflowHidden)
	t.overlay.SetSizeRequest(size.W, size.H)
	t.overlay.SetCursorFromName("pointer")
	t.setBase()
	if mv.HasLocal {
		t.showLocal()
	} else {
		t.showDownload()
	}
	if more > 0 {
		count := gtk.NewLabel(fmt.Sprintf("+%d", more))
		count.AddCSSClass("chatot-album-more")
		count.SetCanTarget(false)
		t.overlay.AddOverlay(count)
	}

	click := gtk.NewGestureClick()
	click.SetButton(gdk.BUTTON_PRIMARY)
	click.ConnectReleased(func(int, float64, float64) { t.activate() })
	t.overlay.AddController(click)
	right := gtk.NewGestureClick()
	right.SetButton(gdk.BUTTON_SECONDARY)
	right.ConnectPressed(func(_ int, x, y float64) { t.menu(x, y) })
	t.overlay.AddController(right)
	shotRegister(msg.ID, func(s *bubbleShot) {
		s.tileMenu = func() { t.menu(float64(size.W)/2, float64(size.H)/2) }
	})
	return t.overlay
}

// setBase puts the sender's preview under everything, or a hatch when the
// message carries none. A cached picture covers it; a cached clip keeps it
// as its poster (mediaVM drops the preview of a cached attachment, so it
// is read off the message).
func (t *albumTileWidget) setBase() {
	var pixbuf *gdkpixbuf.Pixbuf
	if thumb := t.msg.Attachment.Thumbnail; len(thumb) > 0 {
		pixbuf, _ = pixbufFromBytes(thumb)
	}
	if pixbuf != nil {
		t.overlay.SetChild(coverThumb(pixbuf, t.w, t.h, 0, false))
	} else {
		hatch := gtk.NewBox(gtk.OrientationVertical, 0)
		hatch.AddCSSClass("chatot-album-hatch")
		hatch.SetSizeRequest(t.w, t.h)
		t.overlay.SetChild(hatch)
	}
	if !t.mv.HasLocal && t.mv.Kind != "sticker" && t.hooks.onFetchThumbnail != nil && thumbnailIsStamp(pixbuf) {
		// The message's own preview is a stamp stretched over the tile:
		// ask for the sender's high-quality one, which refills the row.
		t.hooks.onFetchThumbnail(t.msg)
	}
	if t.mv.HasLocal && t.mv.Kind == "video" && pixbuf == nil {
		path := t.mv.LocalPath
		go func() {
			jpeg, err := media.VideoPoster(context.Background(), path)
			if err != nil {
				return
			}
			glib.IdleAdd(func() {
				if pb, err := pixbufFromBytes(jpeg); err == nil {
					t.overlay.SetChild(coverThumb(pb, t.w, t.h, 0, false))
				}
			})
		}()
	}
}

// showLocal lays the cached picture over the base, cropped to the tile, or
// a play glyph over a clip's poster.
func (t *albumTileWidget) showLocal() {
	if t.mv.Kind == "video" {
		play := gtk.NewLabel("▶")
		play.AddCSSClass("chatot-album-play")
		play.SetHAlign(gtk.AlignCenter)
		play.SetVAlign(gtk.AlignCenter)
		play.SetCanTarget(false)
		t.overlay.AddOverlay(play)
		return
	}
	p := gtk.NewPicture()
	p.SetCanShrink(true)
	p.SetContentFit(gtk.ContentFitCover)
	p.SetCanTarget(false)
	// Decoded at the photo bubble's size, so a picture shown both ways
	// shares one texture.
	loadPictureAsync(t.mv.LocalPath, inlinePhotoSide*2, p.SetPaintable)
	t.overlay.AddOverlay(p)
}

// showDownload centres the download disc on the tile; the tile fetches
// itself on click, or on its own under the auto-download setting.
func (t *albumTileWidget) showDownload() {
	t.disc = mediaDownloadCircle(true)
	t.disc.SetHAlign(gtk.AlignCenter)
	t.disc.SetVAlign(gtk.AlignCenter)
	t.disc.SetCanTarget(false)
	t.overlay.AddOverlay(t.disc)
	maybeAutoDownload(t.download, t.mv.Kind, t.msg.TS)
}

// activate is the tile's click: the viewer for a cached picture, a fetch
// for one that is not.
func (t *albumTileWidget) activate() {
	if t.mv.HasLocal {
		t.hooks.mediaOpener(t.msg)(t.mv.LocalPath)
		return
	}
	t.download()
}

// download fetches the tile's file off the main loop and shows it; the
// view learns the path too, so a refill or the viewer sees it cached. A
// failure turns the disc into a retry.
func (t *albumTileWidget) download() {
	if t.busy || t.mv.HasLocal || t.hooks.c == nil {
		return
	}
	t.busy = true
	t.disc.SetText("…")
	id, c := t.msg.ID, t.hooks.c
	go func() {
		path, err := c.DownloadMedia(context.Background(), id)
		glib.IdleAdd(func() {
			t.busy = false
			if err != nil {
				t.disc.SetText("↻")
				t.disc.AddCSSClass("chatot-media-dl-fail")
				t.disc.SetTooltipText("Download failed · click to retry")
				return
			}
			t.mv.HasLocal, t.mv.LocalPath = true, path
			if t.hooks.onLocalPath != nil {
				t.hooks.onLocalPath(id, path)
			}
			t.overlay.RemoveOverlay(t.disc)
			t.disc = nil
			t.showLocal()
		})
	}()
}

// menu opens the message menu for this one picture at the click, the way
// the chevron opens it for a bubble.
func (t *albumTileWidget) menu(x, y float64) {
	h := t.hooks
	items := h.menuItemsFor(t.msg, false, true)
	if len(items) == 0 {
		return
	}
	var host gtk.Widgetter = t.overlay
	px, py := int(x), int(y)
	if h.host != nil {
		if b, ok := t.overlay.ComputeBounds(h.host); ok {
			host = h.host
			px += int(b.X())
			py += int(b.Y())
		}
	}
	rect := gdk.NewRectangle(px, py, 1, 1)
	pop := newBubblePopover(host, t.overlay, &rect, func() {})
	pop.SetPosition(gtk.PosBottom)
	pop.SetChild(buildMenuBox(items, pop))
	pop.Popup()
}
