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
	Typing     bool // set by ChatList.refresh from live presence, not chatRowVM
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

// searchResultLimit caps how many hits the sidebar search shows at once.
const searchResultLimit = 30

// ChatList is the sidebar pane: a search entry over a live list of chats
// backed by a client.Client, with a selection callback for the content
// pane. Below the search entry a single ListBox holds either the normal
// chat rows or, while a query is active, search-hit rows; both row kinds
// resolve to a ChatJID in rowJIDs, so activation and onSelect are shared.
type ChatList struct {
	*gtk.Box

	c          client.Client
	list       *gtk.ListBox
	rowJIDs    []string // row index -> JID, rebuilt alongside the ListBox rows
	onSelect   func(jid string)
	typingJIDs map[string]bool // chat JID -> peer currently composing
	query      string          // current search text; "" shows the normal chat list
}

// NewChatList builds a ChatList for c and populates it with the current
// chats. It also subscribes to c.Events() to keep the list live.
func NewChatList(c client.Client) *ChatList {
	root := gtk.NewBox(gtk.OrientationVertical, 0)

	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search chats and messages")
	search.AddCSSClass("chatot-search-entry")
	root.Append(search)

	list := gtk.NewListBox()
	list.AddCSSClass("navigation-sidebar")
	list.SetVExpand(true)
	root.Append(list)

	cl := &ChatList{Box: root, c: c, list: list, typingJIDs: make(map[string]bool)}

	list.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx < 0 || idx >= len(cl.rowJIDs) {
			return
		}
		if cl.onSelect != nil {
			cl.onSelect(cl.rowJIDs[idx])
		}
	})

	search.ConnectSearchChanged(func() {
		cl.query = strings.TrimSpace(search.Text())
		cl.refresh()
	})

	cl.refresh()
	go cl.watchEvents()

	return cl
}

// OnChatSelected registers f to be called with the JID of the activated row.
func (cl *ChatList) OnChatSelected(f func(jid string)) {
	cl.onSelect = f
}

// refresh rebuilds the row widgets from either Search (query set) or Chats
// (query empty). Must run on the GTK main loop.
func (cl *ChatList) refresh() {
	if cl.query != "" {
		cl.refreshSearch()
		return
	}
	cl.refreshChats()
}

func (cl *ChatList) refreshChats() {
	chats, err := cl.c.Chats(0)
	if err != nil {
		return
	}

	cl.list.RemoveAll()

	now := time.Now()
	cl.rowJIDs = make([]string, 0, len(chats))
	for _, chat := range chats {
		vm := chatRowVM(chat, now)
		if cl.typingJIDs[chat.JID] {
			vm.Preview = "typing…"
			vm.Typing = true
		}
		cl.list.Append(buildChatRow(vm))
		cl.rowJIDs = append(cl.rowJIDs, vm.JID)
	}
}

// refreshSearch queries Search and rebuilds the list as result rows.
// Clicking a result opens its chat via the same onSelect path as a normal
// chat row; jumping to the exact matched message is left for later
// (ConversationView.Load already lands the reader at the newest messages).
func (cl *ChatList) refreshSearch() {
	hits, err := cl.c.Search(cl.query, searchResultLimit)
	if err != nil {
		hits = nil
	}

	cl.list.RemoveAll()

	if len(hits) == 0 {
		cl.rowJIDs = nil
		empty := gtk.NewLabel("No results")
		empty.AddCSSClass("chatot-search-empty")
		cl.list.Append(empty)
		return
	}

	now := time.Now()
	cl.rowJIDs = make([]string, 0, len(hits))
	for _, h := range hits {
		cl.list.Append(buildSearchHitRow(searchHitVM(h, now)))
		cl.rowJIDs = append(cl.rowJIDs, h.ChatJID)
	}
}

// watchEvents listens for client events and schedules a list rebuild on the
// GTK main loop via glib.IdleAdd. Runs on its own goroutine for the
// lifetime of the process; the fake/whatsmeow Events() channel is never
// explicitly closed today, so this goroutine simply exits if it is.
// EventChatPresence updates typingJIDs (composing sets it, anything else
// clears it) instead of falling through to the generic full refresh, since
// it needs the event's JID+state before rebuilding rows.
func (cl *ChatList) watchEvents() {
	for ev := range cl.c.Events() {
		if ev.Kind == client.EventChatPresence && ev.ChatPresence != nil {
			jid, typing := ev.ChatPresence.ChatJID, ev.ChatPresence.State == "composing"
			glib.IdleAdd(func() {
				if typing {
					cl.typingJIDs[jid] = true
				} else {
					delete(cl.typingJIDs, jid)
				}
				cl.refresh()
			})
			continue
		}
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
	if vm.Typing {
		previewLabel.AddCSSClass("chatot-chat-typing")
	}
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

// searchHitView holds the pure, pre-rendered display fields for one search
// result row. searchHitVM computes it from a client.SearchHit so it can be
// unit-tested without a display.
type searchHitView struct {
	ChatJID  string
	ChatName string
	Snippet  string
	TimeText string
	Initial  string
}

// searchHitVM derives the display view-model for a single search hit. now
// is injected so time formatting is deterministic in tests.
func searchHitVM(h client.SearchHit, now time.Time) searchHitView {
	name := h.ChatName
	if name == "" {
		name = h.ChatJID
	}
	initial := "?"
	for _, r := range name {
		initial = strings.ToUpper(string(r))
		break
	}
	return searchHitView{
		ChatJID:  h.ChatJID,
		ChatName: name,
		Snippet:  h.Snippet,
		TimeText: formatChatTime(h.TS, now),
		Initial:  initial,
	}
}

// buildSearchHitRow constructs the GTK widget tree for a single search
// result row from its pre-computed view-model.
func buildSearchHitRow(vm searchHitView) *gtk.Box {
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

	nameLabel := gtk.NewLabel(vm.ChatName)
	nameLabel.SetXAlign(0)
	nameLabel.AddCSSClass("chatot-chat-name")
	textCol.Append(nameLabel)

	snippetLabel := gtk.NewLabel(vm.Snippet)
	snippetLabel.SetXAlign(0)
	snippetLabel.SetWrap(true)
	snippetLabel.SetLines(2)
	snippetLabel.SetEllipsize(pango.EllipsizeEnd)
	snippetLabel.AddCSSClass("chatot-search-snippet")
	textCol.Append(snippetLabel)

	row.Append(textCol)

	timeLabel := gtk.NewLabel(vm.TimeText)
	timeLabel.AddCSSClass("chatot-chat-time")
	timeLabel.SetVAlign(gtk.AlignStart)
	row.Append(timeLabel)

	return row
}
