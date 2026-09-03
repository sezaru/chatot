package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// cardDialog is the mockup's in-window dialog: a rounded sheet with a title
// row and a ✕, floating over a dimmed window.
//
// It replaces the plain gtk.Window these dialogs used to be. A separate
// top-level relies on the desktop to draw a title bar and a close button, and
// a compositor without server-side decorations draws neither — the dialogs
// came up as bare, unclosable rectangles. AdwDialog draws its own chrome
// inside the application window, which is also what the design shows.
//
// The method set mirrors the gtk.Window calls the dialogs already made, so
// converting one is a matter of swapping the constructor.
type cardDialog struct {
	*adw.Dialog

	// parent is captured from SetTransientFor; AdwDialog needs it at Present
	// time rather than up front.
	parent *gtk.Window
	body   *adw.ToolbarView
	title  *gtk.Label
	header *adw.HeaderBar
}

// newCardDialog builds an empty dialog card with its title row in place.
func newCardDialog() *cardDialog {
	dialog := adw.NewDialog()

	header := adw.NewHeaderBar()
	header.AddCSSClass("chatot-dialog-header")
	// Close only: the mockup's dialogs carry a single ✕ at the right, and a
	// dialog has no window controls of its own to show.
	header.SetShowStartTitleButtons(false)
	header.SetShowEndTitleButtons(true)

	// The design's dialog titles are left-aligned. AdwHeaderBar always centres
	// its title widget, so the real title is packed at the start and the
	// centre slot is left empty.
	title := gtk.NewLabel("")
	title.AddCSSClass("chatot-dialog-title")
	title.SetXAlign(0)
	header.SetTitleWidget(gtk.NewLabel(""))
	header.PackStart(title)

	body := adw.NewToolbarView()
	body.AddTopBar(header)
	dialog.SetChild(body)

	return &cardDialog{Dialog: dialog, body: body, title: title, header: header}
}

// PackEnd puts w at the right of the title row, where the design's Accounts
// card keeps its Add… pill.
func (d *cardDialog) PackEnd(w gtk.Widgetter) { d.header.PackEnd(w) }

// SetTitle sets the visible title row text, and the dialog's own title with
// it so assistive technology still announces the dialog.
func (d *cardDialog) SetTitle(title string) {
	d.title.SetLabel(title)
	d.Dialog.SetTitle(title)
}

// SetTransientFor records the window the dialog will be presented in. It
// takes a *gtk.Window rather than a widget so existing call sites read
// unchanged.
func (d *cardDialog) SetTransientFor(parent *gtk.Window) { d.parent = parent }

// SetModal is a no-op: an AdwDialog is always modal within its window.
func (d *cardDialog) SetModal(bool) {}

// SetDefaultSize sets the card's content size. A non-positive dimension is
// left to the content, matching gtk.Window's -1.
func (d *cardDialog) SetDefaultSize(width, height int) {
	if width > 0 {
		d.SetContentWidth(width)
	}
	if height > 0 {
		d.SetContentHeight(height)
	}
}

// SetChild puts content below the title row.
func (d *cardDialog) SetChild(content gtk.Widgetter) { d.body.SetContent(content) }

// Present shows the dialog inside the window given to SetTransientFor. With
// no parent it falls back to AdwDialog's own resolution, which finds the
// application's active window.
func (d *cardDialog) Present() {
	if d.parent != nil {
		d.Dialog.Present(d.parent)
		return
	}
	d.Dialog.Present(nil)
}

// Window returns the application window this dialog is presented in. Nested
// dialogs and GTK file choosers need a real gtk.Window parent, and the card
// itself is not one.
func (d *cardDialog) Window() *gtk.Window { return d.parent }
