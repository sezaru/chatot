package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/cairo"
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

// SaveTranscribeModel persists the model the download prompt settled on,
// which opens from a voice note as well as from Preferences; main.go wires
// it to the settings file. Nil only in tests.
var SaveTranscribeModel func(key string)

// transcribeModelWatch hears a pick made in the download prompt while the
// Preferences row for the model is up, so the row follows it.
var transcribeModelWatch func(key string)

// setTranscribeModel makes key the model voice notes are transcribed with,
// saved with the other preferences. The other model's file stays in the
// cache: a switch back must not cost another download.
func setTranscribeModel(key string) {
	key = transcribe.ModelByKey(key).Key
	if key == TranscribeModel {
		return
	}
	TranscribeModel = key
	if SaveTranscribeModel != nil {
		SaveTranscribeModel(key)
	}
	if transcribeModelWatch != nil {
		transcribeModelWatch(key)
	}
}

// modelDialogTitle heads the prompt and stays while the download runs: the
// mockup morphs the one card in place rather than opening another.
const modelDialogTitle = "Download the speech model?"

// modelDialog is the mockup's speech-model card: title and body on the
// left, the two cards, the line under them, and a hairline button bar
// across the bottom. When the download starts it changes in place: the
// confirming button and its divider go, the bar gains a progress row, and
// the cancel word turns from "Not now" into "Cancel".
type modelDialog struct {
	dialog  *adw.Dialog
	content *gtk.Box
	picker  gtk.Widgetter
	cancel  *gtk.Button
	divider *gtk.Separator
	confirm *gtk.Button
	// onCancel, when set, runs on any close of the card while it is
	// busy: the Cancel word, Escape, or a click outside.
	onCancel func()
	done     bool
}

// newModelDialog builds the card with key's model picked. onPick hears a
// click on a card; onConfirm the confirming button.
func newModelDialog(picked string, onPick func(key string), onConfirm func()) *modelDialog {
	d := &modelDialog{dialog: adw.NewDialog()}
	d.dialog.SetTitle(modelDialogTitle)
	d.dialog.SetContentWidth(440)
	d.dialog.AddCSSClass("chatot-model-dlg")

	root := gtk.NewBox(gtk.OrientationVertical, 0)
	d.content = gtk.NewBox(gtk.OrientationVertical, 9)
	d.content.AddCSSClass("chatot-model-dlg-body")
	title := gtk.NewLabel(modelDialogTitle)
	title.AddCSSClass("chatot-model-dlg-title")
	title.SetXAlign(0)
	title.SetWrap(true)
	d.content.Append(title)
	body := gtk.NewLabel(modelDownloadBody)
	body.AddCSSClass("chatot-model-dlg-text")
	body.SetXAlign(0)
	body.SetWrap(true)
	body.SetWrapMode(pango.WrapWordChar)
	d.content.Append(body)
	d.picker = newModelPicker(picked, onPick)
	d.content.Append(d.picker)
	root.Append(d.content)

	bar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	bar.AddCSSClass("chatot-model-dlg-bar")
	d.cancel = gtk.NewButtonWithLabel("Not now")
	d.cancel.AddCSSClass("flat")
	d.cancel.AddCSSClass("chatot-model-dlg-btn")
	d.cancel.AddCSSClass("chatot-model-dlg-btn-start")
	d.cancel.ConnectClicked(func() { d.dialog.Close() })
	bar.Append(d.cancel)
	d.divider = gtk.NewSeparator(gtk.OrientationVertical)
	d.divider.AddCSSClass("chatot-model-dlg-divider")
	bar.Append(d.divider)
	d.confirm = gtk.NewButtonWithLabel(modelDownloadLabel(picked))
	d.confirm.AddCSSClass("flat")
	d.confirm.AddCSSClass("chatot-model-dlg-btn")
	d.confirm.AddCSSClass("chatot-model-dlg-btn-end")
	d.confirm.AddCSSClass("chatot-model-dlg-confirm")
	d.confirm.ConnectClicked(onConfirm)
	bar.Append(d.confirm)
	// Two equal halves either side of the 1px divider.
	d.cancel.SetHExpand(true)
	d.confirm.SetHExpand(true)
	root.Append(bar)

	d.dialog.SetChild(root)
	d.dialog.ConnectClosed(func() {
		if !d.done && d.onCancel != nil {
			d.onCancel()
		}
	})
	return d
}

// present shows the card in parent.
func (d *modelDialog) present(parent *gtk.Window) { d.dialog.Present(parent) }

// close takes the card down as finished, so the close does not count as a
// cancel.
func (d *modelDialog) close() {
	d.done = true
	d.dialog.ForceClose()
}

