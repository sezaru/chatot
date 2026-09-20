package ui

import (
	"strings"
	"unicode"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

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

// buildContactContent renders the mockup's contact card: a 38px initial
// avatar beside the name over the phone number, then a hairline and a centred
// accent "Message" action.
func buildContactContent(v contactView) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-bubble-contact")

	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-contact-row")

	// Keyed on the name rather than a JID: a vCard in a message carries no
	// account we could colour-match against, and the name is what the card
	// shows.
	avatar := newAvatarInitial(v.Name, contactInitial(v.Name), 38)
	avatar.SetVAlign(gtk.AlignCenter)
	row.Append(avatar)

	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetVAlign(gtk.AlignCenter)
	col.SetHExpand(true)
	name := gtk.NewLabel(v.Name)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.AddCSSClass("chatot-contact-name")
	col.Append(name)
	if len(v.Phones) > 0 {
		shown := make([]string, len(v.Phones))
		for i, p := range v.Phones {
			shown[i] = formatPhoneDisplay(p)
		}
		phones := gtk.NewLabel(strings.Join(shown, ", "))
		phones.SetXAlign(0)
		phones.SetSelectable(true)
		phones.SetEllipsize(pango.EllipsizeEnd)
		phones.AddCSSClass("chatot-contact-phones")
		col.Append(phones)
	}
	row.Append(col)
	box.Append(row)

	box.Append(buildContactAction(v))
	return box
}

// buildContactAction is the card's centred "Message" row. It opens a chat
// with the first phone number on the vCard by activating the app's
// open-chat action — the same path the chat list and notifications use — so
// no new callback has to be threaded through every bubble builder. A card
// with no usable number renders the row inert rather than hiding it, since
// the mockup always shows it.
func buildContactAction(v contactView) gtk.Widgetter {
	jid, ok := contactChatJID(v.Phones)
	if !ok {
		action := gtk.NewLabel("Message")
		action.AddCSSClass("chatot-contact-action")
		action.AddCSSClass("chatot-contact-action-off")
		return action
	}

	btn := gtk.NewButton()
	label := gtk.NewLabel("Message")
	label.AddCSSClass("chatot-contact-action")
	btn.SetChild(label)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btn")
	btn.SetFocusOnClick(false)
	btn.ConnectClicked(func() {
		btn.ActivateAction("app.open-chat", glib.NewVariantString(jid))
	})
	return btn
}

// contactChatJID turns the first E.164-looking number on a vCard into the
// WhatsApp JID for a 1:1 chat. ok is false when the card carries no number
// that could address one.
func contactChatJID(phones []string) (string, bool) {
	for _, p := range phones {
		if norm, valid := normalizePhone(p); valid {
			return strings.TrimPrefix(norm, "+") + "@s.whatsapp.net", true
		}
		if digits, valid := vcardBareInternational(p); valid {
			return digits + "@s.whatsapp.net", true
		}
	}
	return "", false
}

// vcardBareInternational reads a TEL value that is already a country code
// and number with no "+" in front of it — which is how a card written
// without a waid parameter carries one. normalizePhone will not guess a
// country for a number that does not say it has one, and rightly so for
// something typed into the new-chat field; on a vCard the alternative is a
// card whose Message row does nothing, which is what a contact shared
// twice showed, working from one copy and dead from the other.
//
// The length is the guard: a national number written without its country
// code is shorter than this, so only a string long enough to carry one is
// read as international.
const vcardMinInternationalDigits = 11

func vcardBareInternational(phone string) (string, bool) {
	var b strings.Builder
	for _, r := range phone {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) || unicode.Is(unicode.Pd, r) || r == '(' || r == ')' || r == '.':
			// Separators the number may be written with.
		default:
			return "", false
		}
	}
	digits := b.String()
	if len(digits) < vcardMinInternationalDigits || len(digits) > 15 {
		return "", false
	}
	return digits, true
}
