package linkpreview

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestFirstURL(t *testing.T) {
	cases := map[string]string{
		"look https://x.com/a/status/1.":          "https://x.com/a/status/1",
		"(www.example.org/p?q=1)":                 "www.example.org/p?q=1",
		"no link here":                            "",
		"two https://a.example https://b.example": "https://a.example",
	}
	for in, want := range cases {
		if got := FirstURL(in); got != want {
			t.Errorf("FirstURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOpenGraph(t *testing.T) {
	base, _ := url.Parse("https://m.youtube.com/watch?v=1")
	body := []byte(`<html><head><title>YouTube</title>
		<meta property="og:title" content="216,000,000 Spy TVs &amp; more">
		<meta content="SUPPORT THE SERIES" property="og:description"/>
		<meta property='og:image' content='/vi/1/max.jpg'>
		<meta property="og:url" content="https://www.youtube.com/watch?v=1">
		</head></html>`)
	p, img := Parse(body, base)
	if p.Title != "216,000,000 Spy TVs & more" || p.Description != "SUPPORT THE SERIES" {
		t.Fatalf("got %+v", p)
	}
	if img != "https://m.youtube.com/vi/1/max.jpg" {
		t.Fatalf("image = %q", img)
	}
	if p.Host() != "www.youtube.com" {
		t.Fatalf("host = %q", p.Host())
	}
}

func TestParseFallsBackToTitleTag(t *testing.T) {
	p, img := Parse([]byte("<html><head><title>\n  Plain   page </title><meta name=\"description\" content=\"About it\"></head></html>"), nil)
	if p.Title != "Plain page" || p.Description != "About it" || img != "" {
		t.Fatalf("got %+v %q", p, img)
	}
}

func TestThumbnailShrinks(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x / 5), G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	out, err := Thumbnail(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != ThumbMaxSide || b.Dy() != 225 {
		t.Fatalf("thumbnail is %v, want 400×225", b)
	}
}

func TestFetchEndToEnd(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 20, 10)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/post":
			if r.Header.Get("User-Agent") != userAgent {
				t.Errorf("user agent %q", r.Header.Get("User-Agent"))
			}
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<meta property="og:title" content="A post"><meta property="og:image" content="/pic.png">`))
		case "/pic.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(buf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p, err := Fetch(t.Context(), srv.URL+"/post", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "A post" || p.URL != srv.URL+"/post" || len(p.Thumbnail) == 0 {
		t.Fatalf("got %+v", p)
	}
	if _, err := Fetch(t.Context(), srv.URL+"/missing", nil); err == nil {
		t.Fatal("a 404 should be an error")
	}
}
