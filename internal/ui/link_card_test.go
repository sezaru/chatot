package ui

import (
	"strings"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// The card is a tile of one width, like the video tile it is sized to line
// up with. When its minimum and its natural width disagree, the bubble
// around it has a range to negotiate over rather than a width to take, and
// a marketplace listing — a title and a description both far longer than
// the card — settled on a bubble half the pane wide with the card's
// picture stretched across it and dead space under the footer.
func TestBuildLinkCard_IsOneFixedWidth(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	cases := map[string]linkView{
		"bare": {URL: "https://example.com/a", Title: "A"},
		"long": {
			URL:         "https://shop.example.com/" + strings.Repeat("a-long-slug/", 8),
			Title:       "Cabeça De Chuveiro De 4 Polegadas De 5 Velocidades Com Filtr - R$ 78,44",
			Description: strings.Repeat("Filtro multistágio remove cloro e metais, garantindo água pura. | ", 6),
			Host:        "shop.example.com",
		},
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			min, nat, _, _ := gtk.BaseWidget(buildLinkCard(v)).Measure(gtk.OrientationHorizontal, -1)
			if min != linkCardW || nat != linkCardW {
				t.Errorf("card width = %d/%d (min/natural), want %d for both", min, nat, linkCardW)
			}
		})
	}
}

// A bubble holding a link card is that card's width: the text under it
// wraps to the card instead of reaching past it and dragging the card wide.
func TestLinkBubble_TakesTheCardsWidth(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	r := newThreadRow()
	long := "Tem esse outro anuncio tb https://shop.example.com/" +
		strings.Repeat("cabeca-de-chuveiro-de-5-velocidades/", 6) + "?wid=MLB4490745411&sid=recos"
	vm := bubbleView{
		Text: long,
		Link: &linkView{
			URL: "https://shop.example.com/p/1", Title: "Cabeça De Chuveiro De 4 Polegadas De 5 Velocidades",
			Description: strings.Repeat("Filtro multistágio remove cloro e metais. | ", 6),
			Host:        "shop.example.com",
		},
	}
	r.render(client.Message{ID: "m", Text: long}, vm, bubbleHooks{})

	min, nat, _, _ := gtk.BaseWidget(r.bubble).Measure(gtk.OrientationHorizontal, -1)
	if min != nat {
		t.Errorf("bubble width = %d/%d (min/natural); a link bubble has one width, not a range", min, nat)
	}
	if nat < linkCardW || nat > linkCardW+40 {
		t.Errorf("bubble natural = %d, want the card's %d plus its padding", nat, linkCardW)
	}
}
