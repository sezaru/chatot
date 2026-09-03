package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// loadingMarkSize is the app mark on the startup screen, in px.
const loadingMarkSize = 96

// NewLoadingView is the screen shown between launch and the first
// connection for an account that is already linked: just the app mark on
// the window background. It exists so the QR pairing page never flashes
// for a user who has nothing to pair, and the main view fades in over it
// (the window's stack crossfades) once the socket is up.
func NewLoadingView() gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-loading")
	box.SetHExpand(true)
	box.SetVExpand(true)
	box.SetHAlign(gtk.AlignCenter)
	box.SetVAlign(gtk.AlignCenter)
	box.Append(newAppMark(loadingMarkSize))
	return box
}
