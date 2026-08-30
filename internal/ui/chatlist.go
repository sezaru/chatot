package ui

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
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
	Pinned     bool
	Muted      bool
	Blocked    bool // set by ChatList.refreshChats via c.IsBlocked, not chatRowVM (pure)
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
		Pinned:   c.Pinned,
		Muted:    c.Muted,
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

// chatActionLabelsView holds the pure per-chat context-menu label choices,
// reflecting the chat's current pin/mute/archive/unread state.
type chatActionLabelsView struct {
	Pin     string
	Mute    string
	Archive string
	Unread  string
}

// chatActionLabels derives the context-menu labels for c: each toggles the
// opposite of its current state (e.g. a pinned chat gets "Unpin").
func chatActionLabels(c client.Chat) chatActionLabelsView {
	v := chatActionLabelsView{Pin: "Pin", Mute: "Mute", Archive: "Archive", Unread: "Mark as unread"}
	if c.Pinned {
		v.Pin = "Unpin"
	}
	if c.Muted {
		v.Mute = "Unmute"
	}
	if c.Archived {
		v.Archive = "Unarchive"
	}
	if c.UnreadCount > 0 {
		v.Unread = "Mark as read"
	}
	return v
}

// blockActionLabel is the pure label for the context menu's block/unblock
// entry, reflecting the contact's current blocked state.
func blockActionLabel(blocked bool) string {
	if blocked {
		return "Unblock"
	}
	return "Block"
}

// showChatInList is the archived-filter predicate: with the toggle off, only
// non-archived chats show; with it on, only archived ones do.
func showChatInList(c client.Chat, showArchived bool) bool {
	return c.Archived == showArchived
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

	c            client.Client
	events       <-chan client.Event
	list         *gtk.ListBox
	rowJIDs      []string // row index -> JID, rebuilt alongside the ListBox rows
	onSelect     func(jid string)
	typingJIDs   map[string]bool // chat JID -> peer currently composing
	query        string          // current search text; "" shows the normal chat list
	avatarCache  *avatarCache
	window       *gtk.Window // parent for the new-chat dialog; set via SetWindow
	showArchived bool        // toggled by the "Archived" button; see showChatInList
	showStarred  bool        // toggled by the "Starred" button; overrides search/archived
	labelFilter  *gtk.DropDown
	// filterLabelIDs maps the label-filter dropdown's row index to a label id;
	// index 0 is "All" (empty id). Rebuilt with the dropdown from c.Labels().
	filterLabelIDs []string
	selectedLabel  string // label id the chat list is filtered to; "" = All
}

// SetWindow supplies the parent window the new-chat dialog needs; call once
// after the top-level window exists.
func (cl *ChatList) SetWindow(w *gtk.Window) { cl.window = w }

