package geo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseNominatim(t *testing.T) {
	body := []byte(`[{"name":"Praça da Liberdade","display_name":"Praça da Liberdade, Baixa, Porto, Portugal","lat":"41.1464565","lon":"-8.6113286"},
	{"name":"","display_name":"Rua de Sá da Bandeira 11, Porto, Portugal","lat":"41.1463196","lon":"-8.6106933"},
	{"name":"bad","display_name":"x","lat":"nope","lon":"1"}]`)
	hits, err := parseNominatim(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2 (bad coordinates dropped)", len(hits))
	}
	if hits[0].Name != "Praça da Liberdade" || hits[0].Address != "Baixa, Porto, Portugal" {
		t.Errorf("hit 0 = %+v", hits[0])
	}
	if hits[1].Name != "Rua de Sá da Bandeira 11" || hits[1].Address != "Porto, Portugal" {
		t.Errorf("hit 1 = %+v", hits[1])
	}
	if hits[0].Lat < 41.14 || hits[0].Lon > -8.6 {
		t.Errorf("hit 0 coordinates = %v,%v", hits[0].Lat, hits[0].Lon)
	}
}

func TestSenderPath(t *testing.T) {
	if got := senderPath(":1.42"); got != "1_42" {
		t.Errorf("senderPath = %q", got)
	}
}

func TestReverseUsesReverseEndpointAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/reverse" || r.URL.Query().Get("lat") == "" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		fmt.Fprint(w, `{"name":"Rua Canto das Sereias","display_name":"Rua Canto das Sereias, Florianópolis, Brasil","lat":"-27.4190","lon":"-48.4064"}`)
	}))
	defer srv.Close()
	s := NewSearcher()
	s.SetServer(srv.URL)
	p, err := s.Reverse(context.Background(), -27.41902, -48.40645)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Rua Canto das Sereias" || p.Address != "Florianópolis, Brasil" {
		t.Errorf("place = %+v", p)
	}
	if _, err := s.Reverse(context.Background(), -27.419024, -48.40645); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (10 m rounding shares the cache entry)", calls)
	}
}
