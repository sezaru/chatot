package ui

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
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

	c              client.Client
	events         <-chan client.Event
	list           *gtk.ListBox
	rowJIDs        []string // row index -> JID, rebuilt alongside the ListBox rows
	onSelect       func(jid string)
	composingJIDs  map[string]string // chat JID -> "typing" or "recording" for a peer currently composing
	query          string            // current search text; "" shows the normal chat list
	avatarCache    *avatarCache
	window         *gtk.Window         // parent for the new-chat dialog; set via SetWindow
	onNewCommunity func()              // "New community" from the ＋ menu; STUBBED until F48
	showArchived   bool                // toggled by the "Archived" button; see showChatInList
	showStarred    bool                // toggled by the "Starred" button; overrides search/archived
	showStatus     bool                // toggled by the "Status" button; overrides search/starred
	showChannels   bool                // toggled by the "Channels" button; overrides the others
	postStatusBar  *gtk.Box            // "Post status" bar, visible only in status mode
	followBar      *gtk.Box            // "Follow channel" bar, visible only in channels mode
	newsletters    []client.Newsletter // channels backing the sidebar in channels mode

	chipRow  *gtk.Box   // fixed + inline-label filter chips, under the search entry
	filter   chatFilter // the active chip; see chatFilter/chatMatchesFilter
	search   *gtk.SearchEntry
	starredT *gtk.ToggleButton
	statusT  *gtk.ToggleButton
	channelT *gtk.ToggleButton
}

// SetWindow supplies the parent window the new-chat dialog needs; call once
// after the top-level window exists.
func (cl *ChatList) SetWindow(w *gtk.Window) { cl.window = w }