// showModelDownloadDialog asks before fetching the speech model (it is a
// large download, not something to start on a click that only meant
// "transcribe"), with the two models to pick from, then fetches the pick
// with a progress bar in the same card. The pick becomes the preference
// only once that model is in place: "Not now", or Cancel on the download,
// leaves the model the app had. onReady, if set, runs once the chosen
// model is in place; a download already under way takes it as another
// waiter. The Preferences row opens this same dialog.
func showModelDownloadDialog(parent *gtk.Window, onReady func()) {
	showModelDownloadDialogPicked(parent, TranscribeModel, onReady)
}

// showModelDownloadDialogPicked is showModelDownloadDialog with the card
// for key picked to start with (the current model, or a hook's choice).
func showModelDownloadDialogPicked(parent *gtk.Window, key string, onReady func()) {
	if modelDownload.running {
		if onReady != nil {
			modelDownload.waiters = append(modelDownload.waiters, onReady)
		}
		return
	}
	picked := transcribe.ModelByKey(key).Key
	var d *modelDialog
	d = newModelDialog(picked, func(key string) {
		picked = key
		d.confirm.SetLabel(modelDownloadLabel(key))
	}, func() {
		if transcribe.ModelReady(cacheDir(), picked) {
			// A model already on disk (the other one, downloaded
			// earlier) is a switch, not a fetch.
			setTranscribeModel(picked)
			d.close()
			if onReady != nil {
				onReady()
			}
			return
		}
		d.startDownload(picked, onReady)
	})
	d.present(parent)
}

// runModelDownload fetches the model called key behind the card already
// in its downloading state, for a hook that skips the prompt.
func runModelDownload(parent *gtk.Window, key string, onReady func()) {
	d := newModelDialog(transcribe.ModelByKey(key).Key, nil, nil)
	d.present(parent)
	d.startDownload(key, onReady)
}

// startDownload turns the card into the download in progress and fetches
// the model called key. The model becomes the preference, and the
// waiters run, when the file is in place; a cancelled or failed fetch
// changes nothing.
func (d *modelDialog) startDownload(key string, onReady func()) {
	modelDownload.running = true
	if onReady != nil {
		modelDownload.waiters = append(modelDownload.waiters, onReady)
	}
	ctx, cancel := context.WithCancel(context.Background())
	model := transcribe.ModelByKey(key)

	// The cards stay as the record of the pick, no longer a control.
	gtk.BaseWidget(d.picker).SetCanTarget(false)
	progress, bar, count := newModelProgress(model)
	d.content.Append(progress)
	d.divider.SetVisible(false)
	d.confirm.SetVisible(false)
	d.cancel.SetLabel("Cancel")
	d.cancel.RemoveCSSClass("chatot-model-dlg-btn-start")
	d.cancel.AddCSSClass("chatot-model-dlg-btn-only")
	d.onCancel = cancel

	go func() {
		err := transcribe.DownloadModel(ctx, cacheDir(), model.Key, func(got, total int64) {
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
			modelDownload.running = false
			waiters := modelDownload.waiters
			modelDownload.waiters = nil
			d.close()
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					showToast(Prefs.Toasts, "Speech model download failed: "+err.Error())
				}
				return
			}
			setTranscribeModel(model.Key)
			// The Preferences row, if up, now says "downloaded" (a
			// re-fetch of the current model is no change to it).
			if transcribeModelWatch != nil {
				transcribeModelWatch(TranscribeModel)
			}
			for _, w := range waiters {
				w()
			}
		})
	}()
}

// modelDownloadBody says what the download is and where it goes. The
// progress dialog keeps it: a download that has started still has to answer
// "what is this and where is it going", and dropping the sentence at the
// moment the bar appears would take the answer away.
const modelDownloadBody = "Voice notes are transcribed on this computer by whisper.cpp. Pick a speech " +
	"model; it is downloaded once, from Hugging Face, into chatot's cache."

// modelDownloadLabel is the prompt's confirming button for the model
// called key: what the click fetches, in megabytes, or a plain switch when
// that model is already in the cache.
func modelDownloadLabel(key string) string {
	if transcribe.ModelReady(cacheDir(), key) {
		return "Use this model"
	}
	return "Download " + modelSizeText(transcribe.ModelByKey(key).Size)
}

// modelMeta is the one-line account of m the picker and the Preferences
// row share: the download, the memory it takes while transcribing, and
// how it compares with the other model. Joined with no-break spaces
// before each dot so a wrap never strands one.
func modelMeta(m transcribe.Model) string {
	parts := []string{
		modelSizeText(m.Size) + " download",
		fmt.Sprintf("~%.1f\u00a0GB\u00a0RAM", float64(m.PeakMemory)/(1<<30)),
		m.Speed,
		m.Quality,
	}
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\u00a0· ")
}