// NewChatList builds a ChatList for c and populates it with the current
// chats. It also subscribes to c.Events() to keep the list live.
func NewChatList(c client.Client) *ChatList {
	root := gtk.NewBox(gtk.OrientationVertical, 0)

	searchRow := gtk.NewBox(gtk.OrientationHorizontal, 4)

	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search chats and messages")
	search.AddCSSClass("chatot-search-entry")
	search.SetHExpand(true)
	searchRow.Append(search)

	newChatBtn := gtk.NewButtonFromIconName("list-add-symbolic")
	newChatBtn.SetTooltipText("New chat")
	searchRow.Append(newChatBtn)

	newGroupBtn := gtk.NewButtonFromIconName("system-users-symbolic")
	newGroupBtn.SetTooltipText("New group")
	searchRow.Append(newGroupBtn)

	joinGroupBtn := gtk.NewButtonFromIconName("insert-link-symbolic")
	joinGroupBtn.SetTooltipText("Join group via link")
	searchRow.Append(joinGroupBtn)

	archiveToggle := gtk.NewToggleButton()
	archiveToggle.SetIconName("mail-archive-symbolic")
	archiveToggle.SetTooltipText("Show archived chats")
	searchRow.Append(archiveToggle)

	starredToggle := gtk.NewToggleButton()
	starredToggle.SetIconName("starred-symbolic")
	starredToggle.SetTooltipText("Starred messages")
	searchRow.Append(starredToggle)

	labelFilter := gtk.NewDropDownFromStrings([]string{"All"})
	labelFilter.SetTooltipText("Filter by label")
	searchRow.Append(labelFilter)

	privacyBtn := gtk.NewButtonFromIconName("preferences-system-privacy-symbolic")
	privacyBtn.SetTooltipText("Privacy settings")
	searchRow.Append(privacyBtn)

	root.Append(searchRow)

	list := gtk.NewListBox()
	list.AddCSSClass("navigation-sidebar")
	list.SetVExpand(true)
	root.Append(list)

	cl := &ChatList{Box: root, c: c, events: c.Events(), list: list, typingJIDs: make(map[string]bool), avatarCache: newAvatarCache(), labelFilter: labelFilter}

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
		if cl.query != "" && cl.showStarred {
			cl.showStarred = false
			starredToggle.SetActive(false)
		}
		cl.refresh()
	})

	newChatBtn.ConnectClicked(func() {
		cl.showNewChatDialog()
	})

	newGroupBtn.ConnectClicked(func() {
		cl.showNewGroupDialog()
	})

	joinGroupBtn.ConnectClicked(func() {
		cl.showJoinGroupDialog()
	})

	archiveToggle.ConnectToggled(func() {
		cl.showArchived = archiveToggle.Active()
		cl.refresh()
	})

	starredToggle.ConnectToggled(func() {
		cl.showStarred = starredToggle.Active()
		if cl.showStarred && cl.query != "" {
			cl.query = ""
			search.SetText("")
		}
		cl.refresh()
	})

	privacyBtn.ConnectClicked(func() {
		showPrivacyDialog(cl.window, cl.c)
	})

	labelFilter.Connect("notify::selected", func() {
		idx := int(labelFilter.Selected())
		if idx >= 0 && idx < len(cl.filterLabelIDs) {
			cl.selectedLabel = cl.filterLabelIDs[idx]
		} else {
			cl.selectedLabel = ""
		}
		cl.refresh()
	})

	cl.rebuildLabelFilter()
	cl.refresh()
	go cl.watchEvents()

	return cl
}

// OnChatSelected registers f to be called with the JID of the activated row.
func (cl *ChatList) OnChatSelected(f func(jid string)) {
	cl.onSelect = f
}

