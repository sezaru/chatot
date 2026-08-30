package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// chatRowView holds the pure, pre-rendered display fields for one chat row.
// chatRowVM computes it from a client.Chat so it can be unit-tested without
// a display.
type chatRowView struct {
	JID        string
	Initial    string
	Name       string
	Preview    string
	TimeText   string
	UnreadText string
	ShowUnread bool
}

// chatRowVM derives the display view-model for a single chat row. now is
// injected so time formatting is deterministic in tests.
func chatRowVM(c client.Chat, now time.Time) chatRowView {
	name := c.Name
	if name == "" {
		name = "Unknown"
	}

	initial := "?"
	for _, r := range name {
		initial = strings.ToUpper(string(r))
		break
	}

	view := chatRowView{
		JID:      c.JID,
		Initial:  initial,
		Name:     name,
		Preview:  c.Preview,
		TimeText: formatChatTime(c.LastMessageTS, now),
	}
	if c.UnreadCount > 0 {
		view.ShowUnread = true
		if c.UnreadCount > 99 {
			view.UnreadText = "99+"
		} else {
			view.UnreadText = strconv.Itoa(c.UnreadCount)
		}
	}
	return view
}

// formatChatTime renders ts (unix seconds) relative to now: "15:04" for
// today, weekday name for the last 7 days, "02/01" otherwise.
func formatChatTime(ts int64, now time.Time) string {
	if ts == 0 {
		return ""
	}
	t := time.Unix(ts, 0).In(now.Location())
	daysSince := int(now.Sub(t).Hours() / 24)

	sameDay := t.Year() == now.Year() && t.YearDay() == now.YearDay()
	if sameDay {
		return t.Format("15:04")
	}
	if daysSince >= 0 && daysSince < 7 {
		return t.Format("Monday")
	}
	return t.Format("02/01")
}

// ChatList is the sidebar pane: a live list of chats backed by a
// client.Client, with a selection callback for the content pane.
type ChatList struct {
	*gtk.ListBox

	c        client.Client
	rowJIDs  []string // row index -> JID, rebuilt alongside the ListBox rows
	onSelect func(jid string)
}

// NewChatList builds a ChatList for c and populates it with the current
// chats. It also subscribes to c.Events() to keep the list live.
func NewChatList(c client.Client) *ChatList {
	box := gtk.NewListBox()
	box.AddCSSClass("navigation-sidebar")

	cl := &ChatList{ListBox: box, c: c}

	box.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx < 0 || idx >= len(cl.rowJIDs) {
			return
		}
		if cl.onSelect != nil {
			cl.onSelect(cl.rowJIDs[idx])
		}
	})

	cl.refresh()
	go cl.watchEvents()

	return cl
}

// OnChatSelected registers f to be called with the JID of the activated row.
func (cl *ChatList) OnChatSelected(f func(jid string)) {
	cl.onSelect = f
}

// refresh re-queries Chats() and rebuilds the row widgets. Must run on the
// GTK main loop.
func (cl *ChatList) refresh() {
	chats, err := cl.c.Chats(0)
	if err != nil {
		return
	}

	cl.RemoveAll()

	now := time.Now()
	cl.rowJIDs = make([]string, 0, len(chats))
	for _, chat := range chats {
		vm := chatRowVM(chat, now)
		cl.Append(buildChatRow(vm))
		cl.rowJIDs = append(cl.rowJIDs, vm.JID)
	}
}

// watchEvents listens for client events and schedules a list rebuild on the
// GTK main loop via glib.IdleAdd. Runs on its own goroutine for the
// lifetime of the process; the fake/whatsmeow Events() channel is never
// explicitly closed today, so this goroutine simply exits if it is.
func (cl *ChatList) watchEvents() {
	for range cl.c.Events() {
		glib.IdleAdd(func() {
			cl.refresh()
		})
	}
}

// buildChatRow constructs the GTK widget tree for a single row from its
// pre-computed view-model.
func buildChatRow(vm chatRowView) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.SetMarginTop(6)
	row.SetMarginBottom(6)
	row.SetMarginStart(8)
	row.SetMarginEnd(8)

	avatar := gtk.NewLabel(vm.Initial)
	avatar.AddCSSClass("chatot-avatar")
	avatar.SetSizeRequest(36, 36)
	row.Append(avatar)

	textCol := gtk.NewBox(gtk.OrientationVertical, 2)
	textCol.SetHExpand(true)

	nameLabel := gtk.NewLabel(vm.Name)
	nameLabel.SetXAlign(0)
	nameLabel.AddCSSClass("chatot-chat-name")
	textCol.Append(nameLabel)

	previewLabel := gtk.NewLabel(vm.Preview)
	previewLabel.SetXAlign(0)
	previewLabel.SetWrap(true)
	previewLabel.SetLines(2)
	previewLabel.SetEllipsize(pango.EllipsizeEnd)
	previewLabel.AddCSSClass("chatot-chat-preview")
	textCol.Append(previewLabel)

	row.Append(textCol)

	metaCol := gtk.NewBox(gtk.OrientationVertical, 4)
	metaCol.SetVAlign(gtk.AlignStart)

	timeLabel := gtk.NewLabel(vm.TimeText)
	timeLabel.AddCSSClass("chatot-chat-time")
	metaCol.Append(timeLabel)

	if vm.ShowUnread {
		badge := gtk.NewLabel(vm.UnreadText)
		badge.AddCSSClass("chatot-unread-badge")
		metaCol.Append(badge)
	}

	row.Append(metaCol)

	return row
}
