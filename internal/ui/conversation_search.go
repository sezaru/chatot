package ui

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// setupHeaderMenu wires the header "⋮" menu to a mockup-style menu popover,
// disabled until a chat is open (see refreshHeader). The rows are rebuilt on
// every popup because half of them read as the action they'd perform
// ("Pin chat" vs "Unpin chat"), which depends on the chat's current state.
func (cv *ConversationView) setupHeaderMenu(menuBtn *gtk.Button) {
	popover := newMenuPopover(nil)
	popover.SetParent(menuBtn)
	cv.headerMenuPop = popover
	// Rebuilt on show rather than on the button's click so every way of
	// popping it — the button, a keyboard activation, the screenshot hooks —
	// gets rows that match the chat's current state.
	popover.ConnectShow(func() { setMenuPopoverItems(popover, cv.headerMenuItems()) })
	menuBtn.ConnectClicked(func() { popover.Popup() })
}

// headerMenuItems builds the header ⋮ menu's rows for the open chat.
//
// A row with no handler renders insensitive rather than silently missing.
// One is: the disappearing-message timer, which WhatsApp only exposes per
// group.
func (cv *ConversationView) headerMenuItems() []menuItem {
	return cv.MenuItemsForChat(cv.jid, nil)
}

// MenuItemsForChat is the ⋮ menu for any chat, not just the open one: the
// chat list's right-click menu shows the same rows. ensureOpen, when set,
// runs before a row that acts on the open conversation (Search in chat,
// Media, Export, Clear) so those land on the right thread.
func (cv *ConversationView) MenuItemsForChat(jid string, ensureOpen func()) []menuItem {
	chat := chatByJID(cv.c, jid)
	opening := func(f func()) func() {
		if ensureOpen == nil {
			return f
		}
		return func() {
			ensureOpen()
			f()
		}
	}

	info := func() { cv.showGroupInfo(chat) }
	if !chat.IsGroup {
		info = func() { cv.showContactInfo(chat) }
	}

	// The timer is set through the same call for groups and 1:1 chats; only
	// the current value is group-only (GroupInfo), so a contact reads "Off".
	disappearing := func() {
		showDisappearingDialog(cv.window, cv.c, jid, cv.disappearingTimer(jid), func(s int64) { cv.rememberTimer(jid, s) })
	}

	// Blocking is per-contact; a group has no meaningful block target, so the
	// row stays inert there.
	blocked := cv.c.IsBlocked(jid)
	var block func()
	if !chat.IsGroup {
		block = func() {
			if blocked {
				cv.chatAction("unblock", func() error { return cv.c.SetBlocked(context.Background(), jid, false) })
				return
			}
			showBlockConfirmDialog(cv.window, cv.c, jid, chat.Name)
		}
	}

	return chatMenuItems(chat, blocked, chatMenuActions{
		Info:   info,
		Search: opening(cv.openSearchBar),
		Media:  opening(func() { callWithJID(cv.onShowMedia, jid) }),
		Mute: func() {
			// "Mute notifications…" asks for how long; unmuting is immediate.
			if chat.Muted {
				cv.chatAction("unmute", func() error { return cv.c.MuteChat(context.Background(), jid, false) })
				return
			}
			showMuteDialog(cv.window, cv.c, jid, chat.Name)
		},
		Pin: func() {
			cv.chatAction("pin", func() error { return cv.c.PinChat(context.Background(), jid, !chat.Pinned) })
		},
		Archive: func() {
			cv.chatAction("archive", func() error { return cv.c.ArchiveChat(context.Background(), jid, !chat.Archived) })
		},
		Disappearing: disappearing,
		Export:       opening(func() { callWithJID(cv.onExportChat, jid) }),
		Clear:        opening(func() { callWithJID(cv.onClearChat, jid) }),
		Block:        block,
	})
}

// MessageMenuItems is the message ⋯ menu for msg, as the bubbles show it;
// the attachment viewer's ⋯ reuses it.
func (cv *ConversationView) MessageMenuItems(msg client.Message) []menuItem {
	canEdit := false
	canDelete := !msg.Deleted && msg.FromMe
	return cv.hooks().menuItemsFor(msg, canEdit, canDelete)
}

