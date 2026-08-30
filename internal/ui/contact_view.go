package ui

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// contactView is the pure display view-model for a contact-card bubble,
// computed from a client.Message so it's unit-testable without a display.
type contactView struct {
	IsContact bool
	Name      string // "Contact" when the vCard carries no display name
	Phones    []string
}

// contactVM derives the contact view-model for m (zero value if m carries
// no contact).
func contactVM(m client.Message) contactView {
	if m.Contact == nil {
		return contactView{}
	}
	name := m.Contact.DisplayName
	if name == "" {
		name = "Contact"
	}
	return contactView{IsContact: true, Name: name, Phones: m.Contact.Phones}
}

// buildContactContent renders a contact bubble: a 👤 name line and one
// selectable label per phone number.
func buildContactContent(v contactView) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 2)
	box.AddCSSClass("chatot-bubble-contact")

	name := gtk.NewLabel("👤 " + v.Name)
	name.SetXAlign(0)
	name.AddCSSClass("chatot-contact-name")
	box.Append(name)

	if len(v.Phones) == 0 {
		return box
	}

	phones := gtk.NewLabel(strings.Join(v.Phones, "\n"))
	phones.SetXAlign(0)
	phones.SetSelectable(true)
	phones.AddCSSClass("chatot-contact-phones")
	box.Append(phones)

	return box
}
