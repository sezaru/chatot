package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Place is one geocoding hit.
type Place struct {
	Name    string
	Address string
	Lat     float64
	Lon     float64
}

// DefaultSearchServer is the public Nominatim endpoint.
const DefaultSearchServer = "https://nominatim.openstreetmap.org"

// Searcher queries Nominatim, honouring its usage policy: an identifying
// User-Agent, at most one request per second, and a small result cache so
// retyping a query never asks twice.
type Searcher struct {
	server string
	client *http.Client

	mu    sync.Mutex
	last  time.Time
	cache map[string][]Place
}

func NewSearcher() *Searcher {
	return &Searcher{
		server: DefaultSearchServer,
		client: &http.Client{Timeout: 10 * time.Second},
		cache:  map[string][]Place{},
	}
}

// SetServer points the searcher elsewhere (tests).
func (s *Searcher) SetServer(url string) { s.server = url }

// Search geocodes q, biased towards a viewbox around (lat, lon). Results
// keep Nominatim's ranking; each carries the place name and a shortened
// address line.
func (s *Searcher) Search(ctx context.Context, q string, lat, lon float64) ([]Place, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("q", q)
	params.Set("format", "jsonv2")
	params.Set("limit", "6")
	params.Set("addressdetails", "0")
	// A ~20 km box around the map centre, as a preference rather than a
	// hard bound, so "Praça da Liberdade" finds the one nearby first.
	const d = 0.1
	params.Set("viewbox", fmt.Sprintf("%s,%s,%s,%s",
		FormatCoord(lon-d), FormatCoord(lat+d), FormatCoord(lon+d), FormatCoord(lat-d)))
	key := strings.ToLower(q) + "@" + FormatCoord(lat) + "," + FormatCoord(lon)
	return s.fetch(ctx, key, "/search", params, false)
}

// Reverse names the place at (lat, lon): the nearest addressable feature
// as Nominatim sees it, with Name the street/building and Address the rest
// of the display name. Coordinates are rounded to ~10 m for the cache key so
// a live share hovering around one spot asks once.
func (s *Searcher) Reverse(ctx context.Context, lat, lon float64) (Place, error) {
	params := url.Values{}
	params.Set("lat", FormatCoord(lat))
	params.Set("lon", FormatCoord(lon))
	params.Set("format", "jsonv2")
	params.Set("zoom", "18")
	params.Set("addressdetails", "0")
	key := "rev@" + strconv.FormatFloat(lat, 'f', 4, 64) + "," + strconv.FormatFloat(lon, 'f', 4, 64)
	hits, err := s.fetch(ctx, key, "/reverse", params, true)
	if err != nil {
		return Place{}, err
	}
	if len(hits) == 0 {
		return Place{}, fmt.Errorf("chatot/geo: nominatim: nothing at %s", key)
	}
	return hits[0], nil
}

// fetch runs one throttled, cached Nominatim request. single marks an
// endpoint that answers with one object rather than an array.
func (s *Searcher) fetch(ctx context.Context, key, endpoint string, params url.Values, single bool) ([]Place, error) {
	s.mu.Lock()
	if hits, ok := s.cache[key]; ok {
		s.mu.Unlock()
		return hits, nil
	}
	// One request per second, serialised across callers.
	if wait := time.Second - time.Since(s.last); wait > 0 {
		s.mu.Unlock()
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		s.mu.Lock()
	}
	s.last = time.Now()
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.server+endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatot/geo: nominatim: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if single {
		body = append(append([]byte{'['}, body...), ']')
	}
	hits, err := parseNominatim(body)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[key] = hits
	s.mu.Unlock()
	return hits, nil
}

// nominatimHit is the subset of a jsonv2 result chatot reads.
type nominatimHit struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

// parseNominatim turns a jsonv2 response into Places. A hit without a
// proper name uses the first component of its display name; the address
// line is the display name with that leading component removed, cut to
// its first three parts so it fits a result row.
func parseNominatim(body []byte) ([]Place, error) {
	var hits []nominatimHit
	if err := json.Unmarshal(body, &hits); err != nil {
		return nil, fmt.Errorf("chatot/geo: nominatim: %w", err)
	}
	out := make([]Place, 0, len(hits))
	for _, h := range hits {
		lat, err1 := strconv.ParseFloat(h.Lat, 64)
		lon, err2 := strconv.ParseFloat(h.Lon, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		parts := strings.Split(h.DisplayName, ", ")
		name := h.Name
		if name == "" && len(parts) > 0 {
			name = parts[0]
		}
		if len(parts) > 0 && strings.EqualFold(parts[0], name) {
			parts = parts[1:]
		}
		if len(parts) > 3 {
			parts = parts[:3]
		}
		out = append(out, Place{Name: name, Address: strings.Join(parts, ", "), Lat: lat, Lon: lon})
	}
	return out, nil
}
