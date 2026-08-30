package ui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/skip2/go-qrcode"
)

// LinkingView is the logged-out pairing screen: instructions, the QR code to
// scan, and a status line. All mutation happens on the GTK main loop.
type LinkingView struct {
	*gtk.Box
	pic    *gtk.Picture
	status *gtk.Label
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

	return &LinkingView{Box: box, pic: pic, status: status}
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