// NewChatList builds a ChatList for c and populates it with the current
// chats. It also subscribes to c.Events() to keep the list live.
func NewChatList(c client.Client) *ChatList {
	root := gtk.NewBox(gtk.OrientationVertical, 0)

	// Row 1: account header. TODO multi-account: name/phone are hard-coded
	// placeholders until per-account data exists.
	accountRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	accountRow.AddCSSClass("chatot-account-row")

	accountAvatar := gtk.NewLabel("S")
	accountAvatar.AddCSSClass("chatot-avatar")
	accountAvatar.AddCSSClass("chatot-account-avatar")
	accountAvatar.SetSizeRequest(32, 32)
	accountRow.Append(accountAvatar)

	accountText := gtk.NewBox(gtk.OrientationVertical, 0)
	accountText.SetHExpand(true)
	accountText.SetVAlign(gtk.AlignCenter)
	accountName := gtk.NewLabel("Sezar (personal)")
	accountName.SetXAlign(0)
	accountName.AddCSSClass("chatot-chat-name")
	accountText.Append(accountName)
	accountPhone := gtk.NewLabel("+351 912 000 000")
	accountPhone.SetXAlign(0)
	accountPhone.AddCSSClass("chatot-account-phone")
	accountText.Append(accountPhone)
	accountRow.Append(accountText)

	plusBtn := gtk.NewMenuButton()
	plusBtn.SetIconName("list-add-symbolic")
	plusBtn.AddCSSClass("flat")
	plusBtn.SetTooltipText("New chat, group, community, or invite")
	accountRow.Append(plusBtn)

	appMenuBtn := gtk.NewMenuButton()
	appMenuBtn.SetIconName("open-menu-symbolic")
	appMenuBtn.AddCSSClass("flat")
	appMenuBtn.SetTooltipText("Menu")
	accountRow.Append(appMenuBtn)

	root.Append(accountRow)

	plusMenu := gtk.NewBox(gtk.OrientationVertical, 0)
	newChatItem := gtk.NewButtonWithLabel("New chat")
	newChatItem.AddCSSClass("flat")
	plusMenu.Append(newChatItem)
	newGroupItem := gtk.NewButtonWithLabel("New group")
	newGroupItem.AddCSSClass("flat")
	plusMenu.Append(newGroupItem)
	newCommunityItem := gtk.NewButtonWithLabel("New community")
	newCommunityItem.AddCSSClass("flat")
	plusMenu.Append(newCommunityItem)
	joinInviteItem := gtk.NewButtonWithLabel("Join with invite link")
	joinInviteItem.AddCSSClass("flat")
	plusMenu.Append(joinInviteItem)

	plusPopover := gtk.NewPopover()
	plusPopover.SetChild(plusMenu)
	plusBtn.SetPopover(plusPopover)

	// Row 2: full-width search. Drives the same query/filter logic as
	// before via ConnectSearchChanged below.
	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search")
	search.AddCSSClass("chatot-search-entry")
	search.SetHExpand(true)
	root.Append(search)

	// The old header's filter icons are gone from the bar; they live in the
	// ⋮ app menu (built below) but stay real ToggleButtons so all the
	// existing active-state cross-clearing keeps working unchanged.
	archiveToggle := gtk.NewToggleButton()
	starredToggle := gtk.NewToggleButton()
	statusToggle := gtk.NewToggleButton()
	channelsToggle := gtk.NewToggleButton()

	chipRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	chipRow.AddCSSClass("chatot-chip-row")
	root.Append(chipRow)

	postStatusBar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	postStatusBtn := gtk.NewButtonWithLabel("➕ Post status")
	postStatusBtn.AddCSSClass("flat")
	postStatusBtn.SetHExpand(true)
	postStatusBar.Append(postStatusBtn)
	postStatusBar.SetVisible(false)
	root.Append(postStatusBar)

	followBar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	followChannelBtn := gtk.NewButtonWithLabel("➕ Follow channel")
	followChannelBtn.AddCSSClass("flat")
	followChannelBtn.SetHExpand(true)
	followBar.Append(followChannelBtn)
	followBar.SetVisible(false)
	root.Append(followBar)

	list := gtk.NewListBox()
	list.AddCSSClass("navigation-sidebar")
	list.SetVExpand(true)
	root.Append(list)

	cl := &ChatList{
		Box: root, c: c, events: c.Events(), list: list,
		composingJIDs: make(map[string]string), avatarCache: newAvatarCache(),
		chipRow: chipRow, postStatusBar: postStatusBar, followBar: followBar,
		search: search, starredT: starredToggle, statusT: statusToggle, channelT: channelsToggle,
	}

	list.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx < 0 || idx >= len(cl.rowJIDs) {
			return
		}
		if cl.showChannels {
			cl.openChannel(cl.rowJIDs[idx])
			return
		}
		if cl.onSelect != nil {
			cl.onSelect(cl.rowJIDs[idx])
		}
	})

	search.ConnectSearchChanged(func() {
		cl.query = strings.TrimSpace(search.Text())
		if cl.query != "" {
			cl.filter = chatFilter{}
		}
		if cl.query != "" && cl.showStarred {
			cl.showStarred = false
			starredToggle.SetActive(false)
		}
		if cl.query != "" && cl.showStatus {
			statusToggle.SetActive(false)
		}
		if cl.query != "" && cl.showChannels {
			channelsToggle.SetActive(false)
		}
		cl.refresh()
	})

	newChatItem.ConnectClicked(func() {
		plusPopover.Popdown()
		cl.showNewChatDialog()
	})

	newGroupItem.ConnectClicked(func() {
		plusPopover.Popdown()
		cl.showNewGroupDialog()
	})

	newCommunityItem.ConnectClicked(func() {
		plusPopover.Popdown()
		if cl.onNewCommunity != nil {
			cl.onNewCommunity()
		}
	})

	joinInviteItem.ConnectClicked(func() {
		plusPopover.Popdown()
		cl.showJoinGroupDialog()
	})

	archiveToggle.ConnectToggled(func() {
		cl.showArchived = archiveToggle.Active()
		cl.refresh()
	})

	starredToggle.ConnectToggled(func() {
		cl.showStarred = starredToggle.Active()
		if cl.showStarred {
			cl.filter = chatFilter{}
		}
		if cl.showStarred && cl.query != "" {
			cl.query = ""
			search.SetText("")
		}
		if cl.showStarred && cl.showStatus {
			statusToggle.SetActive(false)
		}
		if cl.showStarred && cl.showChannels {
			channelsToggle.SetActive(false)
		}
		cl.refresh()
	})

	statusToggle.ConnectToggled(func() {
		cl.showStatus = statusToggle.Active()
		cl.postStatusBar.SetVisible(cl.showStatus)
		if cl.showStatus {
			cl.filter = chatFilter{}
			if cl.query != "" {
				cl.query = ""
				search.SetText("")
			}
			if cl.showStarred {
				cl.showStarred = false
				starredToggle.SetActive(false)
			}
			if cl.showChannels {
				channelsToggle.SetActive(false)
			}
		}
		cl.refresh()
	})

	channelsToggle.ConnectToggled(func() {
		cl.showChannels = channelsToggle.Active()
		cl.followBar.SetVisible(cl.showChannels)
		if cl.showChannels {
			cl.filter = chatFilter{}
			if cl.query != "" {
				cl.query = ""
				search.SetText("")
			}
			if cl.showStarred {
				cl.showStarred = false
				starredToggle.SetActive(false)
			}
			if cl.showStatus {
				statusToggle.SetActive(false)
			}
		}
		cl.refresh()
	})

	followChannelBtn.ConnectClicked(func() {
		cl.showFollowChannelDialog()
	})

	postStatusBtn.ConnectClicked(func() {
		cl.showPostStatusDialog()
	})

	appPopover := gtk.NewPopover()
	appMenu := gtk.NewBox(gtk.OrientationVertical, 0)
	addAppMenuItem := func(label string, onClick func()) {
		item := gtk.NewButtonWithLabel(label)
		item.AddCSSClass("flat")
		item.SetHAlign(gtk.AlignFill)
		item.Child().(*gtk.Label).SetXAlign(0)
		item.ConnectClicked(func() {
			appPopover.Popdown()
			onClick()
		})
		appMenu.Append(item)
	}
	addAppMenuItem("Archived chats", func() { archiveToggle.SetActive(!archiveToggle.Active()) })
	addAppMenuItem("Starred messages", func() { starredToggle.SetActive(!starredToggle.Active()) })
	addAppMenuItem("Status updates", func() { statusToggle.SetActive(!statusToggle.Active()) })
	addAppMenuItem("Channels", func() { channelsToggle.SetActive(!channelsToggle.Active()) })
	appMenu.Append(gtk.NewSeparator(gtk.OrientationHorizontal))
	addAppMenuItem("Set status", func() { cl.showPostStatusDialog() })
	addAppMenuItem("Privacy settings", func() { showPrivacyDialog(cl.window, cl.c) })
	addAppMenuItem("Blocked contacts", func() { showBlockedDialog(cl.window, cl.c) })
	addAppMenuItem("Keyboard shortcuts", func() { showShortcutsDialog(cl.window) })
	addAppMenuItem("Preferences", func() { appMenuBtn.ActivateAction("app.preferences", nil) })
	appMenu.Append(gtk.NewSeparator(gtk.OrientationHorizontal))
	addAppMenuItem("About chatot", func() { showAboutDialog(cl.window) })
	appPopover.SetChild(appMenu)
	appMenuBtn.SetPopover(appPopover)

	cl.refresh()
	go cl.watchEvents()

	return cl
}

