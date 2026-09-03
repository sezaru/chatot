package ui

import (
	"context"
	"log"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// addAccountFallbackLabel names a pairing account until the user's label is
// applied on link (the dialog opens the pairing before asking for one).
const addAccountFallbackLabel = "New account"

// ShowAddAccountDialog opens the mockup's "Add account" card: the instruction,
// a 180px QR card that is live from the moment the card opens, "Waiting for
// you to scan…", and the "Label this account" field. Pairing starts on open
// (am.AddPairingAccount) so there is nothing to click; on pair success the
// account takes the label from the field, the card closes and onAdded runs.
// Closing without linking removes the half-made account again, so cancelling
// leaves no ghost in the rail.
func ShowAddAccountDialog(parent *gtk.Window, am *client.AccountManager, onAdded func()) {
	dialog := newCardDialog()
	dialog.SetTitle("Add account")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(380, -1)

	box := dialogBody(12)

	intro := gtk.NewLabel("Scan with the phone you want to link. The account is added alongside the ones you already have.")
	intro.SetWrap(true)
	intro.SetJustify(gtk.JustifyCenter)
	intro.SetMaxWidthChars(40)
	intro.AddCSSClass("chatot-card-sub")
	box.Append(intro)

	qr := newQRCard()
	box.Append(qr.card)

	status := gtk.NewLabel("Starting pairing…")
	status.SetWrap(true)
	status.SetJustify(gtk.JustifyCenter)
	status.AddCSSClass("chatot-linking-status")
	box.Append(status)

	// "Label this account" beside its entry, in the design's bordered card.
	labelCard := newSettingsCard()
	labelRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	labelRow.AddCSSClass("chatot-card-row")
	labelRow.Append(settingsRowBody("Label this account", "Shown in the account button"))
	labelEntry := gtk.NewEntry()
	labelEntry.SetPlaceholderText("Work")
	labelEntry.SetVAlign(gtk.AlignCenter)
	labelEntry.SetSizeRequest(130, -1)
	labelEntry.AddCSSClass("chatot-card-entry")
	labelRow.Append(labelEntry)
	labelCard.Add(labelRow)
	box.Append(labelCard)

	// done stops the QR/event forwarding goroutines when the dialog closes, so
	// they don't leak waiting on channels that never close.
	done := make(chan struct{})
	closed := false
	linked := false
	var acct *client.Account
	closeOnce := func() {
		if closed {
			return
		}
		closed = true
		close(done)
		if !linked && acct != nil {
			// Nothing paired: drop the provisional account rather than leave a
			// permanently logged-out entry behind.
			go func() {
				if err := am.RemoveAccount(acct.ID); err != nil {
					log.Printf("chatot: discard unpaired account %q: %v", acct.ID, err)
				}
				glib.IdleAdd(func() {
					if onAdded != nil {
						onAdded()
					}
				})
			}()
		}
	}
	dialog.ConnectClosed(closeOnce)

	phoneBox := buildAddAccountPhonePairing(&closed, status)
	box.Append(phoneBox.container)

	var err error
	acct, err = am.AddPairingAccount(addAccountFallbackLabel)
	if err != nil {
		status.SetText("Adding an account needs a live WhatsApp connection.")
		phoneBox.SetVisible(false)
	} else {
		phoneBox.account = acct
		status.SetText("Waiting for a code…")
		if onAdded != nil {
			onAdded() // the rail shows the pending account right away
		}

		go func() {
			codes := acct.QRCodes()
			for {
				select {
				case <-done:
					return
				case code, ok := <-codes:
					if !ok {
						return
					}
					glib.IdleAdd(func() {
						qr.Set(code, status)
						status.SetText("Waiting for you to scan…")
					})
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
							linked = true
							if label := strings.TrimSpace(labelEntry.Text()); label != "" {
								if err := am.RenameAccount(acct.ID, label); err != nil {
									log.Printf("chatot: label new account: %v", err)
								}
							}
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
	}

	dialog.SetChild(box)
	dialog.Present()
	labelEntry.GrabFocus()
}

// addAccountPhonePairing is the "Use a phone number instead" affordance of the
// add-account dialog: it pairs the specific new account rather than the app's
// active account.
type addAccountPhonePairing struct {
	container *gtk.Box
	account   *client.Account
}

func (p *addAccountPhonePairing) SetVisible(v bool) { p.container.SetVisible(v) }

func buildAddAccountPhonePairing(closed *bool, status *gtk.Label) *addAccountPhonePairing {
	p := &addAccountPhonePairing{}

	reveal := gtk.NewButtonWithLabel("Use a phone number instead")
	reveal.AddCSSClass("flat")
	reveal.AddCSSClass("chatot-link-btn")
	reveal.SetHAlign(gtk.AlignCenter)

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Phone number, e.g. +1 555 123 4567")
	entry.SetHExpand(true)
	getCode := gtk.NewButtonWithLabel("Get code")
	getCode.AddCSSClass("chatot-outline-btn")

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

// qrCard is the mockup's 180px bordered QR tile. Until a code arrives it
// shows the design's hatched placeholder, so the card has its final shape
// from the first frame instead of appearing only once pairing starts.
type qrCard struct {
	card *gtk.Box
	pic  *gtk.Picture
}

// qrCardSize is the design's tile; qrCardInner is the bitmap inside its 14px
// padding.
const (
	qrCardSize  = 180
	qrCardInner = 152
)

func newQRCard() *qrCard {
	pic := gtk.NewPicture()
	pic.SetSizeRequest(qrCardInner, qrCardInner)
	pic.SetCanShrink(true)
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetHAlign(gtk.AlignCenter)
	pic.SetVAlign(gtk.AlignCenter)
	pic.SetVisible(false)

	hatch := gtk.NewBox(gtk.OrientationVertical, 0)
	hatch.AddCSSClass("chatot-qr-placeholder")
	hatch.SetSizeRequest(qrCardInner, qrCardInner)
	hatch.SetHAlign(gtk.AlignCenter)
	hatch.SetVAlign(gtk.AlignCenter)

	stack := gtk.NewStack()
	stack.AddNamed(hatch, "placeholder")
	stack.AddNamed(pic, "qr")

	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("chatot-linking-qr")
	card.SetSizeRequest(qrCardSize, qrCardSize)
	card.SetHAlign(gtk.AlignCenter)
	card.SetVAlign(gtk.AlignCenter)
	card.Append(stack)
	q := &qrCard{card: card, pic: pic}
	return q
}

// Set renders payload as the card's QR (reusing the linking screen's qrPNG
// encoder), reporting a render failure on status. Call on the main loop.
func (q *qrCard) Set(payload string, status *gtk.Label) {
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
	q.pic.SetPaintable(texture)
	q.pic.SetVisible(true)
	if stack, ok := q.pic.Parent().(*gtk.Stack); ok {
		stack.SetVisibleChildName("qr")
	}
}
