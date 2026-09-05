package gif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const giphySample = `{"data":[
 {"id":"a","images":{"fixed_height_small":{"url":"https://g/a-s.gif","width":"178","height":"100"},"downsized_small":{"mp4":"https://g/a-ds.mp4","width":"320","height":"180"},"original":{"mp4":"https://g/a.mp4","width":"480","height":"270"}}},
 {"id":"b","images":{"preview_gif":{"url":"https://g/b-p.gif"},"fixed_height":{"url":"https://g/b.gif","mp4":"https://g/b.mp4","width":"356","height":"200"}}},
 {"id":"c","images":{"original":{"url":"https://g/c.gif"}}}
],"meta":{"status":200}}`

func TestParseGiphyPicksPreviewAndSmallestMP4(t *testing.T) {
	got, err := ParseGiphy([]byte(giphySample))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("ParseGiphy = %+v, want 2 (the third has no mp4)", got)
	}
	if got[0].PreviewURL != "https://g/a-s.gif" || got[0].MP4URL != "https://g/a-ds.mp4" || got[0].Width != 320 || got[0].Height != 180 {
		t.Errorf("result a = %+v", got[0])
	}
	if got[1].PreviewURL != "https://g/b-p.gif" || got[1].MP4URL != "https://g/b.mp4" {
		t.Errorf("result b = %+v", got[1])
	}
}

func TestGiphySearchTrendingWithoutQueryAndErrors(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?q="+r.URL.Query().Get("q")+"&api_key="+r.URL.Query().Get("api_key"))
		if r.URL.Query().Get("api_key") == "bad" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"data":[],"meta":{"status":403,"msg":"Invalid authentication credentials"}}`))
			return
		}
		w.Write([]byte(giphySample))
	}))
	defer srv.Close()

	g := NewGiphy("")
	g.Base = srv.URL
	if _, err := g.Search(context.Background(), "cats", 8); err != ErrNoKey {
		t.Errorf("Search without key = %v, want ErrNoKey", err)
	}
	g.Key = "k"
	if got, err := g.Search(context.Background(), "", 8); err != nil || len(got) != 2 {
		t.Fatalf("trending = %+v, %v", got, err)
	}
	if _, err := g.Search(context.Background(), "cats", 8); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/gifs/trending?q=&api_key=k" || paths[1] != "/gifs/search?q=cats&api_key=k" {
		t.Errorf("requests = %v", paths)
	}
	g.Key = "bad"
	if _, err := g.Search(context.Background(), "x", 8); err == nil || err.Error() != "gif: giphy: Invalid authentication credentials" {
		t.Errorf("bad key error = %v", err)
	}
}
