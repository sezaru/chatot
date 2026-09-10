package ui

import (
	"context"
	"errors"
	"fmt"

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
	dialog := adw.NewAlertDialog("Download the speech model?", modelDownloadBody)
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

// modelDownloadBody says what the download is and where it goes. The
// progress dialog keeps it: a download that has started still has to answer
// "what is this and where is it going", and dropping the sentence at the
// moment the bar appears would take the answer away.
const modelDownloadBody = "Voice notes are transcribed on this computer by whisper.cpp. Its speech " +
	"model is downloaded once, from Hugging Face, into chatot's cache."

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

	size := gtk.NewLabel(modelSizeText(transcribe.ModelSize))
	size.AddCSSClass("chatot-model-facts-size")
	size.SetVAlign(gtk.AlignCenter)
	row.Append(size)
	return row
}

// modelSizeText is the model's size in whole megabytes, which is how both
// the facts row and the counter below the bar say it: one file named twice
// in one dialog must not be 181.3 MB in one line and 181 MB in the next.
func modelSizeText(n int64) string {
	return fmt.Sprintf("%d MB", n/(1024*1024))
}

// modelProgressText is the byte count beside the bar, in whole megabytes of
// the whole, the way the mockup counts it: a figure that changes once a
// second reads better than one flickering through decimals. Without a
// length to count towards it is what has arrived so far.
func modelProgressText(got, total int64) string {
	if total <= 0 {
		return modelSizeText(got)
	}
	return fmt.Sprintf("%d / %s", got/(1024*1024), modelSizeText(total))
}

// newModelProgress is the download in progress under the facts row: the
// thin accent bar the mockup draws, with what is happening on one side of
// the line below it and how far it has got on the other.
func newModelProgress() (box *gtk.Box, bar *gtk.ProgressBar, count *gtk.Label) {
	box = gtk.NewBox(gtk.OrientationVertical, 6)
	box.AddCSSClass("chatot-model-progress")
	bar = gtk.NewProgressBar()
	bar.AddCSSClass("chatot-model-bar")
	box.Append(bar)
	line := gtk.NewBox(gtk.OrientationHorizontal, 8)
	status := gtk.NewLabel("Downloading once")
	status.AddCSSClass("chatot-model-status")
	status.SetXAlign(0)
	status.SetHExpand(true)
	line.Append(status)
	count = gtk.NewLabel(modelProgressText(0, transcribe.ModelSize))
	count.AddCSSClass("chatot-model-bytes")
	count.SetXAlign(1)
	line.Append(count)
	box.Append(line)
	return box, bar, count
}

// runModelDownload fetches the model behind a progress dialog whose only
// button cancels it. It is the prompt again with the bar in place of the
// Download button, so the reader is watching the thing they just agreed to
// rather than a new window about something else. The waiters run when the
// file is in place.
func runModelDownload(parent *gtk.Window, onReady func()) {
	modelDownload.running = true
	if onReady != nil {
		modelDownload.waiters = append(modelDownload.waiters, onReady)
	}
	ctx, cancel := context.WithCancel(context.Background())

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.Append(newModelFactsRow())
	progress, bar, count := newModelProgress()
	content.Append(progress)
	dialog := adw.NewAlertDialog("Downloading the speech model", modelDownloadBody)
	dialog.SetExtraChild(content)
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
				} else {
					bar.Pulse()
				}
				count.SetLabel(modelProgressText(got, total))
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