// refresh rebuilds the row widgets from StarredMessages (starred mode),
// Search (query set) or Chats (neither). Must run on the GTK main loop.
func (cl *ChatList) refresh() {
	switch {
	case cl.showStarred:
		cl.refreshStarred()
	case cl.query != "":
		cl.refreshSearch()
	default:
		cl.refreshChats()
	}
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
		if !showChatInList(chat, cl.showArchived) {
			continue
		}
		if !chatHasLabel(cl.c, chat.JID, cl.selectedLabel) {
			continue
		}
		vm := chatRowVM(chat, now)
		if cl.typingJIDs[chat.JID] {
			vm.Preview = "typing…"
			vm.Typing = true
		}
		vm.Blocked = cl.c.IsBlocked(chat.JID)
		row := buildChatRow(cl.c, cl.avatarCache, vm)
		attachChatContextMenu(row, cl.c, chat, cl.window)
		cl.list.Append(row)
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

// refreshStarred rebuilds the list from StarredMessages, reusing the
// search-hit row style (chat name + snippet + time); clicking a hit opens
// its chat via the same onSelect seam as everything else.
func (cl *ChatList) refreshStarred() {
	msgs, err := cl.c.StarredMessages(searchResultLimit)
	if err != nil {
		msgs = nil
	}

	cl.list.RemoveAll()

	if len(msgs) == 0 {
		cl.rowJIDs = nil
		empty := gtk.NewLabel("No starred messages")
		empty.AddCSSClass("chatot-search-empty")
		cl.list.Append(empty)
		return
	}

	chats, _ := cl.c.Chats(0)
	names := make(map[string]string, len(chats))
	for _, c := range chats {
		names[c.JID] = c.Name
	}

	now := time.Now()
	cl.rowJIDs = make([]string, 0, len(msgs))
	for _, m := range msgs {
		hit := client.SearchHit{ChatJID: m.ChatJID, MsgID: m.ID, ChatName: names[m.ChatJID], Snippet: starredSnippet(m), TS: m.TS}
		cl.list.Append(buildSearchHitRow(searchHitVM(hit, now)))
		cl.rowJIDs = append(cl.rowJIDs, m.ChatJID)
	}
}

// starredSnippet renders a starred message's preview line, falling back to a
// kind label for non-text bodies like the chat-list preview does.
func starredSnippet(m client.Message) string {
	switch {
	case m.Text != "":
		return m.Text
	case m.Attachment != nil:
		return mediaChip(*m.Attachment)
	case m.Location != nil:
		return "📍 Location"
	case m.Contact != nil:
		return "👤 Contact"
	case m.Poll != nil:
		return "📊 " + m.Poll.Name
	default:
		return ""
	}
}

// watchEvents listens for client events and schedules a list rebuild on the
// GTK main loop via glib.IdleAdd. Runs on its own goroutine for the
// lifetime of the process; the fake/whatsmeow Events() channel is never
// explicitly closed today, so this goroutine simply exits if it is.
// EventChatPresence updates typingJIDs (composing sets it, anything else
// clears it) instead of falling through to the generic full refresh, since
// it needs the event's JID+state before rebuilding rows. EventChatUpdate
// (pin/mute/archive/unread changes) needs no special handling: it falls
// through to the generic refresh below like most other event kinds.
func (cl *ChatList) watchEvents() {
	for ev := range cl.events {
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
		if ev.Kind == client.EventAvatar && ev.Avatar != nil {
			jid := ev.Avatar.JID
			glib.IdleAdd(func() {
				cl.avatarCache.invalidate(jid)
				cl.refresh()
			})
			continue
		}
		if ev.Kind == client.EventLabelUpdate {
			glib.IdleAdd(func() {
				cl.rebuildLabelFilter()
				cl.refresh()
			})
			continue
		}
		glib.IdleAdd(func() {
			cl.refresh()
		})
	}
}

// chatRowAvatarSize is the chat-list row avatar's fixed square size in px.
const chatRowAvatarSize = 36

// buildChatRow constructs the GTK widget tree for a single row from its
// pre-computed view-model. The avatar renders vm.Initial immediately and
// swaps in the real picture asynchronously via cache/c.Avatar (see
// buildAvatar).
func buildChatRow(c client.Client, cache *avatarCache, vm chatRowView) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.SetMarginTop(6)
	row.SetMarginBottom(6)
	row.SetMarginStart(8)
	row.SetMarginEnd(8)

	row.Append(buildAvatar(c, cache, vm.JID, vm.Initial, chatRowAvatarSize))

	textCol := gtk.NewBox(gtk.OrientationVertical, 2)
	textCol.SetHExpand(true)

	name := vm.Name
	if vm.Pinned {
		name = "📌 " + name
	}
	if vm.Muted {
		name = "🔇 " + name
	}
	if vm.Blocked {
		name = "🚫 " + name
	}
	nameLabel := gtk.NewLabel(name)
	nameLabel.SetXAlign(0)
	nameLabel.SetEllipsize(pango.EllipsizeEnd)
	nameLabel.SetMaxWidthChars(1)
	nameLabel.SetHExpand(true)
	nameLabel.AddCSSClass("chatot-chat-name")
	textCol.Append(nameLabel)

	// Single line, ellipsized. MaxWidthChars(1) keeps the label's natural
	// width tiny so a long message can't stretch the row wider than the
	// sidebar; HExpand lets it fill whatever width the sidebar does give.
	previewLabel := gtk.NewLabel(vm.Preview)
	previewLabel.SetXAlign(0)
	previewLabel.SetEllipsize(pango.EllipsizeEnd)
	previewLabel.SetMaxWidthChars(1)
	previewLabel.SetHExpand(true)
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

// attachChatContextMenu wires a secondary-click (right-click) gesture on row
// that pops a small menu of Pin/Mute/Archive/Mark-unread actions for chat.
// Each action calls the matching Client method in a goroutine; the resulting
// EventChatUpdate (or, for the fake, the equivalent) drives the list refresh
// via ChatList.watchEvents, so nothing here touches the row directly.
func attachChatContextMenu(row *gtk.Box, c client.Client, chat client.Chat, window *gtk.Window) {
	gesture := gtk.NewGestureClick()
	gesture.SetButton(gdk.BUTTON_SECONDARY)
	gesture.ConnectPressed(func(nPress int, x, y float64) {
		showChatContextMenu(row, c, chat, window, x, y)
	})
	row.AddController(gesture)
}

// showChatContextMenu builds and pops a Popover of action buttons anchored at
// (x, y) within row.
func showChatContextMenu(row *gtk.Box, c client.Client, chat client.Chat, window *gtk.Window, x, y float64) {
	labels := chatActionLabels(chat)
	pop := gtk.NewPopover()
	box := gtk.NewBox(gtk.OrientationVertical, 0)

	addAction := func(label string, do func(ctx context.Context) error) {
		btn := gtk.NewButtonWithLabel(label)
		btn.AddCSSClass("flat")
		btn.ConnectClicked(func() {
			pop.Popdown()
			go func() {
				if err := do(context.Background()); err != nil {
					log.Printf("chatot: chat action %q failed: %v", label, err)
				}
			}()
		})
		box.Append(btn)
	}

	addAction(labels.Pin, func(ctx context.Context) error { return c.PinChat(ctx, chat.JID, !chat.Pinned) })
	addAction(labels.Mute, func(ctx context.Context) error { return c.MuteChat(ctx, chat.JID, !chat.Muted) })
	addAction(labels.Archive, func(ctx context.Context) error { return c.ArchiveChat(ctx, chat.JID, !chat.Archived) })
	addAction(labels.Unread, func(ctx context.Context) error {
		return c.MarkChatUnread(ctx, chat.JID, chat.UnreadCount == 0)
	})
	labelsBtn := gtk.NewButtonWithLabel("Labels…")
	labelsBtn.AddCSSClass("flat")
	labelsBtn.ConnectClicked(func() {
		pop.Popdown()
		showLabelsDialog(window, c, chat)
	})
	box.Append(labelsBtn)

	// Blocking is per-contact; groups have no meaningful block target.
	if !chat.IsGroup {
		blocked := c.IsBlocked(chat.JID)
		addAction(blockActionLabel(blocked), func(ctx context.Context) error {
			return c.SetBlocked(ctx, chat.JID, !blocked)
		})
	}

	rect := gdk.NewRectangle(int(x), int(y), 1, 1)
	pop.SetChild(box)
	pop.SetParent(row)
	pop.SetPointingTo(&rect)
	pop.Popup()
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

// normalizePhone strips spaces/dashes/parens from input and checks the
// result looks like an E.164 number: a leading "+" followed by 7-15 digits.
func normalizePhone(input string) (string, bool) {
	var b strings.Builder
	for _, r := range input {
		switch {
		case r == ' ' || r == '-' || r == '(' || r == ')':
			continue
		case r == '+' && b.Len() == 0:
			b.WriteRune(r)
		case unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	s := b.String()
	if !strings.HasPrefix(s, "+") {
		return "", false
	}
	digits := s[1:]
	if len(digits) < 7 || len(digits) > 15 {
		return "", false
	}
	return s, true
}

// showNewChatDialog opens a small modal to compose a new chat: a phone-number
// entry validated by normalizePhone, then checked against WhatsApp via
// c.CheckOnWhatsApp before opening the conversation through the same
// onSelect seam a chat-list row activation uses.
func (cl *ChatList) showNewChatDialog() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("New chat")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Phone number, e.g. +15551234567")
	box.Append(entry)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	status.AddCSSClass("chatot-newchat-status")
	box.Append(status)

	startBtn := gtk.NewButtonWithLabel("Start")
	startBtn.AddCSSClass("suggested-action")
	box.Append(startBtn)

	startChat := func() {
		phone, ok := normalizePhone(entry.Text())
		if !ok {
			status.SetText("Enter a valid phone number, e.g. +15551234567")
			return
		}
		startBtn.SetSensitive(false)
		status.SetText("Checking…")

		go func() {
			jid, onWhatsApp, err := cl.c.CheckOnWhatsApp(context.Background(), phone)
			glib.IdleAdd(func() {
				startBtn.SetSensitive(true)
				switch {
				case err != nil:
					status.SetText("Couldn't check that number, try again")
				case !onWhatsApp:
					status.SetText("This number isn't on WhatsApp")
				default:
					dialog.Close()
					if cl.onSelect != nil {
						cl.onSelect(jid)
					}
				}
			})
		}()
	}
	startBtn.ConnectClicked(startChat)
	entry.ConnectActivate(startChat)

	dialog.SetChild(box)
	dialog.SetDefaultWidget(startBtn)
	dialog.Present()
}

// showNewGroupDialog opens a modal to create a group: a name entry plus a
// comma-separated participant list. On success it opens the new group's chat
// through the same onSelect seam a chat-row activation uses.
func (cl *ChatList) showNewGroupDialog() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("New group")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	nameEntry := gtk.NewEntry()
	nameEntry.SetPlaceholderText("Group name (max 25 chars)")
	box.Append(nameEntry)

	partsEntry := gtk.NewEntry()
	partsEntry.SetPlaceholderText("Participants: +1555…, +1666… (comma-separated)")
	box.Append(partsEntry)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	box.Append(status)

	createBtn := gtk.NewButtonWithLabel("Create")
	createBtn.AddCSSClass("suggested-action")
	box.Append(createBtn)

	createBtn.ConnectClicked(func() {
		name := strings.TrimSpace(nameEntry.Text())
		if name == "" {
			status.SetText("Enter a group name")
			return
		}
		parts := parseParticipantList(partsEntry.Text())
		createBtn.SetSensitive(false)
		status.SetText("Creating…")
		go func() {
			jid, err := cl.c.CreateGroup(context.Background(), name, parts)
			glib.IdleAdd(func() {
				createBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't create group, try again")
					return
				}
				dialog.Close()
				if cl.onSelect != nil {
					cl.onSelect(jid)
				}
			})
		}()
	})

	dialog.SetChild(box)
	dialog.SetDefaultWidget(createBtn)
	dialog.Present()
}

// showJoinGroupDialog opens a modal to join a group by pasting an invite link
// or bare code, then opens the joined group's chat via onSelect.
func (cl *ChatList) showJoinGroupDialog() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Join group")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("chat.whatsapp.com/… or invite code")
	box.Append(entry)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	box.Append(status)

	joinBtn := gtk.NewButtonWithLabel("Join")
	joinBtn.AddCSSClass("suggested-action")
	box.Append(joinBtn)

	join := func() {
		code := strings.TrimSpace(entry.Text())
		if code == "" {
			status.SetText("Paste an invite link or code")
			return
		}
		joinBtn.SetSensitive(false)
		status.SetText("Joining…")
		go func() {
			jid, err := cl.c.JoinGroupWithLink(context.Background(), code)
			glib.IdleAdd(func() {
				joinBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't join, check the link")
					return
				}
				dialog.Close()
				if cl.onSelect != nil {
					cl.onSelect(jid)
				}
			})
		}()
	}
	joinBtn.ConnectClicked(join)
	entry.ConnectActivate(join)

	dialog.SetChild(box)
	dialog.SetDefaultWidget(joinBtn)
	dialog.Present()
}

// showPrivacyDialog opens a read-only modal listing the account's privacy
// settings, fetched via c.PrivacySettings in a goroutine.
func showPrivacyDialog(parent *gtk.Window, c client.Client) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Privacy")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	status := gtk.NewLabel("Loading…")
	status.SetXAlign(0)
	box.Append(status)

	dialog.SetChild(box)
	dialog.Present()

	go func() {
		settings, err := c.PrivacySettings(context.Background())
		glib.IdleAdd(func() {
			box.Remove(status)
			if err != nil {
				box.Append(gtk.NewLabel("Couldn't load privacy settings"))
				return
			}
			for _, row := range privacySettingsRows(settings) {
				line := gtk.NewLabel(row.Name + ": " + row.Value)
				line.SetXAlign(0)
				box.Append(line)
			}
		})
	}()
}

// privacySettingRow is one name/value line in the privacy dialog.
type privacySettingRow struct {
	Name  string
	Value string
}

// privacySettingsRows sorts a privacy-settings map into a deterministic
// display order.
func privacySettingsRows(settings map[string]string) []privacySettingRow {
	rows := make([]privacySettingRow, 0, len(settings))
	for name, value := range settings {
		rows = append(rows, privacySettingRow{Name: name, Value: value})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}
