package ui

import (
	"context"
	"os"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// mediaView is the pure inline-vs-tap-to-load decision for one message's
// attachment, computed from a client.Message so it's unit-testable without
// a display.
type mediaView struct {
	IsMedia   bool
	Kind      string
	Chip      string
	HasLocal  bool
	LocalPath string
}

// mediaVM derives the media view-model for m. HasLocal requires both a
// stored local_path and the file actually existing on disk — a cache
// eviction (internal/media.Evict) NULLs local_path, but a stale in-memory
// copy or a manual deletion shouldn't be trusted either.
func mediaVM(m client.Message) mediaView {
	if m.Attachment == nil {
		return mediaView{}
	}
	a := *m.Attachment
	v := mediaView{IsMedia: true, Kind: a.Kind, Chip: mediaChip(a)}
	if a.LocalPath != "" {
		if info, err := os.Stat(a.LocalPath); err == nil && !info.IsDir() {
			v.HasLocal = true
			v.LocalPath = a.LocalPath
		}
	}
	return v
}

// inlineable reports whether kind renders as an embedded widget once
// cached: image/video/sticker as a Picture/Video, audio as playback
// controls. Documents stay a chip.
func inlineable(kind string) bool {
	switch kind {
	case "image", "video", "sticker", "audio":
		return true
	default:
		return false
	}
}

// buildMediaContent renders a message's attachment: an inline widget
// (Picture/Video, or MediaControls for audio) if it's cached, otherwise a
// chip. An uncached chip downloads on click (goroutine -> glib.IdleAdd back
// to the main loop) and then either swaps itself for the inline widget or
// flips to an "open" affordance (document).
func buildMediaContent(msg client.Message, mv mediaView, c client.Client) gtk.Widgetter {
	slot := gtk.NewBox(gtk.OrientationVertical, 0)

	if mv.HasLocal && inlineable(mv.Kind) {
		slot.Append(inlineMediaWidget(mv))
		return slot
	}

	state := mv
	btn := gtk.NewButtonWithLabel(chipLabel(state))
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-bubble-media")
	btn.ConnectClicked(func() { onMediaChipClicked(&state, msg, c, slot, btn) })
	slot.Append(btn)
	return slot
}

func chipLabel(mv mediaView) string {
	if mv.HasLocal {
		return mv.Chip
	}
	return "⬇ " + mv.Chip
}

func inlineMediaWidget(mv mediaView) gtk.Widgetter {
	switch mv.Kind {
	case "video":
		v := gtk.NewVideoForFilename(mv.LocalPath)
		v.SetSizeRequest(280, 200)
		return v
	case "audio":
		// gtk.MediaControls plays via its bound MediaStream but never
		// autoplays on its own, so no explicit Play()/Pause() call is needed.
		stream := gtk.NewMediaFileForFilename(mv.LocalPath)
		controls := gtk.NewMediaControls(stream)
		controls.SetSizeRequest(240, -1)
		return controls
	}
	p := gtk.NewPictureForFilename(mv.LocalPath)
	p.SetCanShrink(true)
	p.SetContentFit(gtk.ContentFitContain)
	p.SetSizeRequest(280, 200)
	return p
}

// onMediaChipClicked handles a click on the media chip: if already
// downloaded it opens the file with the desktop default app; otherwise it
// downloads in the background and applies the result on the GTK main loop.
func onMediaChipClicked(mv *mediaView, msg client.Message, c client.Client, slot *gtk.Box, btn *gtk.Button) {
	if mv.HasLocal {
		openFile(mv.LocalPath)
		return
	}
	btn.SetSensitive(false)
	go func() {
		path, err := c.DownloadMedia(context.Background(), msg.ID)
		glib.IdleAdd(func() {
			btn.SetSensitive(true)
			if err != nil {
				btn.SetLabel(mv.Chip + " (failed)")
				return
			}
			mv.HasLocal = true
			mv.LocalPath = path
			if inlineable(mv.Kind) {
				slot.Remove(btn)
				slot.Append(inlineMediaWidget(*mv))
				return
			}
			btn.SetLabel(chipLabel(*mv))
		})
	}()
}

// openFile launches path with the desktop's default application for it.
func openFile(path string) {
	launcher := gtk.NewFileLauncher(gio.NewFileForPath(path))
	launcher.Launch(context.Background(), nil, func(res gio.AsyncResulter) {
		_ = launcher.LaunchFinish(res)
	})
}
