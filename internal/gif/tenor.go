// Package gif searches Giphy or Tenor for GIFs. WhatsApp sends a GIF as a short mp4
// flagged for looped playback, which is the form Tenor hands out too, so a
// pick needs no conversion: the mp4 is downloaded and sent as is.
package gif

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrNoKey is returned by Search when the client has no API key.
var ErrNoKey = errors.New("gif: no Tenor API key")

// Result is one GIF: a small animated preview for the picker tile and the
// mp4 that gets sent, with its pixel size.
type Result struct {
	ID         string
	PreviewURL string
	MP4URL     string
	Width      int
	Height     int
}

// Tenor talks to Tenor's v2 API. Tenor stopped issuing keys in January
// 2026, so this only serves people who already hold one; Giphy is the
// default.
type Tenor struct {
	Key    string
	Client *http.Client
	// Base is the API root; tests point it at a local server.
	Base string
	// ClientKey names this integration to Tenor.
	ClientKey string
}

// NewTenor returns a client for key with sensible timeouts.
func NewTenor(key string) *Tenor {
	return &Tenor{
		Key:       key,
		Client:    &http.Client{Timeout: 10 * time.Second},
		Base:      "https://tenor.googleapis.com/v2",
		ClientKey: "chatot",
	}
}

// Search returns up to limit GIFs for q; an empty q lists Tenor's current
// featured GIFs, what the picker shows before the user types.
func (t *Tenor) Search(ctx context.Context, q string, limit int) ([]Result, error) {
	if t.Key == "" {
		return nil, ErrNoKey
	}
	params := url.Values{
		"key":           {t.Key},
		"client_key":    {t.ClientKey},
		"limit":         {strconv.Itoa(limit)},
		"media_filter":  {"tinygif,mp4,tinymp4"},
		"contentfilter": {"medium"},
	}
	endpoint := t.Base + "/featured"
	if q != "" {
		endpoint = t.Base + "/search"
		params.Set("q", q)
	}
	body, status, err := get(ctx, t.Client, endpoint+"?"+params.Encode())
	if err != nil {
		return nil, fmt.Errorf("gif: tenor: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gif: tenor: %s", tenorError(status, body))
	}
	return ParseTenor(body)
}

// tenorError digs the message out of Tenor's JSON error body, else names
// the status.
func tenorError(status int, body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return "HTTP " + strconv.Itoa(status)
}

type tenorFormat struct {
	URL  string `json:"url"`
	Dims []int  `json:"dims"`
}

type tenorResponse struct {
	Results []struct {
		ID      string                 `json:"id"`
		Formats map[string]tenorFormat `json:"media_formats"`
	} `json:"results"`
}

// ParseTenor reads a Tenor v2 search/featured body. A result without a
// preview or an mp4 is skipped; the mp4's size is the one reported.
func ParseTenor(body []byte) ([]Result, error) {
	var tr tenorResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("gif: tenor: %w", err)
	}
	out := make([]Result, 0, len(tr.Results))
	for _, r := range tr.Results {
		mp4, ok := r.Formats["mp4"]
		if !ok || mp4.URL == "" {
			mp4, ok = r.Formats["tinymp4"]
		}
		preview := r.Formats["tinygif"]
		if !ok || mp4.URL == "" || preview.URL == "" {
			continue
		}
		res := Result{ID: r.ID, PreviewURL: preview.URL, MP4URL: mp4.URL}
		if len(mp4.Dims) == 2 {
			res.Width, res.Height = mp4.Dims[0], mp4.Dims[1]
		}
		out = append(out, res)
	}
	return out, nil
}
