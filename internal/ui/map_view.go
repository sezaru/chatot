package ui

import (
	"context"
	"math"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/geo"
)

// mapZoomMin/Max bound the picker's zoom: wide enough to find a city
// without a fix, close enough to place a pin on a doorstep.
const (
	mapZoomMin = 3
	mapZoomMax = 19
)

// sharedTiles is the process-wide tile cache (disk under the XDG cache
// dir), so reopening the sheet never refetches what it already showed.
var sharedTiles = newSharedTiles()

func newSharedTiles() *geo.TileCache {
	return geo.NewTileCache(filepath.Join(cacheDir(), "tiles"))
}

// mapMarker is what the map draws on top of the tiles.
type mapMarker int

const (
	markerNone mapMarker = iota
	markerFix            // accent dot with an accuracy halo (a positioning fix)
	markerPin            // the red drop pin (a point picked by hand)
)

// mapView is the picker's map: a DrawingArea painting OSM tiles around a
// centre, with drag-to-pan, wheel zoom, click-to-place and one marker.
// Tiles it doesn't have yet are fetched in the background and painted when
// they land; the tile cache is shared.
type mapView struct {
	*gtk.DrawingArea

	tiles *geo.TileCache
	// centre and zoom define the viewport.
	lat, lon float64
	zoom     int

	marker            mapMarker
	mLat, mLon        float64
	accuracy          float64 // metres, drawn as the fix halo
	interactive       bool    // false while locating: clicks are ignored
	dragLat, dragLon  float64 // centre when the drag began
	dragged           bool    // the current press moved the map, so its release is not a click
	pixbufs           map[geo.Tile]*gdkpixbuf.Pixbuf
	dark              bool // re-render tiles for the dark scheme (see geo.DarkTile)
	onClick           func(lat, lon float64)
	onViewportChanged func()
}

func newMapView(tiles *geo.TileCache) *mapView {
	m := &mapView{DrawingArea: gtk.NewDrawingArea(), tiles: tiles, zoom: 17, pixbufs: map[geo.Tile]*gdkpixbuf.Pixbuf{}, interactive: true, dark: isDark()}
	m.AddCSSClass("chatot-map")
	m.SetHExpand(true)
	m.SetVExpand(true)
	m.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) { m.draw(cr, w, h) })

	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		if !m.interactive || m.onClick == nil || n != 1 || m.dragged {
			return
		}
		lat, lon := m.pointAt(x, y)
		m.onClick(lat, lon)
	})
	m.AddController(click)

	drag := gtk.NewGestureDrag()
	drag.ConnectDragBegin(func(_, _ float64) {
		m.dragLat, m.dragLon = m.lat, m.lon
		m.dragged = false
	})
	drag.ConnectDragUpdate(func(dx, dy float64) {
		if !m.interactive || (math.Abs(dx) < 3 && math.Abs(dy) < 3) {
			return
		}
		m.dragged = true
		// Moving the pointer right moves the map right: the centre goes left.
		cx, cy := geo.Project(m.dragLat, m.dragLon, m.zoom)
		m.lat, m.lon = geo.Unproject(cx-dx, cy-dy, m.zoom)
		m.viewportChanged()
	})
	m.AddController(drag)

	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical | gtk.EventControllerScrollDiscrete)
	scroll.ConnectScroll(func(_, dy float64) bool {
		if !m.interactive {
			return false
		}
		if dy < 0 {
			m.ZoomBy(1)
		} else if dy > 0 {
			m.ZoomBy(-1)
		}
		return true
	})
	m.AddController(scroll)
	return m
}

// SetCentre moves the viewport.
func (m *mapView) SetCentre(lat, lon float64) {
	m.lat, m.lon = lat, lon
	m.viewportChanged()
}

// SetZoom sets the zoom level, clamped to the picker's range.
func (m *mapView) SetZoom(z int) {
	m.zoom = clampInt(z, mapZoomMin, mapZoomMax)
	m.viewportChanged()
}

// ZoomBy steps the zoom.
func (m *mapView) ZoomBy(d int) { m.SetZoom(m.zoom + d) }

// Zoom is the current zoom level.
func (m *mapView) Zoom() int { return m.zoom }

// SetMarker places (or removes, with markerNone) the marker.
func (m *mapView) SetMarker(kind mapMarker, lat, lon, accuracy float64) {
	m.marker, m.mLat, m.mLon, m.accuracy = kind, lat, lon, accuracy
	m.QueueDraw()
}

// SetInteractive turns clicks, drags and wheel zoom on or off.
func (m *mapView) SetInteractive(on bool) {
	m.interactive = on
	if on {
		m.SetCursorFromName("crosshair")
	} else {
		m.SetCursorFromName("default")
	}
}

func (m *mapView) viewportChanged() {
	m.QueueDraw()
	if m.onViewportChanged != nil {
		m.onViewportChanged()
	}
}

