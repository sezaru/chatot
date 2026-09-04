package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// The mockup builds nearly every dialog body out of the same two pieces: an
// uppercase caption over a white, hairline-bordered card, and inside it rows
// separated by 1px lines. This file is that toolkit, so Preferences, Contact
// info, Linked devices, Export and the choice dialogs all read identically
// instead of each inventing its own row.

// settingsCard is the bordered, rounded container rows go into.
type settingsCard struct {
	*gtk.Box
	rows int
}

// newSettingsCard builds an empty card.
func newSettingsCard() *settingsCard {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-card")
	return &settingsCard{Box: box}
}

// Add appends a row, drawing the hairline separator above every row but the
// first (the mockup's border-top pattern, which avoids a stray line under the
// last row).
func (c *settingsCard) Add(row gtk.Widgetter) {
	if c.rows > 0 {
		gtk.BaseWidget(row).AddCSSClass("chatot-card-sep")
	}
	c.rows++
	c.Append(row)
}

// newSettingsGroup is a card under an uppercase caption, the mockup's
// section unit in Preferences.
func newSettingsGroup(caption string, card *settingsCard) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 6)
	if caption != "" {
		label := gtk.NewLabel(caption)
		label.SetXAlign(0)
		label.AddCSSClass("chatot-card-caption")
		box.Append(label)
	}
	box.Append(card)
	return box
}

// settingsRowBody builds a row's left column: the label, optionally over a
// dim explanatory subtitle.
func settingsRowBody(label, sub string) *gtk.Box {
	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)

	title := gtk.NewLabel(label)
	title.SetXAlign(0)
	title.AddCSSClass("chatot-card-label")
	col.Append(title)

	if sub != "" {
		s := gtk.NewLabel(sub)
		s.SetXAlign(0)
		s.SetWrap(true)
		s.AddCSSClass("chatot-card-sub")
		col.Append(s)
	}
	return col
}

// newSwitchRow is a row whose trailing control is a switch.
func newSwitchRow(label, sub string, active bool, onToggle func(bool)) (gtk.Widgetter, *gtk.Switch) {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.AddCSSClass("chatot-card-row")
	row.Append(settingsRowBody(label, sub))

	sw := gtk.NewSwitch()
	sw.SetVAlign(gtk.AlignCenter)
	sw.SetActive(active)
	if onToggle != nil {
		sw.ConnectStateSet(func(state bool) bool {
			onToggle(state)
			return false
		})
	}
	row.Append(sw)
	return row, sw
}

// newValueRow is a row showing a current value with the mockup's ▾ affordance;
// clicking it opens whatever chooser the caller wires.
func newValueRow(label, sub, value string, onClick func()) (gtk.Widgetter, *gtk.Label) {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.Append(settingsRowBody(label, sub))

	// The ▾ promises a chooser, so a read-only row (nothing wired) shows the
	// bare value.
	text := value
	if onClick != nil {
		text += " ▾"
	}
	val := gtk.NewLabel(text)
	val.AddCSSClass("chatot-card-value")
	val.SetVAlign(gtk.AlignCenter)
	row.Append(val)

	if onClick == nil {
		row.AddCSSClass("chatot-card-row")
		return row, val
	}
	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btnrow")
	btn.ConnectClicked(onClick)
	return btn, val
}

// newActionRow is a row whose trailing element is an accent-coloured action
// word ("Choose…", "Unlink"), the mockup's isButton variant.
func newActionRow(label, sub, action string, destructive bool, onClick func()) gtk.Widgetter {
	row, _ := newActionRowLabel(label, sub, action, destructive, onClick)
	return row
}

// newActionRowLabel is newActionRow that also hands back the action word,
// for rows whose word changes ("2 blocked", "1.4 GB · Clear").
func newActionRowLabel(label, sub, action string, destructive bool, onClick func()) (gtk.Widgetter, *gtk.Label) {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.Append(settingsRowBody(label, sub))

	word := gtk.NewLabel(action)
	word.AddCSSClass("chatot-card-action")
	if destructive {
		word.AddCSSClass("chatot-card-action-danger")
	}
	word.SetVAlign(gtk.AlignCenter)
	row.Append(word)

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btnrow")
	btn.SetSensitive(onClick != nil)
	if onClick != nil {
		btn.ConnectClicked(onClick)
	}
	return btn, word
}

// newIconRow is the Contact-info variant: a 16px glyph column, the label, and
// an optional dim trailing value.
func newIconRow(icon, label, value string, destructive bool, onClick func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)

	glyph := gtk.NewLabel(icon)
	glyph.AddCSSClass("chatot-menu-icon")
	glyph.SetSizeRequest(16, -1)
	glyph.SetVAlign(gtk.AlignCenter)
	row.Append(glyph)

	text := gtk.NewLabel(label)
	text.SetXAlign(0)
	text.SetHExpand(true)
	text.AddCSSClass("chatot-card-label")
	if destructive {
		text.AddCSSClass("chatot-card-action-danger")
	}
	row.Append(text)

	if value != "" {
		val := gtk.NewLabel(value)
		val.AddCSSClass("chatot-card-value")
		val.SetVAlign(gtk.AlignCenter)
		row.Append(val)
	}

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btnrow")
	btn.SetSensitive(onClick != nil)
	if onClick != nil {
		btn.ConnectClicked(onClick)
	}
	return btn
}

// newDeviceRow is the Linked-devices variant: a 32px rounded icon square, the
// device name over a mono meta line, and an optional red Unlink button.
func newDeviceRow(icon, name, meta string, onUnlink func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	row.AddCSSClass("chatot-card-row")

	square := gtk.NewLabel(icon)
	square.AddCSSClass("chatot-device-icon")
	square.SetSizeRequest(32, 32)
	square.SetVAlign(gtk.AlignCenter)
	row.Append(square)

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	title := gtk.NewLabel(name)
	title.SetXAlign(0)
	title.SetEllipsize(pango.EllipsizeEnd)
	title.AddCSSClass("chatot-device-name")
	col.Append(title)
	sub := gtk.NewLabel(meta)
	sub.SetXAlign(0)
	sub.AddCSSClass("chatot-device-meta")
	col.Append(sub)
	row.Append(col)

	if onUnlink != nil {
		btn := gtk.NewButtonWithLabel("Unlink")
		btn.AddCSSClass("chatot-outline-btn")
		btn.AddCSSClass("chatot-outline-btn-danger")
		btn.SetVAlign(gtk.AlignCenter)
		btn.ConnectClicked(onUnlink)
		row.Append(btn)
	}
	return row
}

// dialogBody is the standard padded column a card dialog's content sits in.
func dialogBody(spacing int) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, spacing)
	box.AddCSSClass("chatot-dialog-body")
	return box
}
