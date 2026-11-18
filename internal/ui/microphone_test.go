package ui

import (
	"reflect"
	"testing"

	"chatot/internal/audio"
)

func TestMicrophoneMenuAndLabel(t *testing.T) {
	srcs := []audio.Source{
		{Name: "effect_output.rnnoise", Description: "Noise Canceling source", Default: true},
		{Name: "effect_output.j493-mic", Description: "MacBook Pro J493 Microphone"},
		{Name: "alsa_input.usb", Description: "A very long microphone description that runs on"},
	}
	var picked []string
	items := microphoneMenu(srcs, func(name string) { picked = append(picked, name) })
	want := []string{
		"System default · Noise Canceling source",
		"Noise Canceling source",
		"MacBook Pro J493 Microphone",
		"A very long microphone descript…",
	}
	if got := menuItemLabels(items); !reflect.DeepEqual(got, want) {
		t.Errorf("menu labels = %q, want %q", got, want)
	}
	for _, it := range items {
		if it.OnActivate != nil {
			it.OnActivate()
		}
	}
	if want := []string{"", "effect_output.rnnoise", "effect_output.j493-mic", "alsa_input.usb"}; !reflect.DeepEqual(picked, want) {
		t.Errorf("picks = %q, want %q", picked, want)
	}

	for name, want := range map[string]string{
		"":                       "System default",
		"effect_output.j493-mic": "MacBook Pro J493 Microphone",
		"alsa_input.unplugged":   "alsa_input.unplugged",
	} {
		if got := microphoneLabel(name, srcs); got != want {
			t.Errorf("microphoneLabel(%q) = %q, want %q", name, got, want)
		}
	}
	// No sources known (the list failed): the menu still offers the default.
	if got := menuItemLabels(microphoneMenu(nil, func(string) {})); !reflect.DeepEqual(got, []string{"System default"}) {
		t.Errorf("menu with no sources = %q", got)
	}
}
