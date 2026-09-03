package ui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// showVideoFullscreen shows a clip alone over the whole screen: a black
// stage with the viewer's transport along its bottom edge and nothing else.
// It paints the viewer pane's own player (from, may be nil) rather than a
// second stream, so playback carries on at the same point going in and
// coming back. A click or Space toggles it; Esc, F, F11 and the transport's
// ⤢ close the window.
func showVideoFullscreen(parent *gtk.Window, path string, msg client.Message, from *mediaPlayer) {
	win := gtk.NewWindow()
	win.SetTitle("Video")
	win.SetDecorated(false)
	win.AddCSSClass("chatot-video-fullscreen")
	if parent != nil {
		win.SetTransientFor(parent)
	}

	seconds := 0
	var poster []byte
	if msg.Attachment != nil {
		seconds = msg.Attachment.DurationSecs
		poster = msg.Attachment.Thumbnail
	}
	p := from
	if p == nil || !p.Ready() {
		p = newMediaPlayer(path, seconds)
	}
	stage := newVideoStage(p, poster)
	stage.shared = p == from
	stage.SetHExpand(true)
	stage.SetVExpand(true)

	overlay := gtk.NewOverlay()
	overlay.SetChild(stage)
	bar, unwatchBar := newTransportBarWatched(p, func() { win.Close() })
	bar.AddCSSClass("chatot-transport-fullscreen")
	bar.SetVAlign(gtk.AlignEnd)
	overlay.AddOverlay(bar)
	win.SetChild(overlay)
	// The shared player outlives this window; its watchers must not.
	win.ConnectCloseRequest(func() bool {
		unwatchBar()
		stage.unwatch()
		return false
	})

	keys := gtk.NewEventControllerKey()
	keys.ConnectKeyPressed(func(keyval, _ uint, _ gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Escape, gdk.KEY_f, gdk.KEY_F, gdk.KEY_F11:
			win.Close()
			return true
		case gdk.KEY_space:
			p.Toggle()
			return true
		}
		return false
	})
	win.AddController(keys)
	win.Present()
	win.Fullscreen()
	// Start once the stage is on screen: a stream decodes onto its surface
	// only after the picture painting it is realized.
	glib.IdleAdd(func() {
		if !p.Playing() {
			p.Toggle()
		}
	})
}
