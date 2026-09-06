package ui

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// callView is the display model for a call logged in the thread: WhatsApp's
// wording for what happened ("Missed voice call") beside a glyph for the
// medium, with the length of a call that connected.
type callView struct {
	IsCall bool
	Title  string // "Missed voice call", "Video call", ...
	Glyph  string // 📞 for voice, 🎥 for video
	Missed bool   // the title reads in the alert colour
	Detail string // "2:31" for a connected call, "" otherwise
}

// callVM derives the call view-model for m (zero value if m logs no call).
func callVM(m client.Message) callView {
	if m.CallLog == nil {
		return callView{}
	}
	cl := *m.CallLog
	v := callView{
		IsCall: true,
		Title:  client.CallText(cl.Video, cl.Outcome),
		Glyph:  "📞",
		Missed: cl.Outcome == client.CallMissed,
	}
	if cl.Video {
		v.Glyph = "🎥"
	}
	if cl.Outcome == client.CallAnswered && cl.DurationSecs > 0 {
		v.Detail = callDurationText(cl.DurationSecs)
	}
	return v
}

// callDurationText renders a call's length as m:ss (h:mm:ss past an hour).
func callDurationText(secs int) string {
	if secs >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs%3600/60, secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// buildCallContent renders the call row inside its bubble: the glyph in a
// tinted disc beside the title, with the duration under it when there is one.
func buildCallContent(v callView) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-call-row")

	glyph := gtk.NewLabel(v.Glyph)
	glyph.AddCSSClass("chatot-call-glyph")
	glyph.SetVAlign(gtk.AlignCenter)
	row.Append(glyph)

	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetVAlign(gtk.AlignCenter)
	title := gtk.NewLabel(v.Title)
	title.SetXAlign(0)
	title.AddCSSClass("chatot-call-title")
	if v.Missed {
		title.AddCSSClass("chatot-call-missed")
	}
	col.Append(title)
	if v.Detail != "" {
		detail := gtk.NewLabel(v.Detail)
		detail.SetXAlign(0)
		detail.AddCSSClass("chatot-call-detail")
		col.Append(detail)
	}
	row.Append(col)
	return row
}