// showContactInfo opens the mockup's contact card for a 1:1 chat, wiring its
// rows to the same actions the header ⋮ menu offers.
func (cv *ConversationView) showContactInfo(chat client.Chat) {
	jid := chat.JID
	timer := cv.disappearingTimer(jid)
	showContactInfoDialog(cv.window, cv.c, cv.avatarCache, jid, chat.Name, cv.c.IsBlocked(jid), contactInfoActions{
		Muted: chat.Muted,
		// The card's Mute row is a plain toggle in the mockup (the ⋮ menu is
		// where a duration is asked for).
		Mute: func() {
			cv.chatAction("mute", func() error { return cv.c.MuteChat(context.Background(), jid, !chat.Muted) })
		},
		DisappearingValue: disappearingValueLabel(timer),
		Disappearing: func() {
			showDisappearingDialog(cv.window, cv.c, jid, timer, func(s int64) { cv.rememberTimer(jid, s) })
		},
		MediaCount: cv.mediaCount(jid),
		Media:      func() { callWithJID(cv.onShowMedia, jid) },
		Block: func() {
			if cv.c.IsBlocked(jid) {
				cv.chatAction("unblock", func() error { return cv.c.SetBlocked(context.Background(), jid, false) })
				return
			}
			showBlockConfirmDialog(cv.window, cv.c, jid, chat.Name)
		},
	})
}

// showGroupInfo opens the mockup's info card for a group, its rows wired
// to the same actions the header ⋮ menu offers and its invite dialog to
// the view's toast and forward sinks.
func (cv *ConversationView) showGroupInfo(chat client.Chat) {
	jid := chat.JID
	timer := cv.disappearingTimer(jid)
	showGroupInfoDialog(cv.window, cv.c, cv.avatarCache, jid, groupInfoActions{
		Muted: chat.Muted,
		Mute: func() {
			cv.chatAction("mute", func() error { return cv.c.MuteChat(context.Background(), jid, !chat.Muted) })
		},
		DisappearingValue: disappearingValueLabel(timer),
		Disappearing: func() {
			showDisappearingDialog(cv.window, cv.c, jid, timer, func(s int64) { cv.rememberTimer(jid, s) })
		},
		MediaCount: cv.mediaCount(jid),
		Media:      func() { callWithJID(cv.onShowMedia, jid) },
		Toast:      func(text string) { showToast(cv.toastOverlay, text) },
		Forward:    cv.onForward,
	})
}

// showGroupInvite opens the group's invite-link card directly (the ⋮ menu
// reaches it through Group info).
func (cv *ConversationView) showGroupInvite(chat client.Chat) {
	showInviteLinkDialog(cv.window, cv.c, chat.JID, "Invite to "+chat.Name, groupInviteBody,
		func(text string) { showToast(cv.toastOverlay, text) }, cv.onForward)
}

// disappearingTimer is the chat's current disappearing-message timer in
// seconds, 0 when it's off or unknown. A group's comes from GroupInfo; a
// 1:1 chat's is only what this session set through the chooser (the client
// tracks no per-contact ephemeral setting), so it reads "Off" after a
// restart until it is set again.
func (cv *ConversationView) disappearingTimer(jid string) int64 {
	if !strings.HasSuffix(jid, "@g.us") {
		return cv.timers[jid]
	}
	info, err := cv.c.GroupInfo(context.Background(), jid)
	if err != nil || info == nil {
		return cv.timers[jid]
	}
	return int64(info.DisappearingTimer)
}

// rememberTimer records a timer the chooser applied, for disappearingTimer.
func (cv *ConversationView) rememberTimer(jid string, seconds int64) {
	cv.timers[jid] = seconds
}

// callWithJID invokes f with jid when both are set. The header menu's media,
// export and clear rows are all optional hooks the window owner may not have
// wired.
func callWithJID(f func(jid string), jid string) {
	if f != nil && jid != "" {
		f(jid)
	}
}

// chatAction runs a chat-organization write off the UI thread, logging a
// failure under name. The resulting EventChatUpdate refreshes the list.
// The target jid is bound into do by the caller (the chat list's row menu
// acts on rows that are not the open chat), so no open chat is required.
func (cv *ConversationView) chatAction(name string, do func() error) {
	go func() {
		if err := do(); err != nil {
			log.Printf("chatot: chat action %q failed: %v", name, err)
		}
	}()
}

