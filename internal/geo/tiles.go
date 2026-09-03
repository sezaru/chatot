// Package geo holds the map and positioning plumbing behind the location
// sheet: Web-Mercator maths, an OpenStreetMap raster tile cache, Nominatim
// search and the XDG location portal. Pure Go apart from the portal's D-Bus
// use through gio, so the maths and cache are testable without a display.
package geo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// TileSize is the OSM raster tile edge, in px.
const TileSize = 256

// UserAgent identifies chatot to the tile and search servers, as the
// OpenStreetMap usage policies require.
const UserAgent = "chatot/0.1 (+https://github.com/sezdocs/chatot)"

// Project maps a WGS84 coordinate to Web-Mercator world pixels at zoom.
func Project(lat, lon float64, zoom int) (x, y float64) {
	w := float64(TileSize) * math.Exp2(float64(zoom))
	s := math.Sin(lat * math.Pi / 180)
	// Clamp the pole singularity the way slippy maps do.
	s = math.Max(-0.9999, math.Min(0.9999, s))
	x = (lon + 180) / 360 * w
	y = (0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)) * w
	return x, y
}

// Unproject is Project's inverse.
func Unproject(x, y float64, zoom int) (lat, lon float64) {
	w := float64(TileSize) * math.Exp2(float64(zoom))
	t := math.Pi * (1 - 2*y/w)
	lat = math.Atan(math.Sinh(t)) * 180 / math.Pi
	lon = x/w*360 - 180
	return lat, lon
}

// MetersPerPixel is the ground resolution at lat and zoom, so an accuracy
// radius can be drawn at its true size.
func MetersPerPixel(lat float64, zoom int) float64 {
	return 156543.03392 * math.Cos(lat*math.Pi/180) / math.Exp2(float64(zoom))
}

// Distance is the great-circle distance between two points, in metres.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	s1 := math.Sin((lat2 - lat1) * rad / 2)
	s2 := math.Sin((lon2 - lon1) * rad / 2)
	h := s1*s1 + math.Cos(lat1*rad)*math.Cos(lat2*rad)*s2*s2
	return 2 * r * math.Asin(math.Sqrt(h))
}

// FormatDistance renders metres the way the mockup's result rows do:
// "45 m" (rounded to 5) under a kilometre, "1.2 km" above.
func FormatDistance(m float64) string {
	if m < 950 {
		return strconv.Itoa(int(math.Round(m/5))*5) + " m"
	}
	return strconv.FormatFloat(m/1000, 'f', 1, 64) + " km"
}

// FormatCoord renders a coordinate with five decimals ("41.14615").
func FormatCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', 5, 64)
}

// Tile addresses one raster tile.
type Tile struct{ Z, X, Y int }

// TileCache fetches OSM tiles over HTTP and keeps them on disk under dir
// (one PNG per tile) and in memory for the session. Fetches are limited to
// two at a time, and never bulk-prefetched, per the tile usage policy.
type TileCache struct {
	dir    string
	server string
	client *http.Client

	mu       sync.Mutex
	mem      map[Tile][]byte
	inflight map[Tile]bool
	sem      chan struct{}
}

// DefaultTileServer is the public OSM raster endpoint.
const DefaultTileServer = "https://tile.openstreetmap.org"

// Attribution is the credit line the map must show.
const Attribution = "© OpenStreetMap contributors"

// NewTileCache makes a cache rooted at dir (created on demand).
func NewTileCache(dir string) *TileCache {
	return &TileCache{
		dir:      dir,
		server:   DefaultTileServer,
		client:   &http.Client{Timeout: 15 * time.Second},
		mem:      map[Tile][]byte{},
		inflight: map[Tile]bool{},
		sem:      make(chan struct{}, 2),
	}
}

// SetServer points the cache at another tile source (tests, self-hosting).
// Tiles are cached per server.
func (c *TileCache) SetServer(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if url == c.server {
		return
	}
	c.server = url
	c.mem = map[Tile][]byte{}
}

// Server is the tile source in use.
func (c *TileCache) Server() string { return c.server }

// Cached returns the tile's PNG if it is already in memory or on disk,
// without touching the network. ok=false means Fetch is needed.
func (c *TileCache) Cached(t Tile) ([]byte, bool) {
	c.mu.Lock()
	data, ok := c.mem[t]
	c.mu.Unlock()
	if ok {
		return data, true
	}
	data, err := os.ReadFile(c.path(t))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	c.mu.Lock()
	c.mem[t] = data
	c.mu.Unlock()
	return data, true
}

// ErrInFlight reports that another Fetch of the same tile is already
// running; the caller will get the tile through that fetch's completion.
var ErrInFlight = errors.New("chatot/geo: tile fetch already in flight")

// Fetch downloads a tile (or returns it from cache). At most one download
// per tile runs at a time: a second concurrent call gets ErrInFlight.
func (c *TileCache) Fetch(ctx context.Context, t Tile) ([]byte, error) {
	if data, ok := c.Cached(t); ok {
		return data, nil
	}
	c.mu.Lock()
	if c.inflight[t] {
		c.mu.Unlock()
		return nil, ErrInFlight
	}
	c.inflight[t] = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.inflight, t)
		c.mu.Unlock()
	}()

	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	url := fmt.Sprintf("%s/%d/%d/%d.png", c.server, t.Z, t.X, t.Y)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatot/geo: tile %v: HTTP %d", t, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.mem[t] = data
	c.mu.Unlock()
	p := c.path(t)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err == nil {
		_ = os.WriteFile(p, data, 0o600)
	}
	return data, nil
}

func (c *TileCache) path(t Tile) string {
	style := "osm"
	if c.server != DefaultTileServer {
		style = "other"
	}
	return filepath.Join(c.dir, style, strconv.Itoa(t.Z), strconv.Itoa(t.X), strconv.Itoa(t.Y)+".png")
}

// TilesFor lists the tiles covering a w×h viewport centred on (lat, lon)
// at zoom, with each tile's pixel offset from the viewport's top-left.
// Tiles outside the world (y < 0 or ≥ 2^z) are skipped; x wraps.
func TilesFor(lat, lon float64, zoom, w, h int) []PlacedTile {
	cx, cy := Project(lat, lon, zoom)
	left := cx - float64(w)/2
	top := cy - float64(h)/2
	n := 1 << uint(zoom)
	x0 := int(math.Floor(left / TileSize))
	y0 := int(math.Floor(top / TileSize))
	x1 := int(math.Floor((left + float64(w)) / TileSize))
	y1 := int(math.Floor((top + float64(h)) / TileSize))
	var out []PlacedTile
	for ty := y0; ty <= y1; ty++ {
		if ty < 0 || ty >= n {
			continue
		}
		for tx := x0; tx <= x1; tx++ {
			wx := ((tx % n) + n) % n
			out = append(out, PlacedTile{
				Tile: Tile{Z: zoom, X: wx, Y: ty},
				DX:   float64(tx*TileSize) - left,
				DY:   float64(ty*TileSize) - top,
			})
		}
	}
	return out
}

// PlacedTile is a tile plus where it lands in a viewport.
type PlacedTile struct {
	Tile   Tile
	DX, DY float64
}