// showAboutDialog presents a minimal AboutDialog for the ⋮ menu's "About".
func showAboutDialog(parent *gtk.Window) {
	about := adw.NewAboutDialog()
	about.SetApplicationName("chatot")
	about.SetDeveloperName("chatot")
	about.SetVersion("0.1.0")
	if parent != nil {
		about.Present(parent)
		return
	}
	about.Present(nil)
}

// OnChatSelected registers f to be called with the JID of the activated row.
func (cl *ChatList) OnChatSelected(f func(jid string)) {
	cl.onSelect = f
}

// OnNewCommunityRequested registers f to be called when the user picks "New
// community" from the ＋ menu; STUBBED until F48 implements communities.
func (cl *ChatList) OnNewCommunityRequested(f func()) { cl.onNewCommunity = f }

// OpenGlobalSearch switches the sidebar into search mode with query
// pre-filled, clearing any active starred/status/channels filter — the
// "Search all chats" link in the in-chat search bar funnels here.
func (cl *ChatList) OpenGlobalSearch(query string) {
	cl.search.SetText(query)
	cl.search.GrabFocus()
}

// refresh rebuilds the row widgets from Statuses (status mode),
// StarredMessages (starred mode), Search (query set) or Chats (none of the
// above). Must run on the GTK main loop.
func (cl *ChatList) refresh() {
	cl.updateChipRow()
	switch {
	case cl.showChannels:
		cl.refreshChannels()
	case cl.showStatus:
		cl.refreshStatus()
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
		if !chatVisible(cl.c, chat, cl.filter) {
			continue
		}
		vm := chatRowVM(chat, now)
		if kind, ok := cl.composingJIDs[chat.JID]; ok {
			vm.Preview = composingPreviewText(kind)
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

// refreshStatus rebuilds the list from Statuses (received status updates),
// reusing the search-hit row style: one row per status showing the poster's
// name, a snippet and the time. Clicking a row opens the poster's chat via
// the same onSelect seam as everything else.
func (cl *ChatList) refreshStatus() {
	msgs, err := cl.c.Statuses(searchResultLimit)
	if err != nil {
		msgs = nil
	}

	cl.list.RemoveAll()

	if len(msgs) == 0 {
		cl.rowJIDs = nil
		empty := gtk.NewLabel("No status updates")
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
		vm := statusRowVM(m, posterName(m.FromJID, names), now)
		cl.list.Append(buildSearchHitRow(searchHitView{
			ChatName: vm.PosterName, Snippet: vm.Snippet, TimeText: vm.TimeText, Initial: vm.Initial,
		}))
		cl.rowJIDs = append(cl.rowJIDs, m.FromJID)
	}
}

// posterName resolves a status poster's JID to a display name: a known chat
// name if we have one, else a "+number" derived from the JID, else the raw
// JID.
func posterName(jid string, names map[string]string) string {
	if n := names[jid]; n != "" {
		return n
	}
	at := strings.IndexByte(jid, '@')
	if at > 0 {
		user := jid[:at]
		allDigits := true
		for _, r := range user {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return "+" + user
		}
	}
	return jid
}

// statusRowView holds the pure display fields for one status row.
type statusRowView struct {
	PosterName string
	Snippet    string
	TimeText   string
	Initial    string
}

// statusRowVM derives the display view-model for a single status update.
// posterName is resolved by the caller; now is injected for deterministic
// time formatting in tests.
func statusRowVM(m client.Message, posterName string, now time.Time) statusRowView {
	name := posterName
	if name == "" {
		name = m.FromJID
	}
	initial := "?"
	for _, r := range name {
		initial = strings.ToUpper(string(r))
		break
	}
	return statusRowView{
		PosterName: name,
		Snippet:    statusSnippet(m),
		TimeText:   formatChatTime(m.TS, now),
		Initial:    initial,
	}
}

// statusSnippet renders a status update's preview line: its text, or a media
// placeholder ("📷 Photo" / "🎥 Video" / …) for a media status.
func statusSnippet(m client.Message) string {
	if m.Text != "" {
		return m.Text
	}
	if m.Attachment != nil {
		switch m.Attachment.Kind {
		case "image":
			return "📷 Photo"
		case "video":
			return "🎥 Video"
		case "audio":
			return "🎤 Audio"
		case "sticker":
			return "🎨 Sticker"
		default:
			return "📎 " + m.Attachment.Kind
		}
	}
	return ""
}

// showPostStatusDialog opens a small modal to post a text status: a text
// entry, then PostStatus in a goroutine.
func (cl *ChatList) showPostStatusDialog() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Post status")
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
	entry.SetPlaceholderText("What's on your mind?")
	box.Append(entry)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	box.Append(status)

	postBtn := gtk.NewButtonWithLabel("Post")
	postBtn.AddCSSClass("suggested-action")
	box.Append(postBtn)

	post := func() {
		text := strings.TrimSpace(entry.Text())
		if text == "" {
			status.SetText("Enter some text to post")
			return
		}
		postBtn.SetSensitive(false)
		status.SetText("Posting…")
		go func() {
			err := cl.c.PostStatus(context.Background(), text)
			glib.IdleAdd(func() {
				postBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't post status, try again")
					return
				}
				dialog.Close()
				cl.refresh()
			})
		}()
	}
	postBtn.ConnectClicked(post)
	entry.ConnectActivate(post)

	dialog.SetChild(box)
	dialog.SetDefaultWidget(postBtn)
	dialog.Present()
}

// composingPreviewText renders the chat-list preview override for a peer
// currently composing: kind is "recording" for a voice-note recording, else
// plain typing.
func composingPreviewText(kind string) string {
	if kind == "recording" {
		return "recording audio…"
	}
	return "typing…"
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
// EventChatPresence updates composingJIDs (composing sets "typing" or
// "recording", anything else clears it) instead of falling through to the
// generic full refresh, since it needs the event's JID+state before
// rebuilding rows. EventChatUpdate (pin/mute/archive/unread changes) needs no
// special handling: it falls through to the generic refresh below like most
// other event kinds.
func (cl *ChatList) watchEvents() {
	for ev := range cl.events {
		if ev.Kind == client.EventChatPresence && ev.ChatPresence != nil {
			jid := ev.ChatPresence.ChatJID
			typing, recording := chatPresenceTypingRecording(ev.ChatPresence.State, ev.ChatPresence.Media)
			glib.IdleAdd(func() {
				switch {
				case recording:
					cl.composingJIDs[jid] = "recording"
				case typing:
					cl.composingJIDs[jid] = "typing"
				default:
					delete(cl.composingJIDs, jid)
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
	labelsBtn := gtk.NewButtonWithLabel("Labels ▸")
	labelsBtn.AddCSSClass("flat")
	labelsBtn.ConnectClicked(func() {
		showLabelsSubmenu(labelsBtn, c, chat)
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
	pop.ConnectClosed(func() { pop.Unparent() })
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

// newChatContacts filters chats to 1:1 (non-group) contacts and sorts them
// case-insensitively by name, for the new-chat view's CONTACTS list.
func newChatContacts(chats []client.Chat) []client.Chat {
	out := make([]client.Chat, 0, len(chats))
	for _, c := range chats {
		if !c.IsGroup {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// showNewChatDialog opens the "New chat" view: a phone-number entry at top
// that live-checks WhatsApp membership via c.CheckOnWhatsApp as the user
// types (validated by normalizePhone) and surfaces an "On WhatsApp · start a
// chat" row with a Message button, plus a CONTACTS list of known 1:1 chats
// below. Both paths open the conversation through the same onSelect seam a
// chat-list row activation uses.
func (cl *ChatList) showNewChatDialog() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("New chat")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(360, 480)

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

	resultRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	resultRow.SetVisible(false)
	resultLabel := gtk.NewLabel("On WhatsApp · start a chat")
	resultLabel.SetXAlign(0)
	resultLabel.SetHExpand(true)
	resultRow.Append(resultLabel)
	messageBtn := gtk.NewButtonWithLabel("Message")
	messageBtn.AddCSSClass("suggested-action")
	resultRow.Append(messageBtn)
	box.Append(resultRow)

	var resultJID string
	openResult := func() {
		if resultJID == "" {
			return
		}
		dialog.Close()
		if cl.onSelect != nil {
			cl.onSelect(resultJID)
		}
	}
	messageBtn.ConnectClicked(openResult)
	entry.ConnectActivate(openResult)

	// generation guards against a stale CheckOnWhatsApp response landing
	// after the user has kept typing past it.
	generation := 0
	entry.ConnectChanged(func() {
		resultRow.SetVisible(false)
		resultJID = ""
		generation++ // any edit invalidates an in-flight check, even editing to an invalid number
		phone, ok := normalizePhone(entry.Text())
		if !ok {
			status.SetText("")
			return
		}
		status.SetText("Checking…")
		gen := generation
		go func() {
			jid, onWhatsApp, err := cl.c.CheckOnWhatsApp(context.Background(), phone)
			glib.IdleAdd(func() {
				if gen != generation {
					return
				}
				switch {
				case err != nil:
					status.SetText("Couldn't check that number, try again")
				case !onWhatsApp:
					status.SetText("This number isn't on WhatsApp")
				default:
					status.SetText("")
					resultJID = jid
					resultRow.SetVisible(true)
				}
			})
		}()
	})

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	contactsLabel := gtk.NewLabel("CONTACTS")
	contactsLabel.SetXAlign(0)
	contactsLabel.AddCSSClass("chatot-newchat-contacts-label")
	box.Append(contactsLabel)

	contacts := newChatContacts(chatsOrEmpty(cl.c))

	contactsList := gtk.NewListBox()
	contactsList.AddCSSClass("navigation-sidebar")
	now := time.Now()
	for _, ct := range contacts {
		contactsList.Append(buildChatRow(cl.c, cl.avatarCache, chatRowVM(ct, now)))
	}
	contactsList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx < 0 || idx >= len(contacts) {
			return
		}
		dialog.Close()
		if cl.onSelect != nil {
			cl.onSelect(contacts[idx].JID)
		}
	})

	scroller := gtk.NewScrolledWindow()
	scroller.SetChild(contactsList)
	scroller.SetVExpand(true)
	box.Append(scroller)

	dialog.SetChild(box)
	dialog.Present()
	entry.GrabFocus()
}

// chatsOrEmpty returns c.Chats(0), or nil if it errors, so callers that only
// need a best-effort list don't have to handle the error themselves.
func chatsOrEmpty(c client.Client) []client.Chat {
	chats, err := c.Chats(0)
	if err != nil {
		return nil
	}
	return chats
}

// showNewGroupDialog opens the two-step "New group" flow: page 1 picks
// participants from the 1:1 contact list (with search + a live selected
// count + removable chips), page 2 sets the group's name, disappearing
// timer and "only admins can post" mode. On success it opens the new
// group's chat through the same onSelect seam a chat-row activation uses.
func (cl *ChatList) showNewGroupDialog() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("New group")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(380, 560)

	sel := newParticipantSelection()
	contacts := newChatContacts(chatsOrEmpty(cl.c))
	checkButtons := make(map[string]*gtk.CheckButton, len(contacts))

	stack := gtk.NewStack()
	stack.SetVExpand(true)

	// --- page 1: add participants ---
	page1 := gtk.NewBox(gtk.OrientationVertical, 8)
	page1.SetMarginTop(12)
	page1.SetMarginBottom(12)
	page1.SetMarginStart(12)
	page1.SetMarginEnd(12)

	countLabel := gtk.NewLabel("0 of 1024 selected")
	countLabel.SetXAlign(0)
	countLabel.AddCSSClass("chatot-conv-title")
	page1.Append(countLabel)

	chipsBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	page1.Append(chipsBox)

	nextBtn := gtk.NewButtonWithLabel("Next")
	nextBtn.AddCSSClass("suggested-action")
	nextBtn.SetSensitive(false)

	var refresh func()
	refresh = func() {
		countLabel.SetText(fmt.Sprintf("%d of 1024 selected", sel.Count()))
		nextBtn.SetSensitive(sel.Count() > 0)
		for chipsBox.FirstChild() != nil {
			chipsBox.Remove(chipsBox.FirstChild())
		}
		for _, chip := range sel.Chips() {
			jid := chip.JID
			btn := gtk.NewButtonWithLabel(chip.Name + " ×")
			btn.ConnectClicked(func() {
				if cb, ok := checkButtons[jid]; ok {
					cb.SetActive(false)
				}
			})
			chipsBox.Append(btn)
		}
	}

	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search contacts")
	page1.Append(search)

	contactsList := gtk.NewListBox()
	contactsList.AddCSSClass("navigation-sidebar")
	for _, ct := range contacts {
		ct := ct
		row := gtk.NewBox(gtk.OrientationHorizontal, 8)
		row.SetMarginTop(6)
		row.SetMarginBottom(6)
		row.SetMarginStart(8)
		row.SetMarginEnd(8)
		cb := gtk.NewCheckButton()
		checkButtons[ct.JID] = cb
		cb.ConnectToggled(func() {
			if cb.Active() {
				sel.Add(ct.JID, ct.Name)
			} else {
				sel.Remove(ct.JID)
			}
			refresh()
		})
		row.Append(cb)
		row.Append(buildAvatar(cl.c, cl.avatarCache, ct.JID, contactInitial(ct.Name), 32))
		nameLabel := gtk.NewLabel(ct.Name)
		nameLabel.SetXAlign(0)
		nameLabel.SetHExpand(true)
		row.Append(nameLabel)
		contactsList.Append(row)
	}
	contactsList.SetFilterFunc(func(row *gtk.ListBoxRow) bool {
		idx := row.Index()
		if idx < 0 || idx >= len(contacts) {
			return true
		}
		q := strings.ToLower(strings.TrimSpace(search.Text()))
		if q == "" {
			return true
		}
		return strings.Contains(strings.ToLower(contacts[idx].Name), q)
	})
	search.ConnectSearchChanged(func() { contactsList.InvalidateFilter() })

	scroller := gtk.NewScrolledWindow()
	scroller.SetChild(contactsList)
	scroller.SetVExpand(true)
	page1.Append(scroller)
	page1.Append(nextBtn)

	stack.AddNamed(page1, "participants")

	// --- page 2: group details ---
	page2 := gtk.NewBox(gtk.OrientationVertical, 8)
	page2.SetMarginTop(12)
	page2.SetMarginBottom(12)
	page2.SetMarginStart(12)
	page2.SetMarginEnd(12)

	nameEntry := gtk.NewEntry()
	nameEntry.SetPlaceholderText("Group name (max 25 chars)")
	page2.Append(nameEntry)

	disappearingDropdown := gtk.NewDropDownFromStrings(disappearingOptions)
	page2.Append(dropdownRow("Disappearing messages", disappearingDropdown))

	announceRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	announceLabel := gtk.NewLabel("Only admins can post")
	announceLabel.SetXAlign(0)
	announceLabel.SetHExpand(true)
	announceRow.Append(announceLabel)
	announceSwitch := gtk.NewSwitch()
	announceRow.Append(announceSwitch)
	page2.Append(announceRow)

	hint := gtk.NewLabel("You can change this later")
	hint.SetXAlign(0)
	hint.AddCSSClass("chatot-conv-subtitle")
	page2.Append(hint)

	footer := gtk.NewLabel("")
	footer.SetXAlign(0)
	page2.Append(footer)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	page2.Append(status)

	btnRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	backBtn := gtk.NewButtonWithLabel("Back")
	btnRow.Append(backBtn)
	createBtn := gtk.NewButtonWithLabel("Create")
	createBtn.AddCSSClass("suggested-action")
	createBtn.SetSensitive(false)
	btnRow.Append(createBtn)
	page2.Append(btnRow)

	nameEntry.ConnectChanged(func() {
		createBtn.SetSensitive(strings.TrimSpace(nameEntry.Text()) != "")
	})

	stack.AddNamed(page2, "details")

	nextBtn.ConnectClicked(func() {
		footer.SetText(fmt.Sprintf("%d participants · you will be admin", sel.Count()))
		stack.SetVisibleChildName("details")
	})
	backBtn.ConnectClicked(func() { stack.SetVisibleChildName("participants") })

	createBtn.ConnectClicked(func() {
		name := strings.TrimSpace(nameEntry.Text())
		if name == "" {
			return
		}
		parts := sel.JIDs()
		announce := announceSwitch.Active()
		seconds := disappearingSecondsForIndex(int(disappearingDropdown.Selected()))
		createBtn.SetSensitive(false)
		backBtn.SetSensitive(false)
		status.SetText("Creating…")
		go func() {
			jid, err := cl.c.CreateGroup(context.Background(), name, parts)
			if err == nil {
				if announce {
					if aerr := cl.c.SetGroupAnnounce(context.Background(), jid, true); aerr != nil {
						log.Printf("chatot/ui: set group announce: %v", aerr)
					}
				}
				if seconds > 0 {
					if derr := cl.c.SetGroupDisappearingTimer(context.Background(), jid, seconds); derr != nil {
						log.Printf("chatot/ui: set group disappearing timer: %v", derr)
					}
				}
			}
			glib.IdleAdd(func() {
				createBtn.SetSensitive(true)
				backBtn.SetSensitive(true)
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

	dialog.SetChild(stack)
	stack.SetVisibleChildName("participants")
	dialog.Present()
}

// contactInitial returns the upper-cased first rune of name, or "?" if name
// is empty, for a contact-row avatar placeholder.
func contactInitial(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
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
