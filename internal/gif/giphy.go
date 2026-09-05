package gif

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Provider is a GIF search service: Giphy or Tenor.
type Provider interface {
	// Search returns up to limit GIFs for q; an empty q lists what the
	// service is featuring, what the picker shows before the user types.
	Search(ctx context.Context, q string, limit int) ([]Result, error)
}

// Giphy talks to Giphy's v1 API with an app key (free from
// developers.giphy.com).
type Giphy struct {
	Key    string
	Client *http.Client
	// Base is the API root; tests point it at a local server.
	Base string
}

// NewGiphy returns a client for key with sensible timeouts.
func NewGiphy(key string) *Giphy {
	return &Giphy{
		Key:    key,
		Client: &http.Client{Timeout: 10 * time.Second},
		Base:   "https://api.giphy.com/v1",
	}
}

// Search implements Provider; an empty q lists the trending GIFs.
func (g *Giphy) Search(ctx context.Context, q string, limit int) ([]Result, error) {
	if g.Key == "" {
		return nil, ErrNoKey
	}
	params := url.Values{
		"api_key": {g.Key},
		"limit":   {strconv.Itoa(limit)},
		"rating":  {"pg-13"},
	}
	endpoint := g.Base + "/gifs/trending"
	if q != "" {
		endpoint = g.Base + "/gifs/search"
		params.Set("q", q)
	}
	body, status, err := get(ctx, g.Client, endpoint+"?"+params.Encode())
	if err != nil {
		return nil, fmt.Errorf("gif: giphy: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gif: giphy: %s", giphyError(status, body))
	}
	return ParseGiphy(body)
}

// giphyError digs the message out of Giphy's meta block, else names the
// status.
func giphyError(status int, body []byte) string {
	var e struct {
		Meta struct {
			Msg string `json:"msg"`
		} `json:"meta"`
	}
	if json.Unmarshal(body, &e) == nil && e.Meta.Msg != "" {
		return e.Meta.Msg
	}
	return "HTTP " + strconv.Itoa(status)
}

// giphyImage is one rendition; Giphy reports its size as strings.
type giphyImage struct {
	URL    string `json:"url"`
	MP4    string `json:"mp4"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

type giphyResponse struct {
	Data []struct {
		ID     string                `json:"id"`
		Images map[string]giphyImage `json:"images"`
	} `json:"data"`
}

// ParseGiphy reads a Giphy search/trending body. The preview is the small
// fixed-height GIF; the mp4 sent is the smallest decent rendition Giphy
// offers (downsized_small, then fixed_height, then original). A result
// missing either is skipped.
func ParseGiphy(body []byte) ([]Result, error) {
	var gr giphyResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("gif: giphy: %w", err)
	}
	out := make([]Result, 0, len(gr.Data))
	for _, d := range gr.Data {
		preview := d.Images["fixed_height_small"].URL
		if preview == "" {
			preview = d.Images["preview_gif"].URL
		}
		var mp4 giphyImage
		for _, name := range []string{"downsized_small", "fixed_height", "original"} {
			if img, ok := d.Images[name]; ok && img.MP4 != "" {
				mp4 = img
				break
			}
		}
		if preview == "" || mp4.MP4 == "" {
			continue
		}
		w, _ := strconv.Atoi(mp4.Width)
		h, _ := strconv.Atoi(mp4.Height)
		out = append(out, Result{ID: d.ID, PreviewURL: preview, MP4URL: mp4.MP4, Width: w, Height: h})
	}
	return out, nil
}

// get performs one GET and returns the body (capped) with the status.
func get(ctx context.Context, client *http.Client, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

// Fetch downloads u (a preview or an mp4) with a plain client.
func Fetch(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gif: fetch %s: HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

var fetchClient = &http.Client{Timeout: 60 * time.Second}