// pointAt converts a widget coordinate into a WGS84 point.
func (m *mapView) pointAt(x, y float64) (lat, lon float64) {
	w, h := float64(m.Width()), float64(m.Height())
	cx, cy := geo.Project(m.lat, m.lon, m.zoom)
	return geo.Unproject(cx-w/2+x, cy-h/2+y, m.zoom)
}

// screenPoint is pointAt's inverse.
func (m *mapView) screenPoint(lat, lon float64, w, h float64) (x, y float64) {
	cx, cy := geo.Project(m.lat, m.lon, m.zoom)
	px, py := geo.Project(lat, lon, m.zoom)
	return px - cx + w/2, py - cy + h/2
}

func (m *mapView) draw(cr *cairo.Context, w, h int) {
	fw, fh := float64(w), float64(h)
	for _, p := range geo.TilesFor(m.lat, m.lon, m.zoom, w, h) {
		pb := m.pixbufFor(p.Tile)
		if pb == nil {
			continue
		}
		cr.Save()
		gdk.CairoSetSourcePixbuf(cr, pb, math.Round(p.DX), math.Round(p.DY))
		cr.Rectangle(math.Round(p.DX), math.Round(p.DY), geo.TileSize, geo.TileSize)
		cr.Fill()
		cr.Restore()
	}

	switch m.marker {
	case markerFix:
		x, y := m.screenPoint(m.mLat, m.mLon, fw, fh)
		// The halo is the accuracy at its true size (never under 28px so a
		// precise fix still reads as one).
		r := math.Max(14, m.accuracy/geo.MetersPerPixel(m.mLat, m.zoom))
		cr.SetSourceRGBA(0x1b/255.0, 0x8c/255.0, 0x72/255.0, 0.17)
		cr.Arc(x, y, r, 0, 2*math.Pi)
		cr.FillPreserve()
		cr.SetSourceRGBA(0x1b/255.0, 0x8c/255.0, 0x72/255.0, 0.48)
		cr.SetLineWidth(1)
		cr.Stroke()
		// The dot: accent with a white ring and a soft shadow.
		cr.SetSourceRGBA(0, 0, 0, 0.25)
		cr.Arc(x, y+1, 9, 0, 2*math.Pi)
		cr.Fill()
		cr.SetSourceRGB(1, 1, 1)
		cr.Arc(x, y, 8, 0, 2*math.Pi)
		cr.Fill()
		cr.SetSourceRGB(0x1b/255.0, 0x8c/255.0, 0x72/255.0)
		cr.Arc(x, y, 5, 0, 2*math.Pi)
		cr.Fill()
	case markerPin:
		x, y := m.screenPoint(m.mLat, m.mLon, fw, fh)
		drawDropPin(cr, x, y)
	}
}

// drawDropPin paints the mockup's red teardrop pin with its tip at (x, y):
// a rotated rounded square, white-edged, over a small shadow dot.
func drawDropPin(cr *cairo.Context, x, y float64) {
	cr.SetSourceRGBA(0, 0, 0, 0.4)
	cr.Arc(x, y+2, 2.5, 0, 2*math.Pi)
	cr.Fill()
	cr.Save()
	cr.Translate(x, y-14)
	cr.Rotate(math.Pi / 4)
	// Body: 20px square, three corners rounded, the fourth (pointing down
	// after the rotation) sharp.
	const s, r = 10.0, 9.0
	cr.NewSubPath()
	cr.Arc(-s+r, -s+r, r, math.Pi, 3*math.Pi/2)
	cr.Arc(s-r, -s+r, r, -math.Pi/2, 0)
	cr.LineTo(s, s)
	cr.Arc(-s+r, s-r, r, math.Pi/2, math.Pi)
	cr.ClosePath()
	cr.SetSourceRGB(0xc0/255.0, 0x1c/255.0, 0x28/255.0)
	cr.FillPreserve()
	cr.SetSourceRGB(1, 1, 1)
	cr.SetLineWidth(2)
	cr.Stroke()
	cr.Restore()
}

// pixbufFor returns the decoded tile if it is cached, else kicks off a
// fetch that redraws on completion and returns nil.
func (m *mapView) pixbufFor(t geo.Tile) *gdkpixbuf.Pixbuf {
	if pb, ok := m.pixbufs[t]; ok {
		return pb
	}
	if data, ok := m.tiles.Cached(t); ok {
		if m.dark {
			// The mockup inverts its map in the dark scheme; the tile is
			// re-rendered once and kept decoded.
			if d, err := geo.DarkTile(data); err == nil {
				data = d
			}
		}
		pb, err := pixbufFromBytes(data)
		if err != nil {
			return nil
		}
		m.pixbufs[t] = pb
		return pb
	}
	go func() {
		if _, err := m.tiles.Fetch(context.Background(), t); err != nil {
			return
		}
		glib.IdleAdd(func() { m.QueueDraw() })
	}()
	return nil
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
