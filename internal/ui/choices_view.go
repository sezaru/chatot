package ui

import (
	"context"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// choicesView is the display view-model of a business message's choices:
// the buttons under its text, and the list picker if it has one.
type choicesView struct {
	Buttons []choiceButtonView
	List    *client.ChoiceList
}

// choiceButtonView is one button as the bubble shows it: a glyph saying
// what tapping does (nothing for a plain reply) before the label.
type choiceButtonView struct {
	Glyph  string
	Label  string
	Button client.ChoiceButton
}

// choicesVM derives the choices view-model for m (nil when it offers none).
func choicesVM(m client.Message) *choicesView {
	if m.Choices == nil {
		return nil
	}
	v := &choicesView{List: m.Choices.List}
	for _, b := range m.Choices.Buttons {
		v.Buttons = append(v.Buttons, choiceButtonView{Glyph: choiceGlyph(b.Kind), Label: b.Label, Button: b})
	}
	return v
}

// choiceGlyph marks a button that leaves the chat or touches the clipboard;
// a reply, the common case, stays a bare label.
func choiceGlyph(kind string) string {
	switch kind {
	case "url":
		return "↗"
	case "call":
		return "📞"
	case "copy":
		return "⧉"
	case "flow":
		return "📱"
	}
	return ""
}

// flowOnPhoneText is what tapping a WhatsApp Flow button says: the form
// is rendered by WhatsApp on the phone only (not even WhatsApp Web runs
// one), and a bank's password prompt is not something to imitate.
const flowOnPhoneText = "This form only opens in WhatsApp on your phone"

// listGlyph marks the picker button that opens a list.
const listGlyph = "☰"

// choicesCopyText is the plain rendering "Copy text" takes for a message
// with choices: its text, then one marked line per button and list row.
func choicesCopyText(m client.Message) string {
	lines := []string{}
	if m.Text != "" {
		lines = append(lines, m.Text)
	}
	for _, b := range m.Choices.Buttons {
		lines = append(lines, "▸ "+b.Label)
	}
	if l := m.Choices.List; l != nil {
		lines = append(lines, "▸ "+l.Title)
		for _, sec := range l.Sections {
			for _, r := range sec.Rows {
				lines = append(lines, "▸ "+r.Title)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// buildChoices renders the buttons under a business message: each a
// full-width tap target split from the text and from each other by a
// hairline. A reply sends the pick back through onChoice; a link opens; a
// number or code goes to the clipboard; the list picker opens its dialog.
func buildChoices(msg client.Message, v choicesView, h bubbleHooks) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-choices")
	for _, b := range v.Buttons {
		btn := b.Button
		row := newChoiceButton(b.Glyph, b.Label, func() {
			switch btn.Kind {
			case "flow":
				showToast(h.toasts, flowOnPhoneText)
			case "url":
				if btn.URL != "" {
					gtk.NewURILauncher(btn.URL).Launch(context.Background(), h.window, nil)
				}
			case "call":
				copyChoice(h, btn.Phone, "Number copied to the clipboard")
			case "copy":
				copyChoice(h, btn.CopyText, "Copied to the clipboard")
			default:
				if h.onChoice != nil {
					h.onChoice(msg, client.ChoiceSelection{ID: btn.ID, Label: btn.Label, Index: btn.Index})
				}
			}
		})
		if btn.Kind == "flow" {
			row.AddCSSClass("chatot-choice-phone")
			row.SetTooltipText(flowOnPhoneText)
		}
		box.Append(row)
	}
	if l := v.List; l != nil {
		list := *l
		box.Append(newChoiceButton(listGlyph, list.Title, func() {
			showListPickerDialog(h.window, list, func(r client.ChoiceRow) {
				if h.onChoice != nil {
					h.onChoice(msg, client.ChoiceSelection{ID: r.ID, Label: r.Title, Description: r.Description, IsRow: true})
				}
			})
		}))
	}
	return box
}

// newChoiceButton is one choice row: the glyph, if any, then the label,
// centred the way the phone centres them.
func newChoiceButton(glyph, label string, onClick func()) *gtk.Button {
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)
	row.SetHAlign(gtk.AlignCenter)
	if glyph != "" {
		g := gtk.NewLabel(glyph)
		g.AddCSSClass("chatot-choice-glyph")
		row.Append(g)
	}
	l := gtk.NewLabel(label)
	l.SetWrap(true)
	l.SetWrapMode(pango.WrapWordChar)
	l.SetJustify(gtk.JustifyCenter)
	l.SetMaxWidthChars(40)
	row.Append(l)

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-choice-btn")
	btn.SetHExpand(true)
	// No keyboard focus on click: the thread rebuilds around the reply
	// that follows, and a focused widget vanishing jumps the list.
	btn.SetFocusOnClick(false)
	btn.ConnectClicked(onClick)
	return btn
}

// copyChoice puts text on the clipboard and says so; an empty text (a
// button the business sent without its target) does nothing.
func copyChoice(h bubbleHooks, text, done string) {
	if text == "" {
		return
	}
	gdk.DisplayGetDefault().Clipboard().SetText(text)
	showToast(h.toasts, done)
}

// showListPickerDialog presents a list picker the way the mockup's choice
// card does: the picker's title, then a bordered list of rows per section,
// each section headed by its name. onPick receives the chosen row and the
// dialog closes.
func showListPickerDialog(parent *gtk.Window, list client.ChoiceList, onPick func(client.ChoiceRow)) {
	dialog := newCardDialog()
	dialog.SetTitle(list.Title)
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(360, -1)

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.SetMarginTop(10)
	content.SetMarginStart(20)
	content.SetMarginEnd(20)
	content.SetMarginBottom(14)

	for _, sec := range list.Sections {
		if sec.Title != "" {
			head := gtk.NewLabel(strings.ToUpper(sec.Title))
			head.AddCSSClass("chatot-choice-section")
			head.SetXAlign(0)
			content.Append(head)
		}
		box := gtk.NewBox(gtk.OrientationVertical, 0)
		box.AddCSSClass("chatot-choice-list")
		box.SetMarginTop(6)
		box.SetMarginBottom(8)
		for _, r := range sec.Rows {
			col := gtk.NewBox(gtk.OrientationVertical, 2)
			title := gtk.NewLabel(r.Title)
			title.SetXAlign(0)
			title.SetWrap(true)
			title.SetWrapMode(pango.WrapWordChar)
			col.Append(title)
			if r.Description != "" {
				desc := gtk.NewLabel(r.Description)
				desc.SetXAlign(0)
				desc.SetWrap(true)
				desc.SetWrapMode(pango.WrapWordChar)
				desc.AddCSSClass("chatot-choice-desc")
				col.Append(desc)
			}
			btn := gtk.NewButton()
			btn.SetChild(col)
			btn.AddCSSClass("flat")
			btn.AddCSSClass("chatot-choice-row")
			picked := r
			btn.ConnectClicked(func() {
				dialog.Close()
				onPick(picked)
			})
			box.Append(btn)
		}
		content.Append(box)
	}

	dialog.SetChild(content)
	dialog.Present()
}
