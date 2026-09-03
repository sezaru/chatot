package ui

import (
	"fmt"
	"math"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// pollView is the pure display view-model for a poll bubble, computed from a
// client.Message so it's unit-testable without a display.
type pollView struct {
	IsPoll     bool
	Question   string // "Poll" when the poll carries no name
	Options    []pollOptionView
	Total      int    // total votes cast across all options
	MultiVote  bool   // more than one option may be selected
	HasVoted   bool   // the local user has voted on this poll
	SelectHint string // e.g. "Select one" / "Select up to 2"
}

// pollOptionView is one rendered poll option.
type pollOptionView struct {
	Name    string
	Count   int
	Voted   bool
	Percent int // share of Total, 0 when no votes
}

// pollVM derives the poll view-model for m (zero value if m carries no poll).
func pollVM(m client.Message) pollView {
	if m.Poll == nil {
		return pollView{}
	}
	p := *m.Poll
	v := pollView{IsPoll: true, Question: p.Name}
	if v.Question == "" {
		v.Question = "Poll"
	}
	for _, o := range p.Options {
		v.Total += o.Count
		if o.Voted {
			v.HasVoted = true
		}
	}
	v.Options = make([]pollOptionView, len(p.Options))
	for i, o := range p.Options {
		pct := 0
		if v.Total > 0 {
			pct = int(float64(o.Count)/float64(v.Total)*100 + 0.5)
		}
		v.Options[i] = pollOptionView{Name: o.Name, Count: o.Count, Voted: o.Voted, Percent: pct}
	}
	// The mockup's footer says "Select one" or "Select one or more"; the
	// exact cap is WhatsApp's business, not the reader's.
	v.MultiVote = p.SelectableCount != 1
	if v.MultiVote {
		v.SelectHint = "Select one or more"
	} else {
		v.SelectHint = "Select one"
	}
	return v
}

// pollFooter is the mockup's dim line under a poll: the select hint and the
// total vote count, e.g. "Select one · 4 votes".
func pollFooter(v pollView) string {
	unit := "votes"
	if v.Total == 1 {
		unit = "vote"
	}
	return fmt.Sprintf("%s · %d %s", v.SelectHint, v.Total, unit)
}

// buildPollContent renders the mockup's poll bubble: the bare question in
// bold, then one row per option — a 16px tickbox, the label, the vote count,
// and a 4px accent progress bar under it — closed by a hairline footer
// carrying the select hint and total. Clicking a row submits the new
// selection via onVote (see pollSelection).
func buildPollContent(msg client.Message, v pollView, onVote func(msg client.Message, options []string)) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 7)
	box.AddCSSClass("chatot-bubble-poll")
	box.SetSizeRequest(270, -1)

	// The mockup leads a poll with the bare question, no 📊 prefix.
	question := gtk.NewLabel(v.Question)
	question.SetXAlign(0)
	question.SetWrap(true)
	question.AddCSSClass("chatot-poll-question")
	box.Append(question)

	for _, opt := range v.Options {
		box.Append(buildPollOption(msg, v, opt, onVote))
	}

	footer := gtk.NewLabel(pollFooter(v))
	footer.SetXAlign(0)
	footer.AddCSSClass("chatot-poll-foot")
	box.Append(footer)

	return box
}

// pollSelection is what a click on option name submits: on a single-choice
// poll that option alone, or nothing when it is the one already picked
// (retracting the vote); on a multi-choice poll the current picks with
// name toggled.
func pollSelection(v pollView, name string) []string {
	if !v.MultiVote {
		for _, o := range v.Options {
			if o.Name == name && o.Voted {
				return nil
			}
		}
		return []string{name}
	}
	var out []string
	for _, o := range v.Options {
		if o.Voted != (o.Name == name) {
			out = append(out, o.Name)
		}
	}
	return out
}

// buildPollOption is one option row: tickbox · label · count over the share bar.
func buildPollOption(msg client.Message, v pollView, opt pollOptionView, onVote func(msg client.Message, options []string)) gtk.Widgetter {
	col := gtk.NewBox(gtk.OrientationVertical, 3)

	row := gtk.NewBox(gtk.OrientationHorizontal, 8)

	tick := newCheckGlyph(16, opt.Voted)
	tick.AddCSSClass("chatot-poll-box")
	if opt.Voted {
		tick.AddCSSClass("chatot-poll-box-on")
	}
	row.Append(tick)

	name := gtk.NewLabel(opt.Name)
	name.SetXAlign(0)
	name.SetHExpand(true)
	name.SetWrap(true)
	name.AddCSSClass("chatot-poll-option")
	row.Append(name)

	count := gtk.NewLabel(fmt.Sprintf("%d", opt.Count))
	count.AddCSSClass("chatot-poll-count")
	count.SetVAlign(gtk.AlignCenter)
	row.Append(count)
	col.Append(row)

	// The share bar is drawn, not a GtkProgressBar: Adwaita's bar carries
	// its own min-height and margins that no CSS override fully removes.
	bar := gtk.NewDrawingArea()
	bar.SetContentHeight(pollBarH)
	bar.SetHExpand(true)
	bar.AddCSSClass("chatot-poll-bar")
	fraction := float64(opt.Percent) / 100
	onGreen := msg.FromMe
	bar.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		fg := bar.StyleContext().Color()
		drawPollBar(cr, float64(w), float64(h), fraction, onGreen, float64(fg.Red()), float64(fg.Green()), float64(fg.Blue()))
	})
	col.Append(bar)

	if onVote == nil {
		return col
	}
	btn := gtk.NewButton()
	btn.SetChild(col)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btn")
	// No keyboard focus on click: the row is rebuilt when the tally lands,
	// and a focused widget vanishing makes the list view jump to its top.
	btn.SetFocusOnClick(false)
	name2 := opt.Name
	btn.ConnectClicked(func() { onVote(msg, pollSelection(v, name2)) })
	return btn
}

// pollBarH is the mockup's share bar height.
const pollBarH = 4

// drawPollBar paints the option's share: a pill track in the bubble's text
// colour at low alpha (white at .25 on green) with an accent (white on
// green) fill fraction wide.
func drawPollBar(cr *cairo.Context, w, h, fraction float64, onGreen bool, r, g, b float64) {
	pollPill(cr, 0, 0, w, h)
	if onGreen {
		cr.SetSourceRGBA(1, 1, 1, 0.25)
	} else {
		cr.SetSourceRGBA(r, g, b, 0.14)
	}
	cr.Fill()
	fw := w * clampF(fraction, 0, 1)
	if fw <= 0 {
		return
	}
	pollPill(cr, 0, 0, math.Max(fw, h), h)
	if onGreen {
		cr.SetSourceRGB(1, 1, 1)
	} else {
		cr.SetSourceRGB(0x1b/255.0, 0x8c/255.0, 0x72/255.0)
	}
	cr.Fill()
}

// pollPill traces a fully rounded rectangle.
func pollPill(cr *cairo.Context, x, y, w, h float64) {
	r := h / 2
	cr.NewSubPath()
	cr.Arc(x+w-r, y+r, r, -math.Pi/2, math.Pi/2)
	cr.Arc(x+r, y+r, r, math.Pi/2, 3*math.Pi/2)
	cr.ClosePath()
}