// chatByJID looks up a chat's current organization state (pinned, muted,
// archived, group) from the client's chat list. It returns a zero Chat when
// jid isn't found, which reads as an unpinned, unmuted, unarchived 1:1.
//
// The client exposes no single-chat lookup, so this scans the list. It runs
// once per header-menu popup rather than being cached, because the menu's
// wording has to reflect a pin/mute/archive the user may have just performed
// from anywhere else in the app.
func chatByJID(c client.Client, jid string) client.Chat {
	if jid == "" {
		return client.Chat{}
	}
	chats, err := c.Chats(0)
	if err != nil {
		log.Printf("chatot: look up chat %q failed: %v", jid, err)
		return client.Chat{JID: jid}
	}
	for _, chat := range chats {
		if chat.JID == jid {
			return chat
		}
	}
	return client.Chat{JID: jid}
}

// openSearchBar reveals the in-chat search bar and focuses its entry.
func (cv *ConversationView) openSearchBar() {
	cv.headerStack.SetVisibleChildName("search")
	cv.searchEntry.GrabFocus()
}

// closeSearchBar hides the in-chat search bar and clears any active search
// state (and its highlights).
func (cv *ConversationView) closeSearchBar() {
	cv.headerStack.SetVisibleChildName("identity")
	cv.searchEntry.SetText("")
}

// runSearch re-queries the store for query scoped to the open chat, updates
// the hit counter, jumps to the most recent match, and (un)highlights
// matching bubbles. Must run on the GTK main loop.
func (cv *ConversationView) runSearch(query string) {
	query = strings.TrimSpace(query)
	cv.searchQuery = query

	if query == "" || cv.jid == "" {
		cv.searchHits = nil
		cv.searchIdx = -1
		cv.searchHitLabel.SetLabel("")
		cv.applyHighlights(nil)
		return
	}

	hits, err := cv.c.SearchInChat(cv.jid, query, 0)
	if err != nil {
		hits = nil
	}
	cv.searchHits = hits
	cv.applyHighlights(hits)

	if len(hits) == 0 {
		cv.searchIdx = -1
		cv.searchHitLabel.SetLabel(searchHitCountText(0, 0))
		return
	}

	cv.searchIdx = len(hits) - 1 // most recent match first
	cv.searchHitLabel.SetLabel(searchHitCountText(cv.searchIdx, len(hits)))
	cv.jumpToMessage(hits[cv.searchIdx].MsgID)
}

// stepHit moves to the next (forward) or previous hit, wrapping around, and
// scrolls it into view. Must run on the GTK main loop.
func (cv *ConversationView) stepHit(forward bool) {
	if len(cv.searchHits) == 0 {
		return
	}
	cv.searchIdx = searchHitStep(cv.searchIdx, len(cv.searchHits), forward)
	cv.searchHitLabel.SetLabel(searchHitCountText(cv.searchIdx, len(cv.searchHits)))
	cv.jumpToMessage(cv.searchHits[cv.searchIdx].MsgID)
}

// positionOf returns msgID's index in the currently-loaded cv.msgs, -1 if
// it isn't loaded.
func (cv *ConversationView) positionOf(msgID string) int {
	for i, m := range cv.msgs {
		if m.ID == msgID {
			return i
		}
	}
	return -1
}

// jumpToMessage scrolls the thread to msgID, synchronously loading older
// pages (via MessagesBefore) until it's found or local history runs dry.
// LIMITATION: a hit older than everything MessagesBefore can return locally
// (i.e. it would need a RequestMoreHistory round-trip to the phone) can't be
// reached — search coverage matches whatever the store already has synced.
// Must run on the GTK main loop.
func (cv *ConversationView) jumpToMessage(msgID string) {
	pos := cv.positionOf(msgID)
	for pos < 0 && cv.hasMore && !cv.loadingOlder {
		older, err := cv.c.MessagesBefore(cv.jid, cv.oldestID, conversationPageSize)
		if err != nil || len(older) == 0 {
			cv.hasMore = len(older) == conversationPageSize
			break
		}
		cv.prependOlder(older)
		pos = cv.positionOf(msgID)
	}
	if pos < 0 {
		return
	}
	cv.listView.ScrollTo(uint(pos), gtk.ListScrollNone, nil)
}

// touchRow forces the row at pos to rebind (re-run bindRow), by splicing the
// same message back into the model at that position — the cheapest way to
// get GtkListView to re-render a row that isn't otherwise changing.
func (cv *ConversationView) touchRow(pos int) {
	if pos < 0 || pos >= len(cv.msgs) {
		return
	}
	cv.model.Splice(pos, 1, cv.msgs[pos])
}

