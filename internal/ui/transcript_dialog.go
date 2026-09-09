package ui

import (
	"context"
	"errors"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/transcribe"
)

// modelDownload is the one speech-model download in flight, so a second
// Transcribe while it runs waits for it instead of starting another.
var modelDownload struct {
	running bool
	waiters []func()
}

// showModelDownloadDialog asks before fetching the speech model (it is a
// large download, not something to start on a click that only meant
// "transcribe"), then fetches it with a progress bar. onReady, if set,
// runs once the model is in place; a download already under way takes it
// as another waiter.
func showModelDownloadDialog(parent *gtk.Window, onReady func()) {
	if modelDownload.running {
		if onReady != nil {
			modelDownload.waiters = append(modelDownload.waiters, onReady)
		}
		return
	}
	body := "Voice notes are transcribed on this computer by whisper.cpp. Its speech " +
		"model is downloaded once, from Hugging Face, into chatot's cache."
	dialog := adw.NewAlertDialog("Download the speech model?", body)
	dialog.SetExtraChild(newModelFactsRow())
	dialog.AddResponse("cancel", "Not now")
	dialog.AddResponse("download", "Download")
	dialog.SetResponseAppearance("download", adw.ResponseSuggested)
	dialog.SetDefaultResponse("download")
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response == "download" {
			runModelDownload(parent, onReady)
		}
	})
	dialog.Present(parent)
}

// newModelFactsRow names what is about to be downloaded — the file, what
// runs it, and how big it is — so the size in the prompt belongs to
// something the reader can see rather than to an unnamed "model".
func newModelFactsRow() gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.AddCSSClass("chatot-model-facts")

	glyph := gtk.NewLabel("🎙")
	glyph.AddCSSClass("chatot-model-facts-glyph")
	glyph.SetVAlign(gtk.AlignCenter)
	row.Append(glyph)

	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetHExpand(true)
	name := gtk.NewLabel(transcribe.ModelName)
	name.AddCSSClass("chatot-model-facts-name")
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeMiddle)
	col.Append(name)
	sub := gtk.NewLabel("whisper.cpp · multilingual · runs offline")
	sub.AddCSSClass("chatot-model-facts-sub")
	sub.SetXAlign(0)
	col.Append(sub)
	row.Append(col)

	size := gtk.NewLabel(humanSize(transcribe.ModelSize))
	size.AddCSSClass("chatot-model-facts-size")
	size.SetVAlign(gtk.AlignCenter)
	row.Append(size)
	return row
}

// runModelDownload fetches the model behind a progress dialog whose only
// button cancels it. The waiters run when the file is in place.
func runModelDownload(parent *gtk.Window, onReady func()) {
	modelDownload.running = true
	if onReady != nil {
		modelDownload.waiters = append(modelDownload.waiters, onReady)
	}
	ctx, cancel := context.WithCancel(context.Background())

	bar := gtk.NewProgressBar()
	bar.SetShowText(true)
	bar.SetText("Starting…")
	bar.SetMarginTop(8)
	dialog := adw.NewAlertDialog("Downloading the speech model", "")
	dialog.SetExtraChild(bar)
	dialog.AddResponse("cancel", "Cancel")
	dialog.SetCloseResponse("cancel")
	done := false
	dialog.ConnectResponse(func(string) {
		if !done {
			cancel()
		}
	})
	dialog.Present(parent)

	go func() {
		err := transcribe.DownloadModel(ctx, cacheDir(), func(got, total int64) {
			glib.IdleAdd(func() {
				if total > 0 {
					bar.SetFraction(float64(got) / float64(total))
					bar.SetText(humanSize(got) + " of " + humanSize(total))
				} else {
					bar.Pulse()
					bar.SetText(humanSize(got))
				}
			})
		})
		cancel()
		glib.IdleAdd(func() {
			done = true
			modelDownload.running = false
			waiters := modelDownload.waiters
			modelDownload.waiters = nil
			dialog.ForceClose()
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					showToast(Prefs.Toasts, "Speech model download failed: "+err.Error())
				}
				return
			}
			for _, w := range waiters {
				w()
			}
		})
	}()
}
