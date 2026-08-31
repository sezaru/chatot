package ui

import (
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// eventView is the pure display view-model for a scheduled-event bubble,
// computed from a client.Message so it's unit-testable without a display.
type eventView struct {
	IsEvent  bool
	Title    string // event Name, or "Event" when unnamed
	When     string // formatted start (- end) time, "" when StartTS is unset
	Location string
	Canceled bool
}

// eventVM derives the event view-model for m (zero value if m carries no
// event invite).
func eventVM(m client.Message) eventView {
	if m.EventInvite == nil {
		return eventView{}
	}
	ev := *m.EventInvite
	title := ev.Name
	if title == "" {
		title = "Event"
	}
	return eventView{
		IsEvent:  true,
		Title:    title,
		When:     eventWhenText(ev.StartTS, ev.EndTS),
		Location: ev.Location,
		Canceled: ev.Canceled,
	}
}

// eventWhenText formats an event's start (and, if present, end) time in UTC
// (matching the location bubble's LiveUntil formatting, so it's deterministic
// regardless of the host's timezone), e.g. "Mon, 02 Jan 2006 15:04" or
// "... - 17:04" when the event carries an end time on the same day. Returns
// "" when startTS is unset.
func eventWhenText(startTS, endTS int64) string {
	if startTS == 0 {
		return ""
	}
	start := time.Unix(startTS, 0).UTC()
	text := start.Format("Mon, 02 Jan 2006 15:04")
	if endTS == 0 {
		return text
	}
	end := time.Unix(endTS, 0).UTC()
	if sameDay(startTS, endTS, time.UTC) {
		return text + " - " + end.Format("15:04")
	}
	return text + " - " + end.Format("Mon, 02 Jan 2006 15:04")
}

// buildEventContent renders an event bubble: a 📅 title, the formatted
// start/end time, and an optional location line. A canceled event renders
// its title struck through via CSS.
func buildEventContent(v eventView) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 2)
	box.AddCSSClass("chatot-bubble-event")

	title := gtk.NewLabel("📅 " + v.Title)
	title.SetXAlign(0)
	title.SetWrap(true)
	title.AddCSSClass("chatot-event-title")
	if v.Canceled {
		title.AddCSSClass("chatot-event-canceled")
	}
	box.Append(title)

	if v.When != "" {
		when := gtk.NewLabel(v.When)
		when.SetXAlign(0)
		when.AddCSSClass("chatot-event-when")
		box.Append(when)
	}

	if v.Location != "" {
		loc := gtk.NewLabel("📍 " + v.Location)
		loc.SetXAlign(0)
		loc.SetWrap(true)
		loc.AddCSSClass("chatot-event-location")
		box.Append(loc)
	}

	return box
}
