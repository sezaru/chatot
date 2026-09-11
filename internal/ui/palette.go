package ui

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The cairo-drawn bits (status rings, voice tracks, poll bars, the map
// marker, tab icons) cannot read the stylesheet, so they ask for the same
// @define-color tokens the sheet uses, through the widget they draw on;
// a shell palette that restates a token (theme_dms.go) restyles them too.
// The fallbacks are the light sheet's values, for a token the stack does
// not define.

// accentRGBA is chatot_accent, the brand green fill.
var accentRGBA = [4]float64{0x1b / 255.0, 0x8c / 255.0, 0x72 / 255.0, 1}

// whiteRGBA is chatot_on_accent and chatot_on_bubble_out.
var whiteRGBA = [4]float64{1, 1, 1, 1}

// tokenRGBA returns the colour token name resolves to on w, or fallback
// when no sheet in the stack defines it.
func tokenRGBA(w gtk.Widgetter, name string, fallback [4]float64) [4]float64 {
	if w == nil {
		return fallback
	}
	c, ok := gtk.BaseWidget(w).StyleContext().LookupColor(name)
	if !ok || c == nil {
		return fallback
	}
	return [4]float64{float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha())}
}

// setSourceRGBA sets cr's source to c with its alpha scaled by alpha.
func setSourceRGBA(cr *cairo.Context, c [4]float64, alpha float64) {
	cr.SetSourceRGBA(c[0], c[1], c[2], c[3]*alpha)
}

// tokenHex returns the token as "#rrggbb" for Pango markup, resolved on a
// probe widget that sees the display's providers, or fallback.
func tokenHex(name, fallback string) string {
	if gdk.DisplayGetDefault() == nil {
		return fallback // no GTK (tests)
	}
	if tokenProbe == nil {
		tokenProbe = gtk.NewLabel("")
	}
	c, ok := tokenProbe.StyleContext().LookupColor(name)
	if !ok || c == nil {
		return fallback
	}
	return fmt.Sprintf("#%02x%02x%02x", byte(c.Red()*255+0.5), byte(c.Green()*255+0.5), byte(c.Blue()*255+0.5))
}

// tokenProbe is the unrooted widget tokenHex resolves colours on.
var tokenProbe *gtk.Label

// onBubbleOutHex is the outgoing bubble's text colour for Pango markup.
func onBubbleOutHex() string { return tokenHex("chatot_on_bubble_out", "#ffffff") }
