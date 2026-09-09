package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// chatMaxWidth is the widest the thread's rows and the composer grow, in
// px. As on WhatsApp Web, the conversation column itself keeps growing
// with the window (its header, wallpaper and hairlines span it all), but
// past this width the messages and the input stop and sit centred.
const chatMaxWidth = 1100

// chatClamp wraps w so it fills the column up to chatMaxWidth and stays
// centred at that width beyond it.
func chatClamp(w gtk.Widgetter) *adw.Clamp {
	c := adw.NewClamp()
	c.SetUnit(adw.LengthUnitPx)
	c.SetMaximumSize(chatMaxWidth)
	c.SetTighteningThreshold(chatMaxWidth)
	c.SetChild(w)
	return c
}

// threadRowBox is the bubble box inside a thread row (a chatClamp around
// it, see newThreadList).
func threadRowBox(item *gtk.ListItem) (*gtk.Box, bool) {
	clamp, ok := item.Child().(*adw.Clamp)
	if !ok {
		return nil, false
	}
	box, ok := clamp.Child().(*gtk.Box)
	return box, ok
}
