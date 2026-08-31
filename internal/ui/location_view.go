package ui

import (
	"strconv"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// locationView is the pure display view-model for a location bubble, computed
// from a client.Message so it's unit-testable without a display.
type locationView struct {
	IsLocation bool
	Live       bool
	Title      string // Name, or "Location"/"Live location [· until HH:MM]" when unnamed
	Address    string
	Coords     string // "lat, long"
	MapsURL    string
}

// locationVM derives the location view-model for m (zero value if m carries
// no location).
func locationVM(m client.Message) locationView {
	if m.Location == nil {
		return locationView{}
	}
	loc := *m.Location
	title := loc.Name
	if title == "" {
		if loc.IsLive {
			title = "Live location"
		} else {
			title = "Location"
		}
	}
	// LiveUntil is 0 for a live location whose expiry chatot doesn't know
	// (e.g. received from another WhatsApp client); the bare "Live location"
	// title already conveys the live state in that case.
	if loc.IsLive && loc.LiveUntil != 0 {
		title += " · until " + time.Unix(loc.LiveUntil, 0).UTC().Format("15:04")
	}
	return locationView{
		IsLocation: true,
		Live:       loc.IsLive,
		Title:      title,
		Address:    loc.Address,
		Coords:     fmtCoord(loc.Latitude) + ", " + fmtCoord(loc.Longitude),
		MapsURL:    mapsURL(loc.Latitude, loc.Longitude),
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

// buildLocationContent renders a location bubble: a 📍 title, an optional
// address, the coordinates, and a link opening the point in OpenStreetMap.
func buildLocationContent(v locationView) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 2)
	box.AddCSSClass("chatot-bubble-location")

	title := gtk.NewLabel("📍 " + v.Title)
	title.SetXAlign(0)
	title.AddCSSClass("chatot-location-title")
	if v.Live {
		title.AddCSSClass("chatot-location-live")
	}
	box.Append(title)

	if v.Address != "" {
		addr := gtk.NewLabel(v.Address)
		addr.SetXAlign(0)
		addr.SetWrap(true)
		box.Append(addr)
	}

	coords := gtk.NewLabel(v.Coords)
	coords.SetXAlign(0)
	coords.AddCSSClass("chatot-location-coords")
	box.Append(coords)

	link := gtk.NewLinkButtonWithLabel(v.MapsURL, "Open in maps")
	link.SetHAlign(gtk.AlignStart)
	box.Append(link)

	return box
}
