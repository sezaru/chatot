package ui

import (
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// viewedByText is the "Viewed by N" pill and dialog title.
func viewedByText(n int) string { return "Viewed by " + strconv.Itoa(n) }

// showStatusViewers opens "Viewed by N": everyone whose read receipt for
// any of items reached this device, earliest first. WhatsApp keeps no
// viewer list a linked device could ask for, so views that landed while
// chatot was closed are missing; the card says so.
func (cl *ChatList) showStatusViewers(items []client.Message) {
	c := cl.c
	go func() {
		viewers, err := collectStatusViewers(c, items)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: status viewers: %v", err)
				cl.toast("Couldn't load who viewed your status")
				return
			}
			cl.presentStatusViewers(viewers)
		})
	}()
}

// collectStatusViewers unions the viewers of items, keeping each contact's
// earliest view, ordered by that time.
func collectStatusViewers(c client.Client, items []client.Message) ([]client.StatusViewer, error) {
	earliest := map[string]client.StatusViewer{}
	for _, it := range items {
		vs, err := c.StatusViewers(it.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			if cur, ok := earliest[v.JID]; !ok || v.TS < cur.TS {
				earliest[v.JID] = v
			}
		}
	}
	out := make([]client.StatusViewer, 0, len(earliest))
	for _, v := range earliest {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TS != out[j].TS {
			return out[i].TS < out[j].TS
		}
		return out[i].JID < out[j].JID
	})
	return out, nil
}

func (cl *ChatList) presentStatusViewers(viewers []client.StatusViewer) {
	dialog := newCardDialog()
	dialog.SetTitle(viewedByText(len(viewers)))
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(380, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := dialogBody(10)
	if len(viewers) == 0 {
		body.Append(newDialogBodyText("No views have reached this device yet."))
	} else {
		names := chatNames(cl.c)
		now := time.Now()
		card := newSettingsCard()
		for _, v := range viewers {
			card.Add(cl.statusViewerRow(v, names, now))
		}
		body.Append(card)
	}
	hint := gtk.NewLabel("Views only arrive while chatot is connected; your phone keeps the full list.")
	hint.SetXAlign(0)
	hint.SetWrap(true)
	hint.AddCSSClass("chatot-dialog-hint")
	body.Append(hint)
	box.Append(body)

	footer := newDialogFooter()
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	footer.Append(spacer)
	footer.Append(newPrimaryButton("Done", func() { dialog.Close() }))
	box.Append(footer)
	dialog.SetChild(box)
	dialog.Present()
}

// statusViewerRow is one viewer: avatar, name and when they looked.
func (cl *ChatList) statusViewerRow(v client.StatusViewer, names map[string]string, now time.Time) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-card-row")
	name := posterName(v.JID, names)
	row.Append(buildAvatar(cl.c, cl.avatarCache, v.JID, initialFor(name), 32))
	label := gtk.NewLabel(name)
	label.SetXAlign(0)
	label.SetHExpand(true)
	label.SetEllipsize(pango.EllipsizeEnd)
	row.Append(label)
	when := gtk.NewLabel(formatChatTime(v.TS, now))
	when.AddCSSClass("chatot-card-sub")
	row.Append(when)
	return row
}
