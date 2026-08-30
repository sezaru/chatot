package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestLocationVM_Named(t *testing.T) {
	m := client.Message{Location: &client.Location{
		Name: "Bletchley Park", Address: "Sherwood Dr", Latitude: 51.9976, Longitude: -0.7406,
	}}
	v := locationVM(m)
	if !v.IsLocation {
		t.Fatal("expected IsLocation=true")
	}
	if v.Title != "Bletchley Park" {
		// Title carries just the name; the pin is prepended at render time.
		t.Errorf("Title = %q, want %q", v.Title, "Bletchley Park")
	}
	if v.Address != "Sherwood Dr" {
		t.Errorf("Address = %q", v.Address)
	}
	if v.Coords != "51.9976, -0.7406" {
		t.Errorf("Coords = %q, want %q", v.Coords, "51.9976, -0.7406")
	}
}

func TestLocationVM_UnnamedFallback(t *testing.T) {
	pin := locationVM(client.Message{Location: &client.Location{Latitude: 1, Longitude: 2}})
	if pin.Title != "Location" {
		t.Errorf("Title = %q, want Location", pin.Title)
	}
	live := locationVM(client.Message{Location: &client.Location{Latitude: 1, Longitude: 2, IsLive: true}})
	if live.Title != "Live location" {
		t.Errorf("Title = %q, want Live location", live.Title)
	}
}

func TestLocationVM_NoLocation(t *testing.T) {
	if locationVM(client.Message{Text: "hi"}).IsLocation {
		t.Error("expected IsLocation=false for a non-location message")
	}
}

func TestMapsURL(t *testing.T) {
	got := mapsURL(51.9976, -0.7406)
	want := "https://www.openstreetmap.org/?mlat=51.9976&mlon=-0.7406#map=16/51.9976/-0.7406"
	if got != want {
		t.Errorf("mapsURL = %q, want %q", got, want)
	}
}

func TestBubbleVM_LocationMessage(t *testing.T) {
	m := client.Message{ID: "1", Location: &client.Location{Name: "Home", Latitude: 1.5, Longitude: 2.5}}
	out := bubbleVM(m, nil, nil, mustParse(t, "2026-08-30 12:00:00"))
	if !out.IsLocation {
		t.Fatal("expected IsLocation=true")
	}
	if out.IsMedia {
		t.Error("expected IsMedia=false for a location message")
	}
	if out.Location.Title != "Home" {
		t.Errorf("Location.Title = %q, want Home", out.Location.Title)
	}
}
