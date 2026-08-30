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
	box := gtk.NewBox(gtk.OrientationVertical, 16)
	box.SetVAlign(gtk.AlignCenter)
	box.SetHAlign(gtk.AlignCenter)
	box.SetVExpand(true)
	box.SetHExpand(true)

	title := gtk.NewLabel("On your phone: WhatsApp → Settings → Linked Devices → Link a device, then scan this code")
	title.SetWrap(true)
	title.SetJustify(gtk.JustifyCenter)
	title.SetMaxWidthChars(48)

	pic := gtk.NewPicture()
	pic.SetSizeRequest(256, 256)
	pic.SetCanShrink(false)

	status := gtk.NewLabel("Connecting…")
	status.SetWrap(true)

	box.Append(title)
	box.Append(pic)
	box.Append(status)

	v := &LinkingView{Box: box, pic: pic, status: status}
	v.buildPhonePairing(box)
	return v
}

// buildPhonePairing adds the secondary "link with phone number" affordance:
// a reveal button that swaps in an entry + "Get code" button, and a label
// that shows the resulting code (or an error) once requested.
func (v *LinkingView) buildPhonePairing(box *gtk.Box) {
	reveal := gtk.NewButtonWithLabel("Link with phone number instead")
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
	return qrcode.Encode(payload, qrcode.Medium, 512)
}