// applyHighlights re-renders every row that was highlighted before (so a
// narrowed/cleared query drops stale highlights) or is highlighted now (so a
// widened query picks up new ones), then remembers the new set.
func (cv *ConversationView) applyHighlights(hits []client.SearchHit) {
	touched := make(map[int]bool, len(cv.highlightedPositions)+len(hits))
	for pos := range cv.highlightedPositions {
		touched[pos] = true
	}
	next := make(map[int]bool, len(hits))
	for _, h := range hits {
		if pos := cv.positionOf(h.MsgID); pos >= 0 {
			touched[pos] = true
			next[pos] = true
		}
	}
	for pos := range touched {
		cv.touchRow(pos)
	}
	cv.highlightedPositions = next
}

// searchHitCountText is the pure hit-counter label for idx (currently
// selected, 0-based) of total hits. The mockup renders this as a compact
// mono "1/1" inside the 46px header strip, where the old sentence-length
// label would not fit beside the entry and its three buttons.
func searchHitCountText(idx, total int) string {
	if total == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d", idx+1, total)
}

// searchHitStep returns the next hit index stepping from cur (which may be
// -1 for "no current hit") among total hits, wrapping around at either end.
func searchHitStep(cur, total int, forward bool) int {
	if total <= 0 {
		return -1
	}
	if cur < 0 {
		if forward {
			return 0
		}
		return total - 1
	}
	if forward {
		return (cur + 1) % total
	}
	return (cur - 1 + total) % total
}

// pangoEscapeText escapes text's Pango-markup-significant characters so it's
// safe to embed as literal content inside a <span> markup string.
func pangoEscapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\'':
			b.WriteString("&#39;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// matchRange is a half-open byte range [Start, End) in text matched by
// findMatches.
type matchRange struct{ Start, End int }

// findMatches locates every case-insensitive, non-overlapping occurrence of
// query in text, returning byte ranges into the ORIGINAL text. It walks text
// rune-by-rune and compares unicode.ToLower per rune, so it never slices a
// lower-cased copy whose byte length can differ from the original (e.g.
// Turkish "İ" U+0130 is 2 bytes but folds to 1-byte "i") — every boundary is
// a real rune boundary in text. An empty query matches nothing.
func findMatches(text, query string) []matchRange {
	qr := []rune(query)
	for i := range qr {
		qr[i] = unicode.ToLower(qr[i])
	}
	if len(qr) == 0 {
		return nil
	}

	var ranges []matchRange
	i := 0
	for i < len(text) {
		if end, ok := matchAt(text, i, qr); ok {
			ranges = append(ranges, matchRange{i, end})
			i = end
			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}
	return ranges
}

// matchAt reports whether the lower-cased query runes qr match text starting
// at byte offset start, and if so returns the byte offset just past the match
// in the original text.
func matchAt(text string, start int, qr []rune) (end int, ok bool) {
	pos := start
	for _, want := range qr {
		if pos >= len(text) {
			return 0, false
		}
		r, size := utf8.DecodeRuneInString(text[pos:])
		if unicode.ToLower(r) != want {
			return 0, false
		}
		pos += size
	}
	return pos, true
}

// highlightMarkup renders text as Pango markup with every case-insensitive
// occurrence of query wrapped in a highlight <span>, escaping everything
// else so the result is safe to pass to gtk.Label.SetMarkup.
func highlightMarkup(text, query string) string {
	ranges := findMatches(text, query)
	if len(ranges) == 0 {
		return pangoEscapeText(text)
	}

	var b strings.Builder
	prev := 0
	for _, r := range ranges {
		b.WriteString(pangoEscapeText(text[prev:r.Start]))
		b.WriteString(`<span background="#f5c518" foreground="#1b1b1b">`)
		b.WriteString(pangoEscapeText(text[r.Start:r.End]))
		b.WriteString(`</span>`)
		prev = r.End
	}
	b.WriteString(pangoEscapeText(text[prev:]))
	return b.String()
}

// mediaCount is the info card's "Media, links and docs" value: everything
// the media page would list, or "" when there is nothing (the mockup's
// row shows a number, not a zero).
func (cv *ConversationView) mediaCount(jid string) string {
	media, _ := cv.c.ChatMedia(jid)
	links, _ := cv.c.ChatLinks(jid)
	docs, _ := cv.c.ChatDocs(jid)
	n := len(media) + len(links) + len(docs)
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
