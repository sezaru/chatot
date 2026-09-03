package ui

import (
	"context"
	"strconv"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// locationView is the pure display view-model for a location bubble, computed
// from a client.Message so it's unit-testable without a display.
type locationView struct {
	IsLocation bool
	// Live is a share still running: the tile carries the LIVE badge and
	// an own bubble offers Stop sharing. Ended is a share that expired or
	// was stopped: grey pin, "Live location ended".
	Live    bool
	Ended   bool
	Title   string // Name, or "Location"/"Live location [· until HH:MM]" when unnamed
	Address string
	Coords  string // "lat, long"
	Sub     string // the mockup's dim second line under the title
	MapsURL string
	// Lat/Lon back the reverse lookup that names an unnamed point.
	Lat, Lon float64
	// NeedsPlace is set when the sender gave no address: the subline
	// starts as coordinates and a reverse geocode fills in the street.
	NeedsPlace bool
	// Thumbnail is the sender's embedded map preview, drawn in the tile in
	// place of the hatched placeholder when present.
	Thumbnail []byte
}

// locationSub is the mockup's dim subline: the address when the sender gave
// one, else the raw coordinates (a reverse lookup replaces those once it
// answers; see buildLocationContent).
func locationSub(address, coords string) string {
	if address != "" {
		return address
	}
	return coords
}

// locationVM derives the location view-model for m (zero value if m carries
// no location), judged against the current time.
func locationVM(m client.Message) locationView { return locationVMAt(m, time.Now()) }

// locationVMAt is locationVM at an explicit now, so a live share's running
// vs ended state is testable.
func locationVMAt(m client.Message, now time.Time) locationView {
	if m.Location == nil {
		return locationView{}
	}
	loc := *m.Location
	// A share with a known expiry in the past has ended, whether it ran out
	// or Stop sharing moved LiveUntil to the stop time.
	ended := loc.IsLive && loc.LiveUntil != 0 && !now.Before(time.Unix(loc.LiveUntil, 0))
	live := loc.IsLive && !ended
	coords := fmtCoord(loc.Latitude) + ", " + fmtCoord(loc.Longitude)
	place := loc.Address
	if place == "" {
		place = coords
	}
	title := loc.Name
	sub := locationSub(loc.Address, coords)
	switch {
	case ended:
		title = "Live location ended"
		sub = "Stopped sharing at " + time.Unix(loc.LiveUntil, 0).UTC().Format("15:04")
	case live:
		if title == "" {
			title = "Live location"
		}
		// LiveUntil is 0 for a live location whose expiry chatot doesn't
		// know (received from another WhatsApp client); the bare "Live
		// location" title already conveys the live state in that case.
		if loc.LiveUntil != 0 {
			title += " · until " + time.Unix(loc.LiveUntil, 0).UTC().Format("15:04")
		}
		sub = place
	case title == "":
		title = "Location"
	}
	return locationView{
		IsLocation: true,
		Live:       live,
		Ended:      ended,
		Title:      title,
		Address:    loc.Address,
		Coords:     coords,
		Sub:        sub,
		MapsURL:    mapsURL(loc.Latitude, loc.Longitude),
		Thumbnail:  loc.Thumbnail,
		Lat:        loc.Latitude,
		Lon:        loc.Longitude,
		NeedsPlace: loc.Address == "" && !ended,
	}
}

// fmtCoord renders a coordinate without trailing zeros, deterministically.
func fmtCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// mapsURL builds an OpenStreetMap deep link that drops a marker at lat/long
// and centres the map on it at zoom 16.
func mapsURL(lat, long float64) string {
	la, lo := fmtCoord(lat), fmtCoord(long)
	return "https://www.openstreetmap.org/?mlat=" + la + "&mlon=" + lo + "#map=16/" + la + "/" + lo
}

// locationTileW/H is the design's map tile; every location bubble is exactly
// this wide, with the title and subline wrapping underneath it.
const (
	locationTileW = 270
	locationTileH = 86
)

// buildLocationContent renders the mockup's location bubble: a 270×86 map
// tile with a red pin dot centred on it, then a bold "📍 title" and a dim
// subline. The tile shows the sender's embedded map preview when the message
// carries one and the design's hatched placeholder otherwise. The whole card
// is one click target that opens the point in OpenStreetMap — the mockup has
// no separate link row, and the subline says so.
//
// onStop, when non-nil, adds the mockup's Stop sharing button under a
// running own live share. onOpen is the card's click (the viewer pane);
// nil opens the point in the browser's map instead.
func buildLocationContent(v locationView, onStop, onOpen func()) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 2)
	box.AddCSSClass("chatot-bubble-location")

	tile := gtk.NewOverlay()
	tile.AddCSSClass("chatot-location-tile")
	tile.SetSizeRequest(locationTileW, locationTileH)
	// Clip the map to the tile's rounded corners; GTK only honours a CSS
	// radius on a child's drawing when the widget itself clips.
	tile.SetOverflow(gtk.OverflowHidden)
	tile.SetChild(locationMap(v.Thumbnail))
	dot := gtk.NewBox(gtk.OrientationVertical, 0)
	dot.AddCSSClass("chatot-location-pin")
	dot.SetHAlign(gtk.AlignCenter)
	dot.SetVAlign(gtk.AlignCenter)
	dot.SetSizeRequest(13, 13)
	if v.Ended {
		dot.AddCSSClass("chatot-location-pin-ended")
	}
	tile.AddOverlay(dot)
	if v.Live {
		badge := gtk.NewBox(gtk.OrientationHorizontal, 5)
		badge.AddCSSClass("chatot-location-live-badge")
		badge.SetHAlign(gtk.AlignStart)
		badge.SetVAlign(gtk.AlignStart)
		badge.SetMarginStart(8)
		badge.SetMarginTop(8)
		liveDot := gtk.NewBox(gtk.OrientationVertical, 0)
		liveDot.AddCSSClass("chatot-location-live-dot")
		liveDot.SetSizeRequest(6, 6)
		liveDot.SetVAlign(gtk.AlignCenter)
		badge.Append(liveDot)
		badge.Append(gtk.NewLabel("LIVE"))
		tile.AddOverlay(badge)
	}
	box.Append(tile)

	title := gtk.NewLabel("📍 " + v.Title)
	title.SetXAlign(0)
	title.SetEllipsize(pango.EllipsizeEnd)
	title.AddCSSClass("chatot-location-title")
	if v.Live {
		title.AddCSSClass("chatot-location-live")
	}
	box.Append(title)

	// The tile sets the bubble's width; a long address wraps under it rather
	// than stretching the bubble past the map (which left the tile hugging the
	// left edge with dead space beside it).
	sub := gtk.NewLabel(v.Sub)
	sub.SetXAlign(0)
	sub.SetWrap(true)
	sub.SetWrapMode(pango.WrapWordChar)
	sub.SetMaxWidthChars(locationSubChars)
	sub.AddCSSClass("chatot-location-sub")
	box.Append(sub)
	if v.NeedsPlace {
		// A point without an address (every live share, and phones that
		// send bare coordinates) gets its street from a reverse lookup.
		lookupPlace(v.Lat, v.Lon, func(place string) {
			if place != "" {
				sub.SetLabel(place)
			}
		})
	}

	btn := gtk.NewButton()
	btn.SetChild(box)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btn")
	btn.SetFocusOnClick(false)
	url := v.MapsURL
	btn.ConnectClicked(func() {
		if onOpen != nil {
			onOpen()
			return
		}
		openURI(url)
	})
	if onStop == nil {
		return btn
	}
	// The card stays one click target; Stop sharing sits under it as its
	// own button so a click there never opens the map.
	col := gtk.NewBox(gtk.OrientationVertical, 0)
	col.Append(btn)
	stop := gtk.NewButtonWithLabel("Stop sharing")
	stop.AddCSSClass("chatot-location-stop")
	stop.SetHAlign(gtk.AlignStart)
	stop.ConnectClicked(onStop)
	col.Append(stop)
	return col
}

