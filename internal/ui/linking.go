package ui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/skip2/go-qrcode"
)

// LinkingView is the logged-out pairing screen: instructions, the QR code to
// scan, a status line, and a secondary phone-number pairing-code affordance.
// All mutation happens on the GTK main loop.
type LinkingView struct {
	*gtk.Box
	pic    *gtk.Picture
	status *gtk.Label

	phoneEntry *gtk.Entry
	phoneCode  *gtk.Label
	onPhone    func(phone string)
}

// NewLinkingView builds the centered pairing screen.
func NewLinkingView() *LinkingView {
	box := gtk.NewBox(gtk.OrientationVertical, 18)
	box.SetVAlign(gtk.AlignCenter)
	box.SetHAlign(gtk.AlignCenter)
	box.SetVExpand(true)
	box.SetHExpand(true)
	box.AddCSSClass("chatot-linking")

	// The mockup leads with a 22px heading, then the instruction as a separate
	// dim paragraph — not one long sentence above a bare QR.
	heading := gtk.NewLabel("Link chatot to WhatsApp")
	heading.AddCSSClass("chatot-linking-title")
	box.Append(heading)

	title := gtk.NewLabel("Open WhatsApp on your phone, go to Settings → Linked devices → Link a device, and point the camera here.")
	title.AddCSSClass("chatot-linking-body")
	title.SetWrap(true)
	title.SetJustify(gtk.JustifyCenter)
	title.SetMaxWidthChars(46)
	box.Append(title)

	// A 220px white card with a 14px border around the code, per the design,
	// rather than the raw 256px bitmap on the window background.
	pic := gtk.NewPicture()
	pic.SetSizeRequest(192, 192)
	pic.SetCanShrink(true)
	// Contain + centred alignment, or the picture expands to whatever height
	// the column offers and stretches the 220px card with it.
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetHAlign(gtk.AlignCenter)
	pic.SetVAlign(gtk.AlignCenter)
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("chatot-linking-qr")
	card.SetSizeRequest(220, 220)
	card.SetHAlign(gtk.AlignCenter)
	card.SetVAlign(gtk.AlignCenter)
	card.Append(pic)
	box.Append(card)

	statusRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	statusRow.SetHAlign(gtk.AlignCenter)
	dot := gtk.NewBox(gtk.OrientationVertical, 0)
	dot.AddCSSClass("chatot-linking-dot")
	dot.SetSizeRequest(7, 7)
	dot.SetVAlign(gtk.AlignCenter)
	statusRow.Append(dot)
	status := gtk.NewLabel("Connecting…")
	status.AddCSSClass("chatot-linking-status")
	status.SetWrap(true)
	statusRow.Append(status)
	box.Append(statusRow)

	v := &LinkingView{Box: box, pic: pic, status: status}
	v.buildPhonePairing(box)
	return v
}

// buildPhonePairing adds the secondary "link with phone number" affordance:
// a reveal button that swaps in an entry + "Get code" button, and a label
// that shows the resulting code (or an error) once requested.
func (v *LinkingView) buildPhonePairing(box *gtk.Box) {
	reveal := gtk.NewButtonWithLabel("Link with phone number")
	reveal.AddCSSClass("chatot-chip-btn")
	reveal.SetHAlign(gtk.AlignCenter)
	reveal.AddCSSClass("flat")

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Phone number, e.g. +1 555 123 4567")
	getCode := gtk.NewButtonWithLabel("Get code")

	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.Append(entry)
	row.Append(getCode)

	code := gtk.NewLabel("")
	code.SetWrap(true)
	code.SetJustify(gtk.JustifyCenter)
	code.SetVisible(false)

	phoneBox := gtk.NewBox(gtk.OrientationVertical, 8)
	phoneBox.SetVisible(false)
	phoneBox.Append(row)
	phoneBox.Append(code)

	reveal.ConnectClicked(func() {
		reveal.SetVisible(false)
		phoneBox.SetVisible(true)
	})
	getCode.ConnectClicked(func() {
		if v.onPhone == nil {
			return
		}
		code.SetVisible(false)
		v.onPhone(entry.Text())
	})

	v.phoneEntry = entry
	v.phoneCode = code

	box.Append(reveal)
	box.Append(phoneBox)
}

// OnPhonePairRequested registers the callback fired (off the main loop, by
// the caller) with the entered phone number when "Get code" is clicked.
func (v *LinkingView) OnPhonePairRequested(fn func(phone string)) { v.onPhone = fn }

// SetPairCode displays a successfully requested pairing code. Call on the
// main loop.
func (v *LinkingView) SetPairCode(code string) {
	v.phoneCode.SetText(formatPairCode(code))
	v.phoneCode.SetVisible(true)
}

// SetPairError shows an inline error under the phone-pairing row instead of
// a code. Call on the main loop.
func (v *LinkingView) SetPairError(msg string) {
	v.phoneCode.SetText(msg)
	v.phoneCode.SetVisible(true)
}

// formatPairCode wraps a raw pairing code with the on-phone instructions.
// Pure so it's unit testable.
func formatPairCode(code string) string {
	return "On your phone, enter this code: " + code
}

// SetQR renders payload as a QR code into the picture. Call on the main loop.
func (v *LinkingView) SetQR(payload string) {
	png, err := qrPNG(payload)
	if err != nil {
		v.SetStatus("Failed to render QR code")
		return
	}
	texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(png))
	if err != nil {
		v.SetStatus("Failed to render QR code")
		return
	}
	v.pic.SetPaintable(texture)
}

// SetStatus updates the status line. Call on the main loop.
func (v *LinkingView) SetStatus(text string) { v.status.SetText(text) }

// qrPNG encodes payload as a 512px PNG QR code. Pure (no GTK) so it's unit
// testable; qrcode.Encode returns an error for an empty payload.
func qrPNG(payload string) ([]byte, error) {
	// Rendered at the card's own size, not 512: a GtkPicture's size request is
	// a minimum, so an oversized bitmap grows the 220px card the design draws
	// no matter what SetCanShrink says. 2x for HiDPI.
	return qrcode.Encode(payload, qrcode.Medium, linkingQRSize*2)
}

// linkingQRSize is the QR bitmap's logical size inside the mockup's 220px
// card (220 minus its 14px padding on each side).
const linkingQRSize = 192
