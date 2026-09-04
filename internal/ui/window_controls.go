package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// ShowWindowControls mirrors settings.ShowWindowControls: whether the
// header strips draw the desktop's window buttons at all.
var ShowWindowControls = true

// windowControls are every GtkWindowControls the headers made, so the
// Preferences switch can hide or show them together.
var windowControls []*gtk.WindowControls

// newWindowControls builds a controls widget for side that follows both
// the desktop's gtk-decoration-layout (an empty side stays hidden, so it
// costs no box spacing and GTK never warns about a negative minimum width)
// and the ShowWindowControls preference.
func newWindowControls(side gtk.PackType) *gtk.WindowControls {
	wc := gtk.NewWindowControls(side)
	wc.SetVAlign(gtk.AlignCenter)
	windowControls = append(windowControls, wc)
	sync := func() { wc.SetVisible(ShowWindowControls && !wc.Empty()) }
	sync()
	wc.NotifyProperty("empty", sync)
	return wc
}

// ApplyWindowControls shows or hides every header's window buttons.
func ApplyWindowControls(show bool) {
	ShowWindowControls = show
	for _, wc := range windowControls {
		wc.SetVisible(show && !wc.Empty())
	}
}
