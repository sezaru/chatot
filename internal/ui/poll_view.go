package ui

import (
	"fmt"

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
	v.MultiVote = p.SelectableCount != 1
	switch {
	case p.SelectableCount == 1:
		v.SelectHint = "Select one"
	case p.SelectableCount > 1:
		v.SelectHint = fmt.Sprintf("Select up to %d", p.SelectableCount)
	default:
		v.SelectHint = "Select any"
	}
	return v
}

// buildPollContent renders a poll bubble: a 📊 question, a select hint, and one
// vote button per option (label + count). Clicking an option casts a vote for
// just that option via onVote (single-select semantics — the simplest useful
// behaviour; multi-select refinement is out of scope).
func buildPollContent(msg client.Message, v pollView, onVote func(msg client.Message, options []string)) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.AddCSSClass("chatot-bubble-poll")

	question := gtk.NewLabel("📊 " + v.Question)
	question.SetXAlign(0)
	question.SetWrap(true)
	question.AddCSSClass("chatot-poll-question")
	box.Append(question)

	hint := gtk.NewLabel(v.SelectHint)
	hint.SetXAlign(0)
	hint.AddCSSClass("chatot-poll-hint")
	box.Append(hint)

	for _, opt := range v.Options {
		label := fmt.Sprintf("%s  (%d)", opt.Name, opt.Count)
		if opt.Voted {
			label = "✓ " + label
		}
		btn := gtk.NewButtonWithLabel(label)
		btn.AddCSSClass("flat")
		btn.SetHAlign(gtk.AlignFill)
		if opt.Voted {
			btn.AddCSSClass("suggested-action")
		}
		if onVote != nil {
			name := opt.Name
			btn.ConnectClicked(func() { onVote(msg, []string{name}) })
		}
		box.Append(btn)
	}

	total := gtk.NewLabel(fmt.Sprintf("%d vote(s)", v.Total))
	total.SetXAlign(0)
	total.AddCSSClass("chatot-poll-total")
	box.Append(total)

	return box
}
