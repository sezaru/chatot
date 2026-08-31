package ui

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// ShowAddAccountDialog opens the "Add account" flow: a label entry plus a QR
// area that starts a fresh pairing account (am.AddPairingAccount) and renders
// its QR until it links. On pair success it closes and calls onAdded so the
// switcher/header refresh. In fake/demo mode (no real base dir) AddPairingAccount
// fails cleanly and the dialog shows an explanatory note instead of a QR.
func ShowAddAccountDialog(parent *gtk.Window, am *client.AccountManager, onAdded func()) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Add account")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(360, 520)

	box := gtk.NewBox(gtk.OrientationVertical, 12)
	box.SetMarginTop(16)
	box.SetMarginBottom(16)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)

	intro := gtk.NewLabel("Scan with the phone you want to link. The account is added alongside the ones you already have.")
	intro.SetWrap(true)
	intro.SetJustify(gtk.JustifyCenter)
	intro.SetMaxWidthChars(40)
	box.Append(intro)

	labelEntry := gtk.NewEntry()
	labelEntry.SetPlaceholderText("Label this account (e.g. Work)")
	box.Append(labelEntry)

	linkBtn := gtk.NewButtonWithLabel("Add account")
	linkBtn.AddCSSClass("suggested-action")
	box.Append(linkBtn)

	qrPic := gtk.NewPicture()
	qrPic.SetSizeRequest(256, 256)
	qrPic.SetCanShrink(false)
	qrPic.SetVisible(false)
	box.Append(qrPic)

	status := gtk.NewLabel("")
	status.SetWrap(true)
	status.SetJustify(gtk.JustifyCenter)
	box.Append(status)

	// done stops the QR/event forwarding goroutines when the dialog closes, so
	// they don't leak waiting on channels that never close.
	done := make(chan struct{})
	closed := false
	closeOnce := func() {
		if !closed {
			closed = true
			close(done)
		}
	}
	dialog.ConnectCloseRequest(func() bool {
		closeOnce()
		return false
	})

	phoneBox := buildAddAccountPhonePairing(dialog, &closed, status)
	phoneBox.SetVisible(false)
	box.Append(phoneBox.container)

	linkBtn.ConnectClicked(func() {
		acct, err := am.AddPairingAccount(labelEntry.Text())
		if err != nil {
			status.SetText("Adding a second account requires a live WhatsApp link (unavailable in demo mode).")
			return
		}
		labelEntry.SetSensitive(false)
		linkBtn.SetSensitive(false)
		qrPic.SetVisible(true)
		phoneBox.SetVisible(true)
		phoneBox.account = acct
		status.SetText("Waiting for you to scan…")

		go func() {
			qr := acct.QRCodes()
			for {
				select {
				case <-done:
					return
				case code, ok := <-qr:
					if !ok {
						return
					}
					glib.IdleAdd(func() { setQRPicture(qrPic, status, code) })
				}
			}
		}()

		go func() {
			ev := acct.Events()
			for {
				select {
				case <-done:
					return
				case e, ok := <-ev:
					if !ok {
						return
					}
					if e.Kind == client.EventPairSuccess ||
						(e.Kind == client.EventConnection && e.Connection != nil && e.Connection.Connected) {
						glib.IdleAdd(func() {
							closeOnce()
							dialog.Close()
							if onAdded != nil {
								onAdded()
							}
						})
						return
					}
				}
			}
		}()
	})

	dialog.SetChild(box)
	dialog.SetDefaultWidget(linkBtn)
	dialog.Present()
}

// addAccountPhonePairing is the "Use a phone number instead" affordance of the
// add-account dialog: it pairs the specific new account (set once linking
// starts) rather than the app's active account.
type addAccountPhonePairing struct {
	container *gtk.Box
	account   *client.Account
}

func (p *addAccountPhonePairing) SetVisible(v bool) { p.container.SetVisible(v) }

func buildAddAccountPhonePairing(dialog *gtk.Window, closed *bool, status *gtk.Label) *addAccountPhonePairing {
	p := &addAccountPhonePairing{}

	reveal := gtk.NewButtonWithLabel("Use a phone number instead")
	reveal.AddCSSClass("flat")

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Phone number, e.g. +1 555 123 4567")
	getCode := gtk.NewButtonWithLabel("Get code")

	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.Append(entry)
	row.Append(getCode)

	inner := gtk.NewBox(gtk.OrientationVertical, 8)
	inner.SetVisible(false)
	inner.Append(row)

	reveal.ConnectClicked(func() {
		reveal.SetVisible(false)
		inner.SetVisible(true)
	})
	getCode.ConnectClicked(func() {
		if p.account == nil {
			return
		}
		phone := entry.Text()
		acct := p.account
		go func() {
			code, err := acct.PairPhone(context.Background(), phone)
			glib.IdleAdd(func() {
				if *closed {
					return
				}
				if err != nil {
					status.SetText(err.Error())
					return
				}
				status.SetText("On your phone, enter this code: " + code)
			})
		}()
	})

	container := gtk.NewBox(gtk.OrientationVertical, 8)
	container.Append(reveal)
	container.Append(inner)
	p.container = container
	return p
}

// setQRPicture renders payload as a QR texture into pic (reusing the linking
// screen's qrPNG encoder), updating status on failure. Call on the main loop.
func setQRPicture(pic *gtk.Picture, status *gtk.Label, payload string) {
	png, err := qrPNG(payload)
	if err != nil {
		status.SetText("Failed to render QR code")
		return
	}
	texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(png))
	if err != nil {
		status.SetText("Failed to render QR code")
		return
	}
	pic.SetPaintable(texture)
}