// locationSubChars is the subline's wrap width in characters: at its 11px
// size that is roughly the 270px tile.
const locationSubChars = 44

// locationMap is the tile's fill: the embedded map preview scaled to cover
// the tile, or the hatched placeholder when there is none (or it fails to
// decode, which a corrupt sender thumbnail can do).
func locationMap(thumbnail []byte) gtk.Widgetter {
	if len(thumbnail) > 0 {
		if texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(thumbnail)); err == nil {
			pic := gtk.NewPictureForPaintable(texture)
			pic.SetContentFit(gtk.ContentFitCover)
			pic.SetCanShrink(true)
			pic.SetHExpand(true)
			pic.SetVExpand(true)
			return pic
		}
	}
	hatch := gtk.NewBox(gtk.OrientationVertical, 0)
	hatch.AddCSSClass("chatot-location-hatch")
	hatch.SetHExpand(true)
	hatch.SetVExpand(true)
	return hatch
}

// placeNames memoizes reverse lookups by rounded coordinates so a thread of
// live-share bubbles for one spot asks Nominatim once; nil-safe under the
// GTK main loop, which is the only caller.
var placeNames = map[string]string{}

// lookupPlace resolves (lat, lon) to a "street, area" line off the main loop
// and hands it to done on the main loop. done gets "" when nothing came
// back. Cached answers are delivered synchronously.
func lookupPlace(lat, lon float64, done func(place string)) {
	key := strconv.FormatFloat(lat, 'f', 4, 64) + "," + strconv.FormatFloat(lon, 'f', 4, 64)
	if place, ok := placeNames[key]; ok {
		done(place)
		return
	}
	go func() {
		p, err := sharedSearcher.Reverse(context.Background(), lat, lon)
		place := ""
		if err == nil {
			place = joinMeta(p.Name, p.Address)
		}
		glib.IdleAdd(func() {
			placeNames[key] = place
			done(place)
		})
	}()
}
