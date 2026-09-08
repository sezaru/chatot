// Package linkpreview fetches what a chat shows under a pasted link: the
// page's Open Graph title, description and a thumbnail of its image, the
// way WhatsApp attaches them to the text message it sends.
package linkpreview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	// The image formats a page's og:image commonly comes in.
	_ "image/gif"
	_ "image/png"
)

// Preview is a link's card: the page's title and description and a small
// JPEG of its image, plus the link itself as it was written.
type Preview struct {
	URL          string // the link as written in the message
	CanonicalURL string // the page's own og:url, "" when it names none
	Title        string
	Description  string
	Thumbnail    []byte // JPEG, ThumbMaxSide at most on its longer edge; nil without an image
}

// Host is the site the card names under the title ("m.youtube.com").
func (p *Preview) Host() string {
	return URLHost(firstNonEmpty(p.CanonicalURL, p.URL))
}

// URLHost is rawURL's host, "" when it has none. A "www." link is read as
// https.
func URLHost(rawURL string) string {
	u, err := url.Parse(normalize(rawURL))
	if err != nil {
		return ""
	}
	return u.Host
}

var urlRE = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>"']+`)

// FirstURL is the first http(s) or www link in text, "" when there is
// none. Punctuation that closes the sentence around it is not part of it.
func FirstURL(text string) string {
	return strings.TrimRight(urlRE.FindString(text), ".,;:!?)]")
}

// normalize gives a bare "www." link its scheme.
func normalize(rawURL string) string {
	if strings.HasPrefix(strings.ToLower(rawURL), "www.") {
		return "https://" + rawURL
	}
	return rawURL
}

const (
	// userAgent is WhatsApp's own fetcher: sites that gate their Open Graph
	// tags behind a known crawler (X among them) serve it.
	userAgent = "WhatsApp/2.23.20.0"
	maxHTML   = 1 << 20
	maxImage  = 8 << 20
	// ThumbMaxSide caps the thumbnail's longer edge: enough for a bubble's
	// card at a scaled display, small enough to ride inside the message.
	ThumbMaxSide = 400
	thumbQuality = 72
)

// ErrNoPreview is Fetch's answer for a page with nothing to show: no
// title at all, or not an HTML page.
var ErrNoPreview = errors.New("linkpreview: page has no preview")

// Fetch loads rawURL with client (nil for the default one), reads the
// page's metadata and, when it names an image, fetches and shrinks that
// too. The image failing is not an error: the card still has its text.
func Fetch(ctx context.Context, rawURL string, client *http.Client) (*Preview, error) {
	if client == nil {
		client = http.DefaultClient
	}
	pageURL := normalize(rawURL)
	body, finalURL, err := get(ctx, client, pageURL, maxHTML, "text/html,application/xhtml+xml")
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(finalURL)
	if err != nil {
		base, _ = url.Parse(pageURL)
	}
	p, imageURL := Parse(body, base)
	if p.Title == "" {
		return nil, ErrNoPreview
	}
	p.URL = rawURL
	if imageURL != "" {
		if img, _, err := get(ctx, client, imageURL, maxImage, "image/*"); err == nil {
			p.Thumbnail, _ = Thumbnail(img)
		}
	}
	return p, nil
}

// get is one GET with the fetcher's identity, at most limit bytes of the
// body, and the URL the response came from after redirects.
func get(ctx context.Context, client *http.Client, rawURL string, limit int64, accept string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en")
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", fmt.Errorf("linkpreview: %s: %s", rawURL, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit))
	if err != nil {
		return nil, "", err
	}
	return body, res.Request.URL.String(), nil
}

var (
	metaRE  = regexp.MustCompile(`(?is)<meta\s+([^>]*?)/?>`)
	attrRE  = regexp.MustCompile(`(?is)([\w:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
	titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	spaceRE = regexp.MustCompile(`\s+`)
)

// Parse reads a page's card out of its HTML: Open Graph tags first,
// Twitter's then the plain <title> and description as fallbacks. imageURL
// is the page's image, resolved against base, "" when it names none.
func Parse(body []byte, base *url.URL) (p *Preview, imageURL string) {
	p = &Preview{}
	meta := map[string]string{}
	for _, m := range metaRE.FindAllSubmatch(body, -1) {
		var key, content string
		for _, a := range attrRE.FindAllSubmatch(m[1], -1) {
			name := strings.ToLower(string(a[1]))
			val := string(a[2]) + string(a[3]) + string(a[4])
			switch name {
			case "property", "name":
				if key == "" {
					key = strings.ToLower(val)
				}
			case "content":
				content = val
			}
		}
		if key != "" && content != "" {
			if _, seen := meta[key]; !seen {
				meta[key] = clean(content)
			}
		}
	}
	p.Title = firstNonEmpty(meta["og:title"], meta["twitter:title"])
	if p.Title == "" {
		if m := titleRE.FindSubmatch(body); m != nil {
			p.Title = clean(string(m[1]))
		}
	}
	p.Description = firstNonEmpty(meta["og:description"], meta["twitter:description"], meta["description"])
	p.CanonicalURL = meta["og:url"]
	if img := firstNonEmpty(meta["og:image:secure_url"], meta["og:image"], meta["twitter:image"], meta["twitter:image:src"]); img != "" {
		if u, err := url.Parse(img); err == nil {
			if base != nil {
				u = base.ResolveReference(u)
			}
			if u.Scheme == "http" || u.Scheme == "https" {
				imageURL = u.String()
			}
		}
	}
	return p, imageURL
}

// clean is a tag's text as one line of plain text.
func clean(s string) string {
	return strings.TrimSpace(spaceRE.ReplaceAllString(html.UnescapeString(s), " "))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Thumbnail decodes an image and re-encodes it as a JPEG no larger than
// ThumbMaxSide on its longer edge.
func Thumbnail(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, shrink(img, ThumbMaxSide), &jpeg.Options{Quality: thumbQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// shrink scales src down so its longer edge is at most maxSide, averaging
// the source pixels behind each output pixel (a box filter: no ringing,
// and no dependency for one downscale). A small image is returned as is.
func shrink(src image.Image, maxSide int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if w <= maxSide && h <= maxSide {
		return src
	}
	nw, nh := maxSide, maxSide
	if w >= h {
		nh = max(1, h*maxSide/w)
	} else {
		nw = max(1, w*maxSide/h)
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy0, sy1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < nw; x++ {
			sx0, sx1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					pr, pg, pb, pa := src.At(sx, sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					bl += uint64(pb)
					a += uint64(pa)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(r / n >> 8), G: uint8(g / n >> 8), B: uint8(bl / n >> 8), A: uint8(a / n >> 8),
			})
		}
	}
	return dst
}
