package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// SyncView is the screen shown right after linking while the phone streams
// the chat list and recent messages: the chat list underneath would be
// empty or half-filled, so the user waits here instead. Same centred column
// as the pairing screen, with a spinner and a live tally in place of the
// QR card. All mutation happens on the GTK main loop.
type SyncView struct {
	*gtk.Box
	counts *gtk.Label
}

// NewSyncView builds the syncing screen.
func NewSyncView() *SyncView {
	box := gtk.NewBox(gtk.OrientationVertical, 18)
	box.SetVAlign(gtk.AlignCenter)
	box.SetHAlign(gtk.AlignCenter)
	box.SetVExpand(true)
	box.SetHExpand(true)
	box.AddCSSClass("chatot-linking")
	box.AddCSSClass("chatot-sync")

	box.Append(newAppMark(syncMarkSize))

	heading := gtk.NewLabel("Syncing your chats")
	heading.AddCSSClass("chatot-linking-title")
	box.Append(heading)

	body := gtk.NewLabel("Your recent conversations are being copied from your phone. Keep it unlocked and online until this finishes.")
	body.AddCSSClass("chatot-linking-body")
	body.SetWrap(true)
	body.SetJustify(gtk.JustifyCenter)
	body.SetMaxWidthChars(46)
	box.Append(body)

	spinner := adw.NewSpinner()
	spinner.SetSizeRequest(syncSpinnerSize, syncSpinnerSize)
	spinner.SetHAlign(gtk.AlignCenter)
	spinner.SetMarginTop(6)
	box.Append(spinner)

	counts := gtk.NewLabel("Waiting for your phone…")
	counts.AddCSSClass("chatot-linking-status")
	counts.AddCSSClass("chatot-sync-counts")
	box.Append(counts)

	return &SyncView{Box: box, counts: counts}
}

// SetCounts updates the tally line; empty keeps the waiting prompt.
func (v *SyncView) SetCounts(text string) {
	if text == "" {
		text = "Waiting for your phone…"
	}
	v.counts.SetText(text)
}

const (
	// syncMarkSize is the app mark at the top of the syncing screen.
	syncMarkSize = 72
	// syncSpinnerSize is the AdwSpinner under the copy.
	syncSpinnerSize = 40
)
