package gif

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sample = `{"results":[
 {"id":"1","media_formats":{"tinygif":{"url":"https://t/1.gif","dims":[220,124]},"mp4":{"url":"https://t/1.mp4","dims":[498,280]}}},
 {"id":"2","media_formats":{"tinygif":{"url":"https://t/2.gif","dims":[220,220]},"tinymp4":{"url":"https://t/2s.mp4","dims":[320,320]}}},
 {"id":"3","media_formats":{"gif":{"url":"https://t/3.gif","dims":[1,1]}}}
]}`

func TestParseTenorPicksPreviewAndMP4(t *testing.T) {
	got, err := ParseTenor([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("ParseTenor = %+v, want 2 results (the third has no mp4)", got)
	}
	if got[0].MP4URL != "https://t/1.mp4" || got[0].Width != 498 || got[0].Height != 280 || got[0].PreviewURL != "https://t/1.gif" {
		t.Errorf("result 1 = %+v", got[0])
	}
	if got[1].MP4URL != "https://t/2s.mp4" {
		t.Errorf("result 2 should fall back to tinymp4, got %+v", got[1])
	}
}

func TestSearchNeedsKeyAndHitsFeaturedWithoutQuery(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?q="+r.URL.Query().Get("q")+"&key="+r.URL.Query().Get("key"))
		w.Write([]byte(sample))
	}))
	defer srv.Close()

	tn := NewTenor("")
	tn.Base = srv.URL
	if _, err := tn.Search(context.Background(), "cats", 8); !errors.Is(err, ErrNoKey) {
		t.Errorf("Search without key = %v, want ErrNoKey", err)
	}
	tn.Key = "k"
	if _, err := tn.Search(context.Background(), "", 8); err != nil {
		t.Fatal(err)
	}
	if got, err := tn.Search(context.Background(), "cats", 8); err != nil || len(got) != 2 {
		t.Fatalf("Search = %+v, %v", got, err)
	}
	if len(paths) != 2 || paths[0] != "/featured?q=&key=k" || paths[1] != "/search?q=cats&key=k" {
		t.Errorf("requests = %v", paths)
	}
}

func TestSearchReportsTenorError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"API key not valid"}}`))
	}))
	defer srv.Close()
	tn := NewTenor("bad")
	tn.Base = srv.URL
	_, err := tn.Search(context.Background(), "x", 8)
	if err == nil || err.Error() != "gif: tenor: API key not valid" {
		t.Errorf("Search error = %v", err)
	}
}
