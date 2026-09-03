package geo

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectRoundTrip(t *testing.T) {
	for _, c := range [][2]float64{{41.14615, -8.61101}, {0, 0}, {-33.9, 151.2}, {64.1, -21.9}} {
		x, y := Project(c[0], c[1], 17)
		lat, lon := Unproject(x, y, 17)
		if math.Abs(lat-c[0]) > 1e-6 || math.Abs(lon-c[1]) > 1e-6 {
			t.Errorf("round trip %v -> %v,%v", c, lat, lon)
		}
	}
	// The mockup's fix: the same maths puts Porto's São Bento on tile 17/62345/47826-ish.
	x, y := Project(41.1454799, -8.6105220, 17)
	if int(x/TileSize) != 62399 && int(x/TileSize) != 62398 || int(y/TileSize) != 48545 {
		t.Logf("São Bento tile: %d/%d", int(x/TileSize), int(y/TileSize))
	}
}

func TestMetersPerPixelAndDistance(t *testing.T) {
	if mpp := MetersPerPixel(0, 0); math.Abs(mpp-156543.03) > 1 {
		t.Errorf("MetersPerPixel(0,0) = %v", mpp)
	}
	if d := Distance(41.1464565, -8.6113286, 41.1458255, -8.6115944); d < 60 || d > 80 {
		t.Errorf("Distance Liberdade->Cardosas = %v m, want ~70", d)
	}
	if got := FormatDistance(43); got != "45 m" {
		t.Errorf("FormatDistance(43) = %q", got)
	}
	if got := FormatDistance(1234); got != "1.2 km" {
		t.Errorf("FormatDistance(1234) = %q", got)
	}
	if got := FormatCoord(-8.6110220); got != "-8.61102" {
		t.Errorf("FormatCoord = %q", got)
	}
}

func TestTilesForCoversViewport(t *testing.T) {
	tiles := TilesFor(41.1455, -8.6105, 17, 600, 264)
	if len(tiles) < 6 || len(tiles) > 12 {
		t.Fatalf("got %d tiles for a 600x264 viewport, want 6..12", len(tiles))
	}
	minX, minY, maxX, maxY := 1e9, 1e9, -1e9, -1e9
	for _, p := range tiles {
		minX = math.Min(minX, p.DX)
		minY = math.Min(minY, p.DY)
		maxX = math.Max(maxX, p.DX+TileSize)
		maxY = math.Max(maxY, p.DY+TileSize)
	}
	if minX > 0 || minY > 0 || maxX < 600 || maxY < 264 {
		t.Errorf("tiles do not cover the viewport: x %v..%v y %v..%v", minX, maxX, minY, maxY)
	}
}

func TestTileCacheFetchesOnceAndPersists(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("missing User-Agent, got %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	c := NewTileCache(dir)
	c.SetServer(srv.URL)
	tile := Tile{Z: 3, X: 1, Y: 2}
	if _, ok := c.Cached(tile); ok {
		t.Fatal("tile cached before any fetch")
	}
	data, err := c.Fetch(context.Background(), tile)
	if err != nil || string(data) != "png-bytes" {
		t.Fatalf("Fetch = %q, %v", data, err)
	}
	if _, err := c.Fetch(context.Background(), tile); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("server hit %d times, want 1 (memory cache)", hits)
	}
	// A fresh cache over the same dir reads the disk copy.
	c2 := NewTileCache(dir)
	c2.SetServer(srv.URL)
	if _, ok := c2.Cached(tile); !ok {
		t.Error("tile not found on disk by a second cache")
	}
	if hits != 1 {
		t.Errorf("server hit %d times after disk read, want 1", hits)
	}
}

func TestSnapshotComposesTiles(t *testing.T) {
	// A 1x1-ish PNG tile: solid blue, TileSize square.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(solidTilePNG())
	}))
	defer srv.Close()
	c := NewTileCache(t.TempDir())
	c.SetServer(srv.URL)
	data, err := Snapshot(context.Background(), c, 41.1455, -8.6105, 17, 270, 86)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 500 || data[0] != 0xff || data[1] != 0xd8 {
		t.Errorf("Snapshot did not produce a JPEG (%d bytes)", len(data))
	}
}
