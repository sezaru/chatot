package ui

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// choiceOption is one row of the mockup's choice dialog: a label, a tick when
// it is the current setting, and the value it selects.
type choiceOption struct {
	Label   string
	Seconds int64
	Current bool
}

// disappearingChoices are the disappearing-message durations the mockup
// offers, with the one matching current (in seconds) ticked. An unrecognised
// timer ticks nothing rather than guessing. The labels and durations are the
// same pair the new-group dropdown uses.
func disappearingChoices(current int64) []choiceOption {
	opts := make([]choiceOption, 0, len(disappearingOptions))
	for i, label := range disappearingOptions {
		seconds := disappearingSecondsForIndex(i)
		opts = append(opts, choiceOption{
			Label:   label,
			Seconds: seconds,
			Current: seconds == current,
		})
	}
	return opts
}

// blockConfirmText is the block confirmation's title and body for a contact.
func blockConfirmText(name string) (title, body string) {
	if name == "" {
		name = "this contact"
	}
	return fmt.Sprintf("Block %s?", name),
		"Blocked contacts can't message or call you, and won't see when you're online. They aren't told."
}

// showChoiceDialog presents the mockup's choice card: a title, an explanatory
// body, then a bordered list of options with the current one ticked. onPick
// receives the chosen option and the dialog closes.
func showChoiceDialog(parent *gtk.Window, title, body string, options []choiceOption, onPick func(choiceOption)) {
	dialog := newCardDialog()
	dialog.SetTitle(title)
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(360, -1)

	content := gtk.NewBox(gtk.OrientationVertical, 6)
	content.SetMarginTop(16)
	content.SetMarginStart(20)
	content.SetMarginEnd(20)
	content.SetMarginBottom(14)

	bodyLabel := gtk.NewLabel(body)
	bodyLabel.AddCSSClass("chatot-choice-body")
	bodyLabel.SetXAlign(0)
	bodyLabel.SetWrap(true)
	content.Append(bodyLabel)

	list := gtk.NewBox(gtk.OrientationVertical, 0)
	list.AddCSSClass("chatot-choice-list")
	list.SetMarginTop(8)
	for _, opt := range options {
		row := gtk.NewBox(gtk.OrientationHorizontal, 11)
		label := gtk.NewLabel(opt.Label)
		label.SetXAlign(0)
		label.SetHExpand(true)
		row.Append(label)
		if opt.Current {
			tick := newCheckGlyph(14, true)
			tick.AddCSSClass("chatot-choice-tick")
			row.Append(tick)
		}

		btn := gtk.NewButton()
		btn.SetChild(row)
		btn.AddCSSClass("flat")
		btn.AddCSSClass("chatot-choice-row")
		picked := opt
		btn.ConnectClicked(func() {
			dialog.Close()
			onPick(picked)
		})
		list.Append(btn)
	}
	content.Append(list)

	dialog.SetChild(content)
	dialog.Present()
}

// showDisappearingDialog offers the chat's disappearing-message durations,
// for groups and 1:1 chats alike (the client's call is whatsmeow's generic
// SetDisappearingTimer). onSet, when non-nil, hears the applied value on the
// main loop so the caller can show it back.
func showDisappearingDialog(parent *gtk.Window, c client.Client, jid string, current int64, onSet func(seconds int64)) {
	showChoiceDialog(parent, "Disappearing messages",
		"New messages in this chat vanish from both devices after the timer. Messages already sent are unaffected.",
		disappearingChoices(current),
		func(opt choiceOption) {
			seconds := opt.Seconds
			go func() {
				if err := c.SetGroupDisappearingTimer(context.Background(), jid, seconds); err != nil {
					log.Printf("chatot: set disappearing timer failed: %v", err)
					return
				}
				if onSet != nil {
					glib.IdleAdd(func() { onSet(seconds) })
				}
			}()
		})
}

// muteChoices are the mockup's mute durations; Seconds 0 is "Always".
func muteChoices() []choiceOption {
	return []choiceOption{
		{Label: "8 hours", Seconds: 8 * 3600},
		{Label: "1 week", Seconds: 7 * 24 * 3600},
		{Label: "Always", Seconds: 0},
	}
}

// showMuteDialog offers the mockup's "Mute <name>" durations and applies the
// pick; a timed mute goes through MuteChatFor, "Always" through MuteChat.
func showMuteDialog(parent *gtk.Window, c client.Client, jid, name string) {
	if name == "" {
		name = "chat"
	}
	showChoiceDialog(parent, "Mute "+name,
		"Notifications stay off for this chat until the time is up. Messages still arrive.",
		muteChoices(),
		func(opt choiceOption) {
			seconds := opt.Seconds
			go func() {
				var err error
				if seconds > 0 {
					err = c.MuteChatFor(context.Background(), jid, time.Duration(seconds)*time.Second)
				} else {
					err = c.MuteChat(context.Background(), jid, true)
				}
				if err != nil {
					log.Printf("chatot: mute chat failed: %v", err)
				}
			}()
		})
}

// disappearingValueLabel is the info card's trailing value for a timer in
// seconds: the option's own label, or "Off".
func disappearingValueLabel(seconds int64) string {
	for _, opt := range disappearingChoices(seconds) {
		if opt.Current && opt.Seconds != 0 {
			return opt.Label
		}
	}
	return "Off"
}

// showBlockConfirmDialog asks before blocking a contact, per the mockup:
// Cancel, then a red Block.
func showBlockConfirmDialog(parent *gtk.Window, c client.Client, jid, name string) {
	title, body := blockConfirmText(name)

	dialog := newCardDialog()
	dialog.SetTitle(title)
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(380, -1)

	content := gtk.NewBox(gtk.OrientationVertical, 14)
	content.SetMarginTop(16)
	content.SetMarginStart(20)
	content.SetMarginEnd(20)
	content.SetMarginBottom(16)

	bodyLabel := gtk.NewLabel(body)
	bodyLabel.AddCSSClass("chatot-choice-body")
	bodyLabel.SetXAlign(0)
	bodyLabel.SetWrap(true)
	content.Append(bodyLabel)

	buttons := gtk.NewBox(gtk.OrientationHorizontal, 8)
	buttons.SetHAlign(gtk.AlignEnd)

	cancel := gtk.NewButtonWithLabel("Cancel")
	cancel.ConnectClicked(func() { dialog.Close() })
	buttons.Append(cancel)

	block := gtk.NewButtonWithLabel("Block")
	block.AddCSSClass("destructive-action")
	block.ConnectClicked(func() {
		dialog.Close()
		go func() {
			if err := c.SetBlocked(context.Background(), jid, true); err != nil {
				log.Printf("chatot: block contact failed: %v", err)
			}
		}()
	})
	buttons.Append(block)

	content.Append(buttons)
	dialog.SetChild(content)
	dialog.Present()
}