// newModelPicker is the prompt's list of models, one card each with the
// picked one framed in the accent, and the line under them that says what
// runs any of them. onPick, when set, hears a click on a card; nil makes
// the cards a record of the pick rather than a control, for the progress
// dialog. A card names its file in a tooltip, since the name on it is
// the human one.
func newModelPicker(picked string, onPick func(key string)) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 6)
	box.AddCSSClass("chatot-model-picker")
	var first *gtk.ToggleButton
	for _, m := range transcribe.Models {
		card := newModelCard(m, m.Key == picked)
		if first == nil {
			first = card
		} else {
			card.SetGroup(first)
		}
		if onPick == nil {
			card.SetCanTarget(false)
		} else {
			key := m.Key
			card.ConnectToggled(func() {
				if card.Active() {
					onPick(key)
				}
			})
		}
		box.Append(card)
	}
	foot := gtk.NewLabel("whisper.cpp · multilingual · runs offline")
	foot.AddCSSClass("chatot-model-foot")
	foot.SetXAlign(0)
	box.Append(foot)
	return box
}

// newModelCard is one model in the picker: a radio ring, the name with
// its tags beside it, and modelMeta under them.
func newModelCard(m transcribe.Model, on bool) *gtk.ToggleButton {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)

	ring := gtk.NewDrawingArea()
	ring.SetSizeRequest(17, 17)
	ring.SetVAlign(gtk.AlignCenter)
	ring.AddCSSClass("chatot-model-radio")
	row.Append(ring)

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetHExpand(true)
	head := gtk.NewBox(gtk.OrientationHorizontal, 6)
	name := gtk.NewLabel(m.Name)
	name.AddCSSClass("chatot-model-name")
	name.SetXAlign(0)
	head.Append(name)
	if m.Recommended {
		tag := gtk.NewLabel("Recommended")
		tag.AddCSSClass("chatot-model-tag")
		tag.SetVAlign(gtk.AlignBaseline)
		head.Append(tag)
	}
	if transcribe.ModelReady(cacheDir(), m.Key) {
		have := gtk.NewLabel("· Downloaded")
		have.AddCSSClass("chatot-model-have")
		have.SetVAlign(gtk.AlignBaseline)
		head.Append(have)
	}
	col.Append(head)
	meta := gtk.NewLabel(modelMeta(m))
	meta.AddCSSClass("chatot-model-meta")
	meta.SetXAlign(0)
	meta.SetWrap(true)
	col.Append(meta)
	row.Append(col)

	card := gtk.NewToggleButton()
	card.SetChild(row)
	card.AddCSSClass("chatot-model-card")
	card.SetTooltipText(m.File)
	card.SetActive(on)

	// The ring is drawn, not a GtkCheckButton: the design's radio is a 2px
	// ring with a 7px dot, and a CSS ring on a round node comes out dotted
	// at fractional scales. The colour is the area's own CSS colour, so
	// the sheet decides the accent.
	ring.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		c := area.Color()
		cr.SetSourceRGBA(float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha()))
		cx, cy := float64(w)/2, float64(h)/2
		cr.SetLineWidth(2)
		cr.Arc(cx, cy, 7.5, 0, 6.2832)
		cr.Stroke()
		if card.Active() {
			cr.Arc(cx, cy, 3.5, 0, 6.2832)
			cr.Fill()
		}
	})
	setOn := func() {
		if card.Active() {
			ring.AddCSSClass("chatot-model-radio-on")
		} else {
			ring.RemoveCSSClass("chatot-model-radio-on")
		}
		ring.QueueDraw()
	}
	setOn()
	card.ConnectToggled(setOn)
	return card
}

// modelSizeText is the model's size in whole megabytes, which is how both
// the picker and the counter below the bar say it: one file named twice
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

// newModelProgress is the download in progress under the picker: the thin
// accent bar the mockup draws, with what is happening on one side of the
// line below it and how far it has got on the other.
func newModelProgress(m transcribe.Model) (box *gtk.Box, bar *gtk.ProgressBar, count *gtk.Label) {
	box = gtk.NewBox(gtk.OrientationVertical, 6)
	box.AddCSSClass("chatot-model-progress")
	bar = gtk.NewProgressBar()
	bar.AddCSSClass("chatot-model-bar")
	box.Append(bar)
	line := gtk.NewBox(gtk.OrientationHorizontal, 8)
	status := gtk.NewLabel("Downloading " + m.Name + " once")
	status.AddCSSClass("chatot-model-status")
	status.SetXAlign(0)
	status.SetHExpand(true)
	line.Append(status)
	count = gtk.NewLabel(modelProgressText(0, m.Size))
	count.AddCSSClass("chatot-model-bytes")
	count.SetXAlign(1)
	line.Append(count)
	box.Append(line)
	return box, bar, count
}
