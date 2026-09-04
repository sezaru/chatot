package ui

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pangocairo"

	"chatot/internal/client"
)

// avatarCache is a view's own memo of jid -> resolved avatar path, so
// rebuilding a chat list / header doesn't re-fetch on every redraw. A jid
// present with path "" means "known to have no avatar" (still don't
// re-fetch); a jid absent means "not resolved yet".
type avatarCache struct {
	paths map[string]string
}

func newAvatarCache() *avatarCache {
	return &avatarCache{paths: make(map[string]string)}
}

// get reports the memoized path for jid and whether it's been resolved yet.
func (a *avatarCache) get(jid string) (path string, resolved bool) {
	path, resolved = a.paths[jid]
	return path, resolved
}

func (a *avatarCache) set(jid, path string) { a.paths[jid] = path }

// invalidate drops jid so the next buildAvatar call for it re-fetches.
func (a *avatarCache) invalidate(jid string) {
	if path := a.paths[jid]; path != "" {
		// The picture file is rewritten in place, so its decoded copy is
		// stale too.
		delete(pictureTextures, path+"|"+strconv.Itoa(avatarDecodeSide))
	}
	delete(a.paths, jid)
}

// buildAvatar returns a size x size container showing jid's avatar picture if
// known, else initial as an immediate fallback with an async fetch kicked off
// in the background (unless cache already knows jid has none). The fetch
// swaps the picture in via glib.IdleAdd when it completes; box is captured by
// that closure so a stale swap (the row/header having since been rebuilt or
// removed) is harmless — it just mutates a widget nobody's looking at anymore.
func buildAvatar(c client.Client, cache *avatarCache, jid, initial string, size int) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetSizeRequest(size, size)

	if path, resolved := cache.get(jid); resolved {
		if path != "" {
			box.Append(newAvatarPicture(path, initial, size))
		} else {
			box.Append(newAvatarInitial(jid, initial, size))
		}
		return box
	}

	box.Append(newAvatarInitial(jid, initial, size))

	go func() {
		path, err := c.Avatar(context.Background(), jid)
		if err != nil {
			// Transient (not connected yet, timeout): leave jid unresolved so
			// the next rebuild retries instead of pinning the initial.
			return
		}
		glib.IdleAdd(func() {
			cache.set(jid, path)
			if path == "" {
				return
			}
			removeAllChildren(box)
			box.Append(newAvatarPicture(path, initial, size))
		})
	}()

	return box
}

// newAvatarInitial is the initials disc: exactly size×size whatever the
// glyph, so a wide or tall fallback-font character (₿, an emoji) can't
// stretch it into an oval the way it did a label. The disc and its palette
// colour, radius and font still come from the CSS classes; only the glyph
// is drawn here, centred, in the CSS colour.
func newAvatarInitial(jid, initial string, size int) *gtk.DrawingArea {
	area := gtk.NewDrawingArea()
	area.AddCSSClass("chatot-avatar")
	area.AddCSSClass(avatarColorClass(jid))
	area.SetSizeRequest(size, size)
	area.SetHAlign(gtk.AlignCenter)
	area.SetVAlign(gtk.AlignCenter)
	area.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		layout := area.CreatePangoLayout(initial)
		lw, lh := layout.PixelSize()
		c := area.Color()
		cr.SetSourceRGBA(float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha()))
		cr.MoveTo(float64(w-lw)/2, float64(h-lh)/2)
		pangocairo.ShowLayout(cr, layout)
	})
	return area
}

// avatarColorClass maps jid to one of the 8 fixed initials-avatar palette
// classes via FNV-1a, so a contact always gets the same colour.
func avatarColorClass(jid string) string {
	h := fnv.New32a()
	h.Write([]byte(jid))
	return fmt.Sprintf("chatot-avatar-c%d", h.Sum32()%8)
}

// newAvatarPicture renders a photo avatar as a size×size circle. AdwAvatar
// rather than a GtkPicture: a picture's natural size is the image's own, so
// inside a box it grew past the request and, with the row taller than it
// was wide, covered an oval instead of a circle. AdwAvatar is a fixed-size
// widget that crops its custom image to a true disc.
// The file is decoded off the main loop; the initial shows until it lands.
func newAvatarPicture(path, initial string, size int) gtk.Widgetter {
	avatar := adw.NewAvatar(size, initial, true)
	avatar.AddCSSClass("chatot-avatar-img")
	loadPictureAsync(path, avatarDecodeSide, func(t gdk.Paintabler) { avatar.SetCustomImage(t) })
	return avatar
}

// avatarDecodeSide is the box avatar files are decoded into: the largest
// avatar on screen is the profile card's, well under this on HiDPI.
const avatarDecodeSide = 192

// initialFor derives the single-uppercase-letter fallback shown until (or
// instead of) a real avatar picture: the first rune of name, or "?" if name
// is empty.
func initialFor(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// centreGlyph re-centres a size×size label's text on its ink rather than
// its font box. GtkLabel centres the layout's logical rectangle (ascent +
// descent), so a capital or an emoji, whose ink sits high and sometimes
// left in that box, lands a pixel or two off the disc's centre; how far
// depends on the font the user's system resolves, so the offset is
// measured from the laid-out glyph rather than tuned by hand.
func centreGlyph(label *gtk.Label, size int) {
	apply := func() {
		layout := label.Layout()
		if layout == nil {
			return
		}
		ink, logical := layout.PixelExtents()
		x, y := glyphAlign(size, size,
			ink.X(), ink.Y(), ink.Width(), ink.Height(),
			logical.X(), logical.Y(), logical.Width(), logical.Height())
		label.SetXAlign(float32(x))
		label.SetYAlign(float32(y))
	}
	// Map, not construction: the CSS font (size, weight) is only resolved
	// once the label is in a mapped tree, and the extents follow it.
	label.ConnectMap(apply)
	label.NotifyProperty("label", func() {
		if label.Mapped() {
			apply()
		}
	})
}

// glyphAlign returns the xalign/yalign that put the ink rectangle's centre
// in the middle of a w×h label, given the layout's pixel extents. GtkLabel
// places the logical rectangle's origin at align × (widget − logical) on
// each axis, so the ink centre lands at that plus its offset inside the
// logical box. An axis the text already fills cannot be shifted and keeps
// the default 0.5.
func glyphAlign(w, h, ix, iy, iw, ih, lx, ly, lw, lh int) (x, y float64) {
	axis := func(size, inkOff, inkLen, logicalLen int) float64 {
		room := size - logicalLen
		if room <= 0 {
			return 0.5
		}
		want := float64(size)/2 - float64(inkOff) - float64(inkLen)/2
		return min(1, max(0, want/float64(room)))
	}
	return axis(w, ix-lx, iw, lw), axis(h, iy-ly, ih, lh)
}
