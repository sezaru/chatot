package ui

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// bubbleView holds the pure, pre-rendered display fields for one message
// bubble. bubbleVM computes it from a client.Message so it can be
// unit-tested without a display.
type bubbleView struct {
	Text             string
	TimeText         string
	FromMe           bool
	ShowDaySeparator bool
	DayText          string
	QuotedText       string
	HasQuote         bool
	Reactions        []reactionView
	MediaChip        string
	IsMedia          bool
	Media            mediaView
	IsLocation       bool
	Location         locationView
	IsContact        bool
	Contact          contactView
	IsPoll           bool
	Poll             pollView
	IsEvent          bool
	Event            eventView
	Edited           bool
	EditedMarker     string
	Deleted          bool
	// Forwarded marks a message that carried WhatsApp's forwarded flag; drives
	// the "↩ Forwarded" label above the bubble content.
	Forwarded bool
	// TickText is the own-message delivery/read indicator ("✓"/"✓✓"), set
	// only when FromMe; TickRead marks it should render in the accent color
	// (status == read) rather than the plain dim tick.
	TickText string
	TickRead bool
	// StarGlyph/StarTooltip drive the bubble's star toggle; see starAffordanceVM.
	StarGlyph   string
	StarTooltip string
	// IsEmojiOnly marks a text message that's 1-3 emoji and nothing else: it
	// renders large with no bubble, like a sticker.
	IsEmojiOnly bool
	// Author is the sender's name above an incoming group bubble (mockup:
	// 12px bold accent); "" for 1:1 chats and own messages.
	Author string
	// ShowUnreadSeparator puts the "N unread messages" pill above this
	// bubble: it is the first message the reader hasn't seen yet.
	ShowUnreadSeparator bool
	UnreadText          string
}

// starAffordanceVM derives the bubble star-toggle's glyph and tooltip from
// the message's current starred state.
func starAffordanceVM(starred bool) (glyph, tooltip string) {
	if starred {
		return "★", "Unstar"
	}
	return "☆", "Star"
}

// tombstoneText is what a revoked message renders as, regardless of its
// original content.
const tombstoneText = "🚫 This message was deleted"

// bubbleVM derives the display view-model for a single message. prev is the
// previous message in the thread (nil for the first), used to decide
// whether to show a day separator. byID resolves reply targets among the
// loaded messages. now is injected so Today/Yesterday are deterministic.
func bubbleVM(m client.Message, prev *client.Message, byID map[string]client.Message, now time.Time) bubbleView {
	v := bubbleView{
		TimeText: time.Unix(m.TS, 0).In(now.Location()).Format("15:04"),
		FromMe:   m.FromMe,
	}
	v.StarGlyph, v.StarTooltip = starAffordanceVM(m.Starred)

	if prev == nil || !sameDay(prev.TS, m.TS, now.Location()) {
		v.ShowDaySeparator = true
		v.DayText = dayText(m.TS, now)
	}

	if m.Deleted {
		v.Deleted = true
		v.Text = tombstoneText
		return v
	}

	v.Forwarded = m.Forwarded

	if m.ReplyTo != nil {
		v.HasQuote = true
		if q, ok := byID[m.ReplyTo.MsgID]; ok {
			v.QuotedText = q.Text
			if v.QuotedText == "" {
				v.QuotedText = "↩ reply"
			}
		} else {
			v.QuotedText = "↩ reply"
		}
	}

	v.Reactions = reactionViews(m.Reactions)

	if m.Edited {
		v.Edited = true
		v.EditedMarker = " · edited"
	}

	if m.FromMe {
		v.TickText, v.TickRead = tickVM(m.Status)
	}

	switch {
	case m.Location != nil:
		v.IsLocation = true
		v.Location = locationVM(m)
	case m.Contact != nil:
		v.IsContact = true
		v.Contact = contactVM(m)
	case m.Poll != nil:
		v.IsPoll = true
		v.Poll = pollVM(m)
	case m.EventInvite != nil:
		v.IsEvent = true
		v.Event = eventVM(m)
	case m.Attachment != nil:
		v.IsMedia = true
		v.Media = mediaVM(m)
		v.MediaChip = v.Media.Chip
	default:
		v.Text = m.Text
		v.IsEmojiOnly = isEmojiOnly(m.Text)
	}

	return v
}

// tickVM maps an outgoing message's delivery/read status to its WhatsApp-style
// tick glyph: 0 (sent) -> single check, 1 (delivered) or 2 (read) -> double
// check, the latter flagged for accent-color rendering.
func tickVM(status int) (text string, read bool) {
	if status >= client.MessageStatusDelivered {
		return "✓✓", status >= client.MessageStatusRead
	}
	return "✓", false
}

// mediaChip is the one-line stand-in for an attachment (see
// attachmentPreview).
func mediaChip(a client.Attachment) string { return attachmentPreview(a) }

func sameDay(a, b int64, loc *time.Location) bool {
	ta := time.Unix(a, 0).In(loc)
	tb := time.Unix(b, 0).In(loc)
	return ta.Year() == tb.Year() && ta.YearDay() == tb.YearDay()
}

// dayText renders ts as "Today", "Yesterday", or "02/01/2006" relative to now.
func dayText(ts int64, now time.Time) string {
	t := time.Unix(ts, 0).In(now.Location())
	if sameDay(ts, now.Unix(), now.Location()) {
		return "Today"
	}
	yesterday := now.AddDate(0, 0, -1)
	if sameDay(ts, yesterday.Unix(), now.Location()) {
		return "Yesterday"
	}
	return t.Format("02/01/2006")
}

// ConversationView is the content pane: a live thread of message bubbles
// backed by a client.Client, loaded per-chat by JID.
type ConversationView struct {
	*gtk.Box

	c      client.Client
	events <-chan client.Event
	jid    string // "" until a chat is loaded

	header        *gtk.WindowHandle
	headerContent *gtk.Box // avatar+title box; hidden (not the whole bar) when no chat is open
	avatarSlot    *gtk.Box
	avatarCache   *avatarCache
	avatarJID     string // jid the avatar widget currently shows, "" until set
	titleLabel    *gtk.Label
	subtitleLabel *gtk.Label

	headerMenuPop *gtk.Popover // ⋮ header menu; dev-hook popup target
	window        *gtk.Window  // parent for the group-info dialog; set via SetWindow
	scroller      *gtk.ScrolledWindow
	// model mirrors msgs 1:1 and listView shows it; see thread_rows.go.
	model    *gioutil.ListModel[client.Message]
	listView *gtk.ListView
	// sticky is set while the reader is at the foot of the thread and
	// autoScrolling while a scroll of ours is landing; autoGen tells a
	// stale grace timer from the current one (thread_scroll.go).
	sticky        bool
	autoScrolling bool
	autoGen       int
	// lastUpper and lastPage are the geometry seen by the last scroll
	// event, to tell a re-layout from the reader scrolling.
	lastUpper, lastPage float64
	// flingGen tells a running fling (thread_input.go) from a stopped one.
	flingGen int
	empty    *gtk.Label
	emptyBox *gtk.Box

	// msgs mirrors rows 1:1 and is the authoritative slice fillRow indexes
	// by position (for the message and its predecessor, which
	// bubbleVM needs for day separators). byID resolves reply targets across
	// the whole loaded window.
	msgs []client.Message
	byID map[string]client.Message

	// Pagination: Load fetches only the newest conversationPageSize messages;
	// scrolling near the top prepends older pages until the store runs dry.
	// oldestID is the cursor (the oldest currently-rendered message); hasMore
	// is cleared once a short page comes back; loadingOlder guards against
	// re-entrant fetches while one is in flight / while the anchor is restored.
	oldestID     string
	hasMore      bool
	loadingOlder bool
	// historyRequested is set once loadOlder has asked the phone for more
	// history and stays set until a batch actually arrives (reset on a genuine
	// non-empty prepend), so a second empty page after a request means
	// "genuinely no more". Reset in Load too. historyInFlight guards the window
	// between firing a request and its EventHistorySync reply, so a rapid
	// second scroll can't prematurely mark the thread exhausted.
	historyRequested bool
	historyInFlight  bool

	// presence is the UI's own view of contact/chat presence, built from
	// EventPresence/EventChatPresence on Events() — chosen over growing the
	// Client interface with a Presence(jid) getter, since ConversationView
	// (and ChatList, for the typing preview override) already consume that
	// stream directly. Keyed by contact JID for EventPresence (online/
	// last-seen) and by chat JID for EventChatPresence (typing); those
	// coincide for 1:1 chats, which is the only case the header's
	// online/last-seen line is expected to cover.
	presence map[string]PresenceState
	// typingShown is set while the typing sentinel is the model's last
	// item; it renders as the mockup's dotted bubble at the foot of the
	// thread (see showTypingRow).
	typingShown bool
	// composingGen counts composing events per chat so the stale-typing
	// timer only clears the state it was armed for.
	composingGen map[string]int
	// chatIsGroup caches whether the open chat is a group (sender names).
	chatIsGroup bool
	// chatInfo is the open chat's row as of the last Load: the chat list
	// query is one pass over every chat, too much to repeat for one name.
	chatInfo client.Chat
	// unreadAnchor is the id of the first message the reader hasn't seen
	// in the open chat ("" for none): the "N unread messages" pill sits
	// above it until the chat is left.
	unreadAnchor string
	// anchorRow is the recycled row currently showing the unread pill and
	// rowMsg which message each row holds, so the pill can be seen to be
	// on screen; seenTimer counts down its stay once it is.
	anchorRow *gtk.Box
	rowMsg    map[*gtk.Box]string
	seenTimer glib.SourceHandle
	// names memoizes ContactName lookups for senders and mentions; it is
	// dropped on EventChatUpdate, which the contact sync fires once new
	// names land.
	names map[string]string

	onReply   func(client.Message)
	onReact   func(msg client.Message, emoji string)
	onVote    func(msg client.Message, options []string)
	onEdit    func(client.Message)
	onDelete  func(client.Message)
	onStar    func(client.Message)
	onForward func(client.Message)
	// onUnreadSeen fires when the unread pill comes down because the reader
	// has looked at it: the chat is read from that moment.
	onUnreadSeen func(jid string, msgs []client.Message)
	// onStopLive ends an own live location share early (the bubble's Stop
	// sharing button).
	onStopLive func(client.Message)
	// onOpenViewer opens an attachment (picture, clip, voice note, file,
	// location) in the content pane's viewer.
	onOpenViewer func(client.Message)

	// onShowMedia/onExportChat/onClearChat are the header "⋮" menu's stubbed
	// seams for F43 (media/links/docs page) and F44 (export/clear chat);
	onShowMedia  func(jid string)
	onExportChat func(jid string)
	onClearChat  func(jid string)

	menuBtn *gtk.Button

	// In-chat search: searchQuery drives fillRow's highlight check (any
	// currently-bound row whose text contains it, case-insensitively, is
	// rendered with Pango highlight markup); searchHits is the ordered
	// (oldest-first) store match set for the open chat, searchIdx the
	// currently-selected hit ("-1" = none). highlightedPositions tracks which
	// cv.msgs positions were last force-rebound so a narrowing query can
	// un-highlight rows that no longer match.
	// timers holds disappearing-message timers set this session, by chat JID
	// (see disappearingTimer).
	timers map[string]int64
	// headerStack swaps the header's identity area for the in-chat search bar.
	headerStack          *gtk.Stack
	searchEntry          *gtk.Entry
	searchHitLabel       *gtk.Label
	searchQuery          string
	searchHits           []client.SearchHit
	searchIdx            int
	highlightedPositions map[int]bool

	// joinBanner shows a "N people requested to join" strip above the thread
	// when the open chat is a group with pending, admin-reviewable join
	// requests; joinBannerReq tracks the in-flight fetch's target jid so a
	// stale response (from a chat the user already navigated away from)
	// can't clobber the banner.
	joinBanner      *gtk.Revealer
	joinBannerLabel *gtk.Label
	joinBannerReq   string

	toastOverlay *adw.ToastOverlay
}

// OnReplyRequested registers f to be called when the user picks the reply
// affordance on a bubble; the composer wires this to StartReply.
func (cv *ConversationView) OnReplyRequested(f func(client.Message)) { cv.onReply = f }

// OnReactRequested registers f to be called when the user picks an emoji
// from a bubble's react affordance; msg carries the ChatJID needed to send.
func (cv *ConversationView) OnReactRequested(f func(msg client.Message, emoji string)) {
	cv.onReact = f
}

// OnVoteRequested registers f to be called when the user clicks a poll option;
// options is the set the user selected (currently always one).
func (cv *ConversationView) OnVoteRequested(f func(msg client.Message, options []string)) {
	cv.onVote = f
}

// OnEditRequested registers f to be called when the user picks the edit
// affordance on one of their own text bubbles; the composer wires this to
// enter edit mode.
func (cv *ConversationView) OnEditRequested(f func(client.Message)) { cv.onEdit = f }

// OnDeleteRequested registers f to be called when the user picks the delete
// affordance on one of their own bubbles.
func (cv *ConversationView) OnDeleteRequested(f func(client.Message)) { cv.onDelete = f }

// OnStarRequested registers f to be called when the user clicks a bubble's
// star toggle, on any message (own or theirs).
func (cv *ConversationView) OnStarRequested(f func(client.Message)) { cv.onStar = f }

// OnForwardRequested registers f to be called when the user picks Forward
// from a bubble's "⋯" menu; msg is the message to forward.
func (cv *ConversationView) OnForwardRequested(f func(client.Message)) { cv.onForward = f }

// OnStopLiveRequested wires the location bubble's Stop sharing button.
func (cv *ConversationView) OnStopLiveRequested(f func(client.Message)) { cv.onStopLive = f }

// OnOpenViewerRequested registers f to open a message's attachment in the
// viewer pane (a tap on a picture, a clip's play pill, a document's View,
// a location tile).
func (cv *ConversationView) OnOpenViewerRequested(f func(client.Message)) { cv.onOpenViewer = f }

// OnShowMediaRequested registers f to be called when the user picks "Media,
// links and docs" from the header menu; STUBBED until F43 builds the page.
func (cv *ConversationView) OnShowMediaRequested(f func(jid string)) { cv.onShowMedia = f }

// OnExportRequested registers f to be called when the user picks "Export
// chat…" from the header menu; STUBBED until F44.
func (cv *ConversationView) OnExportRequested(f func(jid string)) { cv.onExportChat = f }

// OnClearRequested registers f to be called when the user picks "Clear
// chat…" from the header menu; STUBBED until F44.
func (cv *ConversationView) OnClearRequested(f func(jid string)) { cv.onClearChat = f }

// OnUnreadSeen registers the callback that marks a chat read once its
// unread pill has been looked at (a message that arrived in the
// background is read when the window comes back, not when it landed).
func (cv *ConversationView) OnUnreadSeen(f func(jid string, msgs []client.Message)) {
	cv.onUnreadSeen = f
}

// SetWindow supplies the parent window the group-info dialog needs; call
// once after NewConversationView.
func (cv *ConversationView) SetWindow(w *gtk.Window) {
	cv.window = w
	// Coming back to the window is when a pill left by a message that
	// arrived in the background gets seen.
	w.NotifyProperty("is-active", cv.scheduleUnreadClear)
}

// SetToastOverlay supplies the overlay the copy-with-undo toast is shown on;
// call once after NewConversationView.
func (cv *ConversationView) SetToastOverlay(overlay *adw.ToastOverlay) { cv.toastOverlay = overlay }

// Messages returns the currently-loaded thread, for mark-read on open.
func (cv *ConversationView) Messages() []client.Message { return cv.msgs[:cv.threadLen()] }

// CurrentJID returns the chat currently loaded, "" if none.
func (cv *ConversationView) CurrentJID() string { return cv.jid }

// conversationPageSize is how many messages Load fetches up front and each
// scroll-up page adds — small enough that opening a huge chat is instant.
const conversationPageSize = 40

// historySyncRequestSize is how many older messages RequestMoreHistory asks
// the phone for once local paging runs dry.
const historySyncRequestSize = 50

// nextHistoryAction decides what loadOlder should do once MessagesBefore
// returns olderCount messages: a non-empty page means keep paging locally;
// an empty page requests more from the phone the first time (request), and
// only gives up (exhausted) once that's already been tried and is still
// empty.
func nextHistoryAction(olderCount int, alreadyRequested bool) (request, exhausted bool) {
	if olderCount > 0 {
		return false, false
	}
	if alreadyRequested {
		return false, true
	}
	return true, false
}

// NewConversationView builds an empty ConversationView backed by c and
// subscribes to c.Events() for live append.
func NewConversationView(c client.Client) *ConversationView {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-conv-root")
	root.SetVExpand(true)
	root.SetHExpand(true)

	// The header is a real AdwHeaderBar so the window's min/max/close controls
	// render here (the app is a content-only AdwApplicationWindow with no
	// separate titlebar). The bar itself stays visible even with no chat open
	// — only its avatar+title content is hidden — so the controls never vanish.
	//
	// A WindowHandle around a plain box, like the sidebar's account strip,
	// rather than an AdwHeaderBar: the header bar centres its title widget at
	// natural width and caps start-packed children at half the bar, so the
	// mockup's search pill (which runs from the left edge to ⋮) could not
	// be laid out in it. Dragging still works through the handle.
	headerRow := gtk.NewBox(gtk.OrientationHorizontal, 10)
	headerRow.AddCSSClass("chatot-conv-headerrow")
	header := gtk.NewWindowHandle()
	header.SetChild(headerRow)
	header.AddCSSClass("chatot-conv-header")

	headerContent := gtk.NewBox(gtk.OrientationHorizontal, 10)

	avatarSlot := gtk.NewBox(gtk.OrientationVertical, 0)
	avatarSlot.SetVAlign(gtk.AlignCenter)
	headerContent.Append(avatarSlot)

	textCol := gtk.NewBox(gtk.OrientationVertical, 0)
	textCol.SetVAlign(gtk.AlignCenter)
	headerContent.Append(textCol)

	titleLabel := gtk.NewLabel("")
	titleLabel.SetXAlign(0)
	titleLabel.AddCSSClass("chatot-conv-title")
	textCol.Append(titleLabel)

	subtitleLabel := gtk.NewLabel("")
	subtitleLabel.SetXAlign(0)
	subtitleLabel.AddCSSClass("chatot-conv-subtitle")
	textCol.Append(subtitleLabel)

	headerContent.SetVisible(false)
	// GtkWindowControls centred in the strip so the mockup's 24px circles
	// don't stretch to its height (there is no vertical alignment in GTK
	// CSS). No explicit decoration layout: the buttons follow the desktop's
	// gtk-decoration-layout (GNOME's button-layout via the settings portal,
	// or gtk-4.0/settings.ini), so a tiling-WM user can hide them all and
	// a left-side layout lands in the sidebar header's PackStart controls.
	windowControls := newWindowControls(gtk.PackEnd)

	// The mockup's in-chat search REPLACES the header's identity area rather
	// than opening a second row beneath it, so both live in a stack packed at
	// the header's start.
	headerStack := gtk.NewStack()
	headerStack.SetHExpand(true)
	// headerContent hides itself when no chat is open, and GtkStack skips an
	// invisible page — so it goes in a wrapper that always stays visible,
	// otherwise the stack would fall through to the search bar on an empty
	// pane.
	identityPage := gtk.NewBox(gtk.OrientationHorizontal, 0)
	identityPage.SetHExpand(true)
	identityPage.Append(headerContent)
	headerStack.AddNamed(identityPage, "identity")
	headerRow.Append(headerStack)

	// Text ⋮ like the sidebar's app menu: the mockup shows vertical dots and
	// view-more-symbolic's orientation varies by icon theme.
	menuBtn := gtk.NewButtonWithLabel("⋮")
	menuBtn.AddCSSClass("flat")
	menuBtn.AddCSSClass("chatot-hdr-icon")
	// Centred, or the header bar stretches it to its full height and the
	// 28px square becomes a tall rectangle.
	menuBtn.SetVAlign(gtk.AlignCenter)
	menuBtn.SetTooltipText("Chat options")
	menuBtn.SetSensitive(false)

	// Only ⋮ beside the window controls: the mockup keeps Group info inside
	// the menu, and an icon-theme button here rendered blank on themes
	// without the icon.
	headerRow.Append(menuBtn)
	headerRow.Append(windowControls)

	// The mockup's search is one 32px white pill holding the field, the hit
	// counter and three 24px round glyph buttons — not a GtkSearchEntry (its
	// magnifier and clear icons are not in the design) with buttons beside it.
	searchBar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	searchBar.AddCSSClass("chatot-conv-searchbar")
	searchBar.SetVAlign(gtk.AlignCenter)
	searchBar.SetHExpand(true)

	searchPill := gtk.NewBox(gtk.OrientationHorizontal, 8)
	searchPill.AddCSSClass("chatot-conv-searchpill")
	searchPill.SetHExpand(true)
	searchBar.Append(searchPill)

	searchEntry := gtk.NewEntry()
	searchEntry.SetPlaceholderText("Search in this chat")
	searchEntry.SetHasFrame(false)
	searchEntry.SetHExpand(true)
	searchEntry.AddCSSClass("chatot-conv-search-entry")
	searchPill.Append(searchEntry)

	searchHitLabel := gtk.NewLabel("")
	searchHitLabel.AddCSSClass("chatot-conv-search-count")
	searchHitLabel.SetVAlign(gtk.AlignCenter)
	searchPill.Append(searchHitLabel)

	searchGlyphBtn := func(glyph, tip string) *gtk.Button {
		b := gtk.NewButtonWithLabel(glyph)
		b.AddCSSClass("flat")
		b.RemoveCSSClass("text-button")
		b.AddCSSClass("chatot-conv-search-btn")
		b.SetTooltipText(tip)
		b.SetVAlign(gtk.AlignCenter)
		return b
	}
	searchUpBtn := searchGlyphBtn("▲", "Previous match")
	searchPill.Append(searchUpBtn)
	searchDownBtn := searchGlyphBtn("▼", "Next match")
	searchPill.Append(searchDownBtn)
	searchCloseBtn := searchGlyphBtn("✕", "Close search")
	searchPill.Append(searchCloseBtn)

	headerStack.AddNamed(searchBar, "search")
	headerStack.SetVisibleChildName("identity")

	joinBannerBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	joinBannerBox.AddCSSClass("chatot-join-banner")
	joinBannerBox.SetHAlign(gtk.AlignCenter)
	joinBannerBox.SetMarginTop(8)
	joinBannerBox.SetMarginBottom(2)

	joinBannerLabel := gtk.NewLabel("")
	joinBannerLabel.AddCSSClass("chatot-join-text")
	joinBannerBox.Append(joinBannerLabel)

	joinReviewBtn := gtk.NewButtonWithLabel("Review")
	joinReviewBtn.AddCSSClass("flat")
	joinBannerBox.Append(joinReviewBtn)

	joinBanner := gtk.NewRevealer()
	joinBanner.SetChild(joinBannerBox)
	joinBanner.SetRevealChild(false)
	root.Append(joinBanner)

	emptyBox := gtk.NewBox(gtk.OrientationVertical, 8)
	emptyBox.SetVExpand(true)
	emptyBox.SetVAlign(gtk.AlignCenter)
	emptyBox.SetHAlign(gtk.AlignCenter)
	emptyIcon := newEmptyChatGlyph(64)
	emptyIcon.AddCSSClass("chatot-placeholder")
	emptyBox.Append(emptyIcon)
	empty := gtk.NewLabel("Select a chat")
	empty.AddCSSClass("chatot-placeholder")
	emptyBox.Append(empty)
	root.Append(emptyBox)

	scroller := gtk.NewScrolledWindow()

	cv := &ConversationView{
		Box:           root,
		c:             c,
		events:        c.Events(),
		rowMsg:        map[*gtk.Box]string{},
		header:        header,
		headerContent: headerContent,
		avatarSlot:    avatarSlot,
		avatarCache:   newAvatarCache(),
		titleLabel:    titleLabel,
		subtitleLabel: subtitleLabel,

		menuBtn:              menuBtn,
		headerStack:          headerStack,
		searchEntry:          searchEntry,
		searchHitLabel:       searchHitLabel,
		searchIdx:            -1,
		highlightedPositions: make(map[int]bool),
		scroller:             scroller,
		model:                threadModelType.New(),
		empty:                empty,
		emptyBox:             emptyBox,
		presence:             make(map[string]PresenceState),
		composingGen:         make(map[string]int),
		names:                make(map[string]string),
		timers:               make(map[string]int64),
		joinBanner:           joinBanner,
		joinBannerLabel:      joinBannerLabel,
	}

	joinReviewBtn.ConnectClicked(func() {
		if cv.jid != "" {
			showJoinRequestsDialog(cv.window, cv.c, cv.jid, func() { cv.refreshJoinBanner() })
		}
	})

	cv.setupHeaderMenu(menuBtn)

	searchEntry.ConnectChanged(func() {
		cv.runSearch(searchEntry.Text())
	})
	keyController := gtk.NewEventControllerKey()
	keyController.SetPropagationPhase(gtk.PhaseCapture)
	keyController.ConnectKeyPressed(func(keyval, _ uint, state gdk.ModifierType) bool {
		switch keyval {
		case gdk.KEY_Return, gdk.KEY_KP_Enter:
			cv.stepHit(state&gdk.ShiftMask == 0)
			return true
		case gdk.KEY_Escape:
			cv.closeSearchBar()
			return true
		}
		return false
	})
	searchEntry.AddController(keyController)
	searchUpBtn.ConnectClicked(func() { cv.stepHit(false) })
	searchDownBtn.ConnectClicked(func() { cv.stepHit(true) })
	searchCloseBtn.ConnectClicked(func() { cv.closeSearchBar() })

	scroller.SetVExpand(true)
	scroller.SetHExpand(true)
	// Keep the scroller's own height request minimal so a tall thread scrolls
	// internally instead of growing the pane and pushing the composer (a
	// sibling below this view) off the bottom of the window.
	scroller.SetPropagateNaturalHeight(false)
	scroller.SetMinContentHeight(0)
	// Never scroll horizontally: a long message must wrap within the pane, not
	// widen it (which would push content off the right edge like the sidebar
	// list did vertically).
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	cv.listView = cv.newThreadList()
	scroller.SetChild(cv.listView)
	scroller.SetVisible(false)

	root.Append(scroller)

	adj := scroller.VAdjustment()
	adj.ConnectValueChanged(cv.onScroll)
	adj.NotifyProperty("upper", cv.onUpperChanged)
	adj.NotifyProperty("page-size", cv.onUpperChanged)
	cv.installScrollInput()

	go cv.watchEvents()

	return cv
}

// fillRow renders the message at pos into its row box, rebuilding the
// bubble from scratch. Called when a row is created or its message (or
// its predecessor, for day separators and reply quotes) changed.
func (cv *ConversationView) fillRow(box *gtk.Box, pos int) {
	removeAllChildren(box)
	trace(2, "fill row %d of %d", pos, len(cv.msgs))
	if pos < 0 || pos >= len(cv.msgs) {
		return
	}
	msg := cv.msgs[pos]
	if msg.ID == typingSentinelID {
		box.Append(newTypingBubble())
		return
	}
	var prev *client.Message
	if pos > 0 {
		prev = &cv.msgs[pos-1]
	}
	vm := bubbleVM(msg, prev, cv.byID, time.Now())
	if cv.chatIsGroup && !msg.FromMe {
		vm.Author = cv.senderName(msg.FromJID)
	}
	cv.rowMsg[box] = msg.ID
	if cv.unreadAnchor != "" && msg.ID == cv.unreadAnchor {
		vm.ShowUnreadSeparator = true
		vm.UnreadText = unreadSeparatorText(cv.threadLen() - pos)
		cv.anchorRow = box
		glib.IdleAdd(cv.scheduleUnreadClear)
	}
	box.Append(buildBubble(msg, vm, cv.hooks()))
}

// unreadSeenDelay is how long the unread pill stays once its message is on
// screen in the active window before it is taken down.
const unreadSeenDelay = 3 * time.Second

// anchorSeen tells whether the reader is looking at the unread pill: the
// window is active and the pill's row is inside the scroller's viewport.
func (cv *ConversationView) anchorSeen() bool {
	if cv.unreadAnchor == "" || cv.window == nil || !cv.window.IsActive() {
		return false
	}
	row := cv.anchorRow
	if row == nil || cv.rowMsg[row] != cv.unreadAnchor {
		return false
	}
	b, ok := row.ComputeBounds(cv.scroller)
	if !ok {
		return false
	}
	return rowOnScreen(float64(b.Y()), float64(b.Height()), float64(cv.scroller.AllocatedHeight()))
}

// rowOnScreen tells whether a row spanning [y, y+h) in viewport coordinates
// overlaps a viewport of the given height.
func rowOnScreen(y, h, viewport float64) bool {
	return y+h > 0 && y < viewport
}

// scheduleUnreadClear starts the pill's countdown once it is being looked
// at; the countdown checks again when it ends, so glancing away keeps it.
func (cv *ConversationView) scheduleUnreadClear() {
	if cv.seenTimer != 0 || !cv.anchorSeen() {
		return
	}
	cv.seenTimer = glib.TimeoutAdd(uint(unreadSeenDelay/time.Millisecond), func() bool {
		cv.seenTimer = 0
		if cv.anchorSeen() {
			cv.clearUnreadPill()
		}
		return false
	})
}

// clearUnreadPill takes the pill down by re-rendering its row.
func (cv *ConversationView) clearUnreadPill() {
	id := cv.unreadAnchor
	cv.unreadAnchor = ""
	cv.anchorRow = nil
	if cv.onUnreadSeen != nil {
		cv.onUnreadSeen(cv.jid, cv.Messages())
	}
	cv.refillRow(cv.positionOf(id))
}

// unreadSeparatorText is the pill above the first unseen message.
func unreadSeparatorText(n int) string {
	if n == 1 {
		return "1 unread message"
	}
	return strconv.Itoa(n) + " unread messages"
}

// unreadAnchorFor picks the message the "unread" pill goes above when a
// chat opens with unread messages: the first of the trailing unread ones,
// provided it is the other side's (an own message never reads as unread).
func unreadAnchorFor(msgs []client.Message, unread int) string {
	if unread <= 0 || unread > len(msgs) {
		return ""
	}
	first := msgs[len(msgs)-unread]
	if first.FromMe {
		return ""
	}
	return first.ID
}

// Load fetches the newest page of jid's thread and renders it, replacing
// whatever was shown before; older messages are pulled in on scroll-up (see
// loadOlder). Must run on the GTK main loop.
func (cv *ConversationView) Load(jid string) {
	trace(1, "Load %s", jid)
	// A reload of the currently-open chat (receipts/reactions/revokes/poll
	// votes all trigger one) keeps the search bar open; only an actual chat
	// switch resets it.
	if cv.jid != jid {
		cv.closeSearchBar()
		cv.unreadAnchor = ""
		// A voice note playing in the chat being left stops.
		pauseVoicePlayers()
	}
	cv.jid = jid
	chat := chatByJID(cv.c, jid)
	cv.chatInfo = chat
	cv.chatIsGroup = chat.IsGroup
	cv.loadingOlder = false
	cv.historyRequested = false
	cv.historyInFlight = false
	cv.refreshHeader()
	cv.refreshJoinBanner()

	msgs, err := cv.c.Messages(jid, conversationPageSize)
	if err != nil {
		msgs = nil
	}
	cv.msgs = msgs
	cv.byID = indexByID(msgs)
	cv.hasMore = len(msgs) == conversationPageSize
	if len(msgs) > 0 {
		cv.oldestID = msgs[0].ID
	} else {
		cv.oldestID = ""
	}
	if cv.unreadAnchor == "" {
		cv.unreadAnchor = unreadAnchorFor(msgs, chat.UnreadCount)
	}

	// Replace every row with the new page; the typing sentinel (if any)
	// goes with it and is re-added below.
	cv.typingShown = false
	cv.model.Splice(0, cv.model.Len(), msgs...)

	if len(msgs) == 0 {
		cv.empty.SetLabel("No messages yet")
		cv.emptyBox.SetVisible(true)
		cv.scroller.SetVisible(false)
		return
	}

	cv.emptyBox.SetVisible(false)
	cv.scroller.SetVisible(true)
	// Presence is per-chat, so a switch re-evaluates the typing bubble
	// rather than leaving the previous chat's state showing.
	cv.refreshTypingBubble()
	cv.scrollToBottom()
}

// bubbleSig captures the render-affecting, mutable fields of a message —
// everything a live receipt/reaction/revoke/edit/poll-vote can change — so
// refreshInPlace can cheaply tell which already-rendered rows actually need
// re-binding and leave the rest (and the scroll position) alone.
func bubbleSig(m client.Message) string {
	var b strings.Builder
	b.WriteString(m.ID)
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(m.Status))
	if m.Deleted {
		b.WriteByte('D')
	}
	if m.Edited {
		b.WriteByte('E')
	}
	if m.Starred {
		b.WriteByte('S')
	}
	b.WriteByte('|')
	b.WriteString(m.Text)
	if len(m.Reactions) > 0 {
		// Reactors, not just emojis: a second 👍 changes the pill's count and
		// the row has to rebind for it.
		b.WriteByte('|')
		for _, r := range reactionViews(m.Reactions) {
			b.WriteString(r.Emoji)
			b.WriteString(strings.Join(r.Reactors, ","))
			b.WriteByte(';')
		}
	}
	if m.Poll != nil {
		b.WriteByte('|')
		for _, o := range m.Poll.Options {
			b.WriteString(o.Name)
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(o.Count))
			if o.Voted {
				b.WriteByte('*')
			}
		}
	}
	return b.String()
}

// refreshInPlace re-renders only the rows whose content actually changed
// (delivery ticks, reactions, revokes, poll tallies, edits), leaving every
// other row — and the scroll position — untouched. This replaces the old
// full cv.Load() reload that fired on every one of those events: Load splices
// the entire model and jumps to the bottom, which on a busy chat (receipts
// arrive constantly) rebinds every visible bubble many times a second and
// reads as heavy flicker/lag. Falls back to a full Load only when the loaded
// set no longer lines up positionally (a genuine add/remove, or the user has
// paged history beyond the refetch window). Must run on the GTK main loop.
func (cv *ConversationView) refreshInPlace() {
	if cv.jid == "" {
		return
	}
	base := cv.threadLen()
	n := base
	if n < conversationPageSize {
		n = conversationPageSize
	}
	msgs, err := cv.c.Messages(cv.jid, n)
	if err != nil || len(msgs) != base {
		cv.Load(cv.jid)
		return
	}
	for i := range msgs {
		if msgs[i].ID != cv.msgs[i].ID {
			cv.Load(cv.jid)
			return
		}
	}
	changed := 0
	for i := range msgs {
		if bubbleSig(msgs[i]) == bubbleSig(cv.msgs[i]) {
			continue
		}
		cv.msgs[i] = msgs[i]
		cv.byID[msgs[i].ID] = msgs[i]
		cv.refillRow(i)
		changed++
	}
	trace(1, "refreshInPlace: %d rows changed", changed)
}

// loadOlder prepends the next older page. Must run on the GTK main loop.
func (cv *ConversationView) loadOlder() {
	cv.loadingOlder = true

	older, err := cv.c.MessagesBefore(cv.jid, cv.oldestID, conversationPageSize)
	if err != nil {
		cv.hasMore = false
		cv.loadingOlder = false
		return
	}
	if len(older) == 0 {
		request, exhausted := nextHistoryAction(len(older), cv.historyRequested)
		trace(1, "loadOlder: store floor reached; request=%v exhausted=%v", request, exhausted)
		cv.hasMore = !exhausted
		cv.loadingOlder = false
		if request {
			cv.historyRequested = true
			cv.historyInFlight = true
			jid, oldestID := cv.jid, cv.oldestID
			go func() {
				_ = cv.c.RequestMoreHistory(context.Background(), jid, oldestID, historySyncRequestSize)
			}()
		}
		return
	}

	// A genuine older page arrived (from local store or a landed history
	// sync). The list view keeps the row under the reader where it is.
	trace(1, "loadOlder: prepend %d (oldest %s)", len(older), cv.oldestID)
	cv.prependOlder(older)
	cv.loadingOlder = false
}

// prependOlder splices older (oldest-first) onto the front of the currently
// loaded thread, updating the pagination cursor/index. Shared by loadOlder
// (scroll-up paging) and jumpToMessage (loading pages synchronously to reach
// a search hit that isn't loaded yet). historyRequested is cleared so hitting
// the store's floor again re-requests the next batch from the phone, one
// page at a time rather than stopping after one. Must run on the GTK main
// loop.
func (cv *ConversationView) prependOlder(older []client.Message) {
	cv.historyRequested = false
	cv.msgs = append(older, cv.msgs...)
	cv.oldestID = cv.msgs[0].ID
	cv.byID = indexByID(cv.msgs[:cv.threadLen()])
	cv.hasMore = len(older) == conversationPageSize
	cv.model.Splice(0, 0, older...)
	// The old first row's predecessor changed (its day separator goes).
	cv.refillRow(len(older))
}

// watchEvents listens for client events and, for the currently-loaded chat,
// schedules a UI update on the GTK main loop via glib.IdleAdd. New messages
// are appended in place; reactions and receipts (delivery/read ticks) trigger
// a full reload (simpler, and the thread sizes here don't warrant a targeted
// patch). Presence/chat-presence
// events update cv.presence unconditionally (so it's warm when the user
// switches to that chat) but only repaint the header when they're for the
// currently-open chat.
func (cv *ConversationView) watchEvents() {
	for ev := range cv.events {
		switch ev.Kind {
		case client.EventMessage:
			if ev.Message == nil {
				continue
			}
			msg := *ev.Message
			glib.IdleAdd(func() {
				// The peer's message ends their typing burst whether or
				// not a "paused" notice follows.
				if !msg.FromMe {
					cv.clearComposing(msg.ChatJID)
				}
				if msg.ChatJID != cv.jid {
					return
				}
				// An edit updates an existing row (keyed to the original id),
				// so reload the thread rather than append a duplicate bubble.
				if msg.Edited {
					cv.Load(cv.jid)
					return
				}
				cv.appendMessage(msg)
			})
		case client.EventReceipt:
			if ev.Receipt == nil {
				continue
			}
			chatJID := ev.Receipt.ChatJID
			glib.IdleAdd(func() {
				if chatJID != cv.jid {
					return
				}
				cv.refreshInPlace()
			})
		case client.EventReaction:
			if ev.Reaction == nil {
				continue
			}
			chatJID := ev.Reaction.ChatJID
			glib.IdleAdd(func() {
				if chatJID != cv.jid {
					return
				}
				cv.refreshInPlace()
			})
		case client.EventRevoke:
			if ev.Revoke == nil {
				continue
			}
			chatJID := ev.Revoke.ChatJID
			glib.IdleAdd(func() {
				if chatJID != cv.jid {
					return
				}
				cv.refreshInPlace()
			})
		case client.EventPollVote:
			if ev.PollVote == nil {
				continue
			}
			chatJID := ev.PollVote.ChatJID
			glib.IdleAdd(func() {
				if chatJID != cv.jid {
					return
				}
				cv.refreshInPlace()
			})
		case client.EventPresence:
			if ev.Presence == nil {
				continue
			}
			p := *ev.Presence
			glib.IdleAdd(func() {
				state := cv.presence[p.JID]
				state.Online = p.Online
				if p.LastSeen != 0 {
					state.LastSeen = time.Unix(p.LastSeen, 0)
				}
				cv.presence[p.JID] = state
				if p.JID == cv.jid {
					cv.refreshHeader()
				}
			})
		case client.EventChatPresence:
			if ev.ChatPresence == nil {
				continue
			}
			cp := *ev.ChatPresence
			glib.IdleAdd(func() {
				state := cv.presence[cp.ChatJID]
				state.Typing, state.Recording = chatPresenceTypingRecording(cp.State, cp.Media)
				cv.presence[cp.ChatJID] = state
				cv.composingGen[cp.ChatJID]++
				if state.Typing || state.Recording {
					gen := cv.composingGen[cp.ChatJID]
					jid := cp.ChatJID
					glib.TimeoutSecondsAdd(composingStaleSecs, func() bool {
						if cv.composingGen[jid] == gen {
							cv.clearComposing(jid)
						}
						return false
					})
				}
				if cp.ChatJID == cv.jid {
					cv.refreshHeader()
					cv.refreshTypingBubble()
				}
			})
		case client.EventHistorySync:
			if ev.HistorySync == nil {
				continue
			}
			jids := ev.HistorySync.ChatJIDs
			glib.IdleAdd(func() {
				if cv.loadingOlder || !cv.historyRequested {
					return
				}
				for _, j := range jids {
					if j == cv.jid {
						cv.historyInFlight = false
						cv.loadOlder()
						return
					}
				}
			})
		case client.EventChatUpdate:
			if ev.ChatUpdate == nil {
				continue
			}
			jid := ev.ChatUpdate.JID
			glib.IdleAdd(func() {
				// Contact names may have changed (the sync fires this).
				cv.names = make(map[string]string)
				if jid == cv.jid {
					cv.chatInfo = client.Chat{}
					cv.refreshJoinBanner()
				}
			})
		case client.EventAvatar:
			if ev.Avatar == nil {
				continue
			}
			jid := ev.Avatar.JID
			glib.IdleAdd(func() {
				cv.avatarCache.invalidate(jid)
				if jid == cv.jid {
					cv.avatarJID = "" // force refreshHeader to rebuild the avatar widget
					cv.refreshHeader()
				}
			})
		}
	}
}

// conversationAvatarSize is the conversation header avatar's fixed square
// size in px — bigger than the chat-list row's since there's just the one.
const conversationAvatarSize = 28

// refreshHeader repaints the title/subtitle/avatar for the currently-open
// chat (hides the whole header if none is open). Must run on the GTK main
// loop. The avatar widget is only rebuilt when the open jid changes (tracked
// via avatarJID), so a presence-driven refreshHeader doesn't restart the
// async fetch or cause flicker on every presence update.
func (cv *ConversationView) refreshHeader() {
	if cv.jid == "" {
		cv.headerContent.SetVisible(false)
		cv.menuBtn.SetSensitive(false)
		return
	}
	name := cv.chatName(cv.jid)
	cv.titleLabel.SetLabel(name)
	cv.subtitleLabel.SetLabel(presenceSubtitle(cv.presence[cv.jid], time.Now()))

	cv.headerContent.SetVisible(true)
	cv.menuBtn.SetSensitive(true)

	if cv.avatarJID != cv.jid {
		cv.avatarJID = cv.jid
		removeAllChildren(cv.avatarSlot)
		cv.avatarSlot.Append(buildAvatar(cv.c, cv.avatarCache, cv.jid, initialFor(name), conversationAvatarSize))
	}
}

// refreshJoinBanner hides the join-request banner and, for a group chat,
// re-fetches its pending requests off the GTK main loop to repopulate it.
// Must run on the GTK main loop; the fetch itself does not.
func (cv *ConversationView) refreshJoinBanner() {
	cv.joinBanner.SetRevealChild(false)
	jid := cv.jid
	cv.joinBannerReq = jid
	if jid == "" || !strings.HasSuffix(jid, "@g.us") {
		return
	}
	go func() {
		reqs, err := cv.c.GroupJoinRequests(context.Background(), jid)
		glib.IdleAdd(func() {
			if cv.joinBannerReq != jid || cv.jid != jid {
				return
			}
			text := joinRequestBannerText(len(reqs))
			if err != nil || text == "" {
				cv.joinBanner.SetRevealChild(false)
				return
			}
			cv.joinBannerLabel.SetLabel(text)
			cv.joinBanner.SetRevealChild(true)
		})
	}()
}

// chatName looks up jid's display name from the chat list, falling back to
// the raw JID if it isn't found (e.g. a chat not yet synced into the store).
func (cv *ConversationView) chatName(jid string) string {
	if jid == cv.chatInfo.JID && cv.chatInfo.Name != "" {
		return cv.chatInfo.Name
	}
	chats, err := cv.c.Chats(0)
	if err != nil {
		return jid
	}
	for _, c := range chats {
		if c.JID == jid && c.Name != "" {
			return c.Name
		}
	}
	return jid
}

// AppendSentMessage appends an optimistic echo of a just-sent message if
// it belongs to the currently-open chat. Must run on the GTK main loop
// (the composer calls it from within a glib.IdleAdd).
func (cv *ConversationView) AppendSentMessage(msg client.Message) {
	if msg.ChatJID != cv.jid {
		return
	}
	cv.appendMessage(msg)
}

// ApplyOwnReaction re-renders the thread so a just-sent own reaction shows
// immediately, if chatJID is the currently-open chat. It reloads from the
// store (idempotent), so a later echo EventReaction for the same reaction
// re-runs the same reload harmlessly. Must run on the GTK main loop.
func (cv *ConversationView) ApplyOwnReaction(chatJID string) {
	if chatJID != cv.jid {
		return
	}
	cv.refreshInPlace()
}

// appendMessage adds msg to the end of the currently-loaded thread. The
// factory renders the new row when it realizes at the bottom. Must run on the
// GTK main loop.
func (cv *ConversationView) appendMessage(msg client.Message) {
	trace(1, "appendMessage %s", msg.ID)
	if !cv.typingShown {
		cv.msgs = append(cv.msgs, msg)
	}
	cv.byID[msg.ID] = msg

	if cv.emptyBox.Visible() {
		cv.emptyBox.SetVisible(false)
		cv.scroller.SetVisible(true)
	}

	// A message that lands while the window is in the background is one
	// the reader hasn't seen: the unread pill goes above the first such.
	if !msg.FromMe && cv.unreadAnchor == "" && cv.window != nil && !cv.window.IsActive() {
		cv.unreadAnchor = msg.ID
	}

	// Follow the thread only when the reader is already at its foot (or
	// just sent this message); someone scrolled up reading history keeps
	// their place.
	follow := msg.FromMe || cv.sticky
	if cv.typingShown {
		// Keep the typing sentinel last: the new row goes in front of it.
		at := len(cv.msgs) - 1
		cv.msgs = append(cv.msgs, client.Message{})
		copy(cv.msgs[at+1:], cv.msgs[at:])
		cv.msgs[at] = msg
		cv.model.Splice(at, 0, msg)
	} else {
		cv.model.Append(msg)
	}
	if follow {
		cv.scrollToBottom()
	}
}

func indexByID(msgs []client.Message) map[string]client.Message {
	byID := make(map[string]client.Message, len(msgs))
	for _, m := range msgs {
		byID[m.ID] = m
	}
	return byID
}

func removeAllChildren(box *gtk.Box) {
	for child := box.FirstChild(); child != nil; {
		w := gtk.BaseWidget(child)
		next := w.NextSibling()
		box.Remove(w)
		child = next
	}
}

// newTypingBubble builds the mockup's 54x26 dotted bubble, left-aligned at
// the foot of the thread.
func newTypingBubble() *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 0)
	row.AddCSSClass("chatot-typing-row")
	row.SetHAlign(gtk.AlignStart)

	bubble := gtk.NewBox(gtk.OrientationHorizontal, 4)
	bubble.AddCSSClass("chatot-typing-bubble")
	for i := 0; i < 3; i++ {
		dot := gtk.NewBox(gtk.OrientationVertical, 0)
		dot.AddCSSClass("chatot-typing-dot")
		dot.AddCSSClass("chatot-typing-dot-" + strconv.Itoa(i+1))
		dot.SetSizeRequest(6, 6)
		dot.SetVAlign(gtk.AlignCenter)
		bubble.Append(dot)
	}
	row.Append(bubble)
	return row
}

// typingSentinelID marks the model row that renders as the typing bubble.
// It never collides with a WhatsApp message id.
const typingSentinelID = "\x00typing"

// threadLen is the number of real messages loaded (the typing sentinel,
// when shown, is the model's last row and not a message).
func (cv *ConversationView) threadLen() int {
	if cv.typingShown {
		return len(cv.msgs) - 1
	}
	return len(cv.msgs)
}

// refreshTypingBubble shows the dotted bubble at the foot of the thread
// while the open chat's peer is composing. Recording counts too: the header
// already says which, and the bubble's job is only to say "something is
// coming". The bubble is a row of the thread (so it sits right under the
// last message and scrolls with it), not a widget below the scroller.
func (cv *ConversationView) refreshTypingBubble() {
	state := cv.presence[cv.jid]
	want := cv.jid != "" && (state.Typing || state.Recording) && cv.scroller.Visible()
	switch {
	case want && !cv.typingShown:
		cv.showTypingRow()
	case !want && cv.typingShown:
		cv.hideTypingRow()
	}
}

func (cv *ConversationView) showTypingRow() {
	follow := cv.sticky
	cv.msgs = append(cv.msgs, client.Message{ID: typingSentinelID, ChatJID: cv.jid})
	cv.model.Append(cv.msgs[len(cv.msgs)-1])
	cv.typingShown = true
	if follow {
		cv.scrollToBottom()
	}
}

func (cv *ConversationView) hideTypingRow() {
	at := len(cv.msgs) - 1
	if at < 0 || cv.msgs[at].ID != typingSentinelID {
		cv.typingShown = false
		return
	}
	cv.msgs = cv.msgs[:at]
	cv.model.Remove(at)
	cv.typingShown = false
}

// composingStaleSecs is how long a composing notice stays up without a
// follow-up: WhatsApp doesn't always send "paused" (a sent message or a
// closed app ends the burst silently), so a stale one is dropped.
const composingStaleSecs = 20

// clearComposing drops jid's typing/recording state: the peer's message
// arrived, or nothing followed the composing notice.
func (cv *ConversationView) clearComposing(jid string) {
	state, ok := cv.presence[jid]
	if !ok || (!state.Typing && !state.Recording) {
		return
	}
	state.Typing, state.Recording = false, false
	cv.presence[jid] = state
	cv.composingGen[jid]++
	if jid == cv.jid {
		cv.refreshHeader()
		cv.refreshTypingBubble()
	}
}

// senderName resolves a group message's sender for the bubble's author
// line: "You" for the account itself, the contact's name, else the bare
// number/identity so the line is never empty.
func (cv *ConversationView) senderName(jid string) string {
	if isOwnJID(jid, cv.c.OwnJID()) {
		return "You"
	}
	jid = nonADJID(jid)
	// mentionName shares the cache and records "" for a user it cannot
	// name; the author line always shows something, so that is a miss here.
	if n, ok := cv.names[jid]; ok && n != "" {
		return n
	}
	name := cv.c.ContactName(jid)
	if name == "" {
		return bareJIDUser(jid)
	}
	cv.names[jid] = name
	return name
}

// mentionName resolves the numeric user part of an @mention (a phone
// number or a LID) to a display name, "" when unknown.
func (cv *ConversationView) mentionName(user string) string {
	own := cv.c.OwnJID()
	for _, jid := range []string{user + "@s.whatsapp.net", user + "@lid"} {
		if isOwnJID(jid, own) {
			return "You"
		}
		if n, ok := cv.names[jid]; ok {
			if n != "" {
				return n
			}
			continue
		}
		n := cv.c.ContactName(jid)
		cv.names[jid] = n
		if n != "" {
			return n
		}
	}
	return ""
}

// buildBubble constructs the GTK widget tree for a single message from its
// pre-computed view-model, wiring the reply/react affordances (if the
// callbacks are set) to msg.
// bubbleAvatarSize is the sender avatar beside a group bubble.
const bubbleAvatarSize = 28

func buildBubble(msg client.Message, vm bubbleView, h bubbleHooks) *gtk.Box {
	c, onVote, searchQuery := h.c, h.onVote, h.searchQuery
	wrapper := gtk.NewBox(gtk.OrientationVertical, 4)

	if vm.ShowDaySeparator {
		sep := gtk.NewLabel(vm.DayText)
		sep.AddCSSClass("chatot-day-separator")
		sep.SetHAlign(gtk.AlignCenter)
		wrapper.Append(sep)
	}
	if vm.ShowUnreadSeparator {
		sep := gtk.NewLabel(vm.UnreadText)
		sep.AddCSSClass("chatot-unread-separator")
		sep.SetHAlign(gtk.AlignCenter)
		wrapper.Append(sep)
	}

	// band spans the pane; row inside it hugs the bubble at one margin.
	band := gtk.NewBox(gtk.OrientationHorizontal, 0)
	band.SetHExpand(true)
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)
	row.SetHExpand(true)
	band.Append(row)
	if vm.FromMe {
		row.SetHAlign(gtk.AlignEnd)
	} else {
		row.SetHAlign(gtk.AlignStart)
	}

	// A sticker or an emoji-only text renders bare — no bubble background or
	// padding, per the mockup — while still keeping the quote/footer/reactions
	// affordances every other bubble gets.
	isSticker := vm.IsMedia && vm.Media.Kind == "sticker"
	noChrome := isSticker || vm.IsEmojiOnly

	bubble := gtk.NewBox(gtk.OrientationVertical, 2)
	if noChrome {
		bubble.AddCSSClass("chatot-bubble-bare")
	} else {
		bubble.AddCSSClass("chatot-bubble")
		if vm.FromMe {
			bubble.AddCSSClass("chatot-bubble-out")
		} else {
			bubble.AddCSSClass("chatot-bubble-in")
		}
	}

	if vm.Author != "" {
		author := gtk.NewLabel(vm.Author)
		author.AddCSSClass("chatot-bubble-author")
		author.SetXAlign(0)
		author.SetEllipsize(pango.EllipsizeEnd)
		author.SetMaxWidthChars(40)
		bubble.Append(author)
	}

	if vm.Forwarded {
		fwd := gtk.NewLabel("↩ Forwarded")
		fwd.AddCSSClass("chatot-forwarded")
		fwd.SetXAlign(0)
		bubble.Append(fwd)
	}

	if vm.HasQuote {
		quote := gtk.NewLabel(resolveMentionsPlain(vm.QuotedText, h.names))
		quote.AddCSSClass("chatot-bubble-quote")
		quote.SetXAlign(0)
		quote.SetWrap(true)
		bubble.Append(quote)
	}

	if vm.IsLocation {
		var stop func()
		if vm.Location.Live && msg.FromMe && h.onStopLive != nil {
			m := msg
			stop = func() { h.onStopLive(m) }
		}
		var open func()
		if h.onOpenViewer != nil {
			m := msg
			open = func() { h.onOpenViewer(m) }
		}
		bubble.Append(buildLocationContent(vm.Location, stop, open))
	} else if vm.IsContact {
		bubble.Append(buildContactContent(vm.Contact))
	} else if vm.IsPoll {
		bubble.Append(buildPollContent(msg, vm.Poll, onVote))
	} else if vm.IsEvent {
		bubble.Append(buildEventContent(vm.Event))
	} else if vm.IsMedia {
		bubble.Append(buildMediaContent(msg, vm.Media, c, h.mediaOpener(msg)))
	} else {
		text := gtk.NewLabel("")
		text.AddCSSClass("chatot-bubble-text")
		if vm.Deleted {
			text.AddCSSClass("chatot-bubble-deleted")
		}
		if vm.IsEmojiOnly {
			text.AddCSSClass("chatot-emoji-only")
		}
		text.SetXAlign(0)
		text.SetWrap(true)
		// WrapWordChar so a long unbroken token (e.g. a URL) still breaks
		// instead of forcing the bubble wider than the pane.
		text.SetWrapMode(pango.WrapWordChar)
		// Cap the natural width so a long paragraph wraps into a hugging bubble
		// (~two-thirds of the pane) instead of stretching edge-to-edge, matching
		// the mockup's bubble sizing.
		text.SetMaxWidthChars(48)
		switch {
		case !vm.Deleted && searchQuery != "" && len(findMatches(vm.Text, searchQuery)) > 0:
			text.SetMarkup(highlightMarkup(vm.Text, searchQuery))
		case !vm.Deleted && hasMention(vm.Text):
			text.SetMarkup(mentionMarkupColor(vm.Text, h.names, vm.FromMe, mentionAccentFor()))
		default:
			text.SetLabel(vm.Text)
		}
		bubble.Append(text)
	}

	footer := gtk.NewBox(gtk.OrientationHorizontal, 4)
	footer.SetHAlign(gtk.AlignEnd)

	timeLabel := gtk.NewLabel(vm.TimeText + vm.EditedMarker)
	timeLabel.AddCSSClass("chatot-bubble-time")
	timeLabel.SetXAlign(1)
	footer.Append(timeLabel)

	if vm.FromMe && vm.TickText != "" {
		tick := gtk.NewLabel(vm.TickText)
		tick.AddCSSClass("chatot-bubble-tick")
		if vm.TickRead {
			tick.AddCSSClass("chatot-tick-read")
		}
		footer.Append(tick)
	}

	bubble.Append(footer)

	// Reactions hang off the bubble's bottom edge as small white pills (the
	// mockup's bottom:-11px), so they overlay the bubble rather than growing
	// it. bubbleStack is what the row actually packs. The row itself gets
	// bottom room for the hanging part (chatot-row-reacted), as WhatsApp does:
	// a pill never sits on top of the next message. The row rather than the
	// stack carries it so the hover icons stay centred on the bubble.
	bubbleStack := gtk.NewOverlay()
	bubbleStack.SetChild(bubble)

	if len(vm.Reactions) > 0 {
		row.AddCSSClass("chatot-row-reacted")
		reactions := gtk.NewBox(gtk.OrientationHorizontal, 4)
		reactions.AddCSSClass("chatot-bubble-reactions")
		// Same side as the hover pill: reactions hug the bubble's outer edge.
		if vm.FromMe {
			reactions.SetHAlign(gtk.AlignEnd)
		} else {
			reactions.SetHAlign(gtk.AlignStart)
		}
		reactions.SetVAlign(gtk.AlignEnd)
		for _, r := range vm.Reactions {
			reactions.Append(newReactionPill(r, msg, h))
		}
		bubbleStack.AddOverlay(reactions)
	}

	// Editing is a text-only, own-message affordance (WhatsApp only edits text);
	// a deleted bubble gets no affordances at all (nothing left to act on).
	canEdit := !vm.Deleted && msg.FromMe && !vm.IsMedia && !vm.IsLocation && !vm.IsContact && !vm.IsPoll && !vm.IsEvent
	// Anyone's message can be deleted for this account (the prompt offers
	// "for everyone" only on own ones); a tombstone has no menu at all.
	canDelete := !vm.Deleted
	// A group's incoming bubble sits beside its sender's avatar (WhatsApp's
	// group thread; the mockup names the sender only).
	if vm.Author != "" && !vm.FromMe && h.avatars != nil {
		avatar := buildAvatar(c, h.avatars, nonADJID(msg.FromJID), initialFor(vm.Author), bubbleAvatarSize)
		avatar.AddCSSClass("chatot-bubble-avatar")
		avatar.SetVAlign(gtk.AlignStart)
		row.Append(avatar)
	}
	if vm.Deleted {
		row.Append(bubbleStack)
	} else {
		// The 🙂 and ⌄ sit together just outside the bubble on the side
		// away from the margin; packing them after an incoming bubble and
		// before an outgoing one means the bubble never moves when they
		// appear. Motion as well as enter: once a popover closes the buttons
		// hide, and the next movement over the row brings them back.
		a := attachBubbleAffordances(bubble, msg, vm, h, canEdit, canDelete)
		if vm.FromMe {
			row.Append(a.actions)
			row.Append(bubbleStack)
		} else {
			row.Append(bubbleStack)
			row.Append(a.actions)
		}
		motion := gtk.NewEventControllerMotion()
		motion.ConnectEnter(func(_, _ float64) { a.setVisible(true) })
		motion.ConnectMotion(func(_, _ float64) { a.setVisible(true) })
		motion.ConnectLeave(func() { a.setVisible(false) })
		// On the band, not the row: the row hugs the bubble, and the user
		// wants the buttons to appear anywhere along the message's line.
		band.AddController(motion)
	}
	wrapper.Append(band)

	return wrapper
}

// copyableText is what "Copy text" puts on the clipboard for msg: the body
// for a text message, the caption for media, and a plain-text rendering for
// the rich kinds (a location's name, address and map link; a contact's name
// and numbers; a poll's question and options). Empty means there is nothing
// worth copying and the row stays inert.
func copyableText(m client.Message) string {
	switch {
	case m.Deleted:
		return ""
	case m.Text != "":
		return m.Text
	case m.Location != nil:
		v := locationVM(m)
		parts := []string{v.Title}
		if v.Address != "" {
			parts = append(parts, v.Address)
		}
		parts = append(parts, v.MapsURL)
		return strings.Join(parts, "\n")
	case m.Contact != nil:
		return strings.Join(append([]string{m.Contact.DisplayName}, m.Contact.Phones...), "\n")
	case m.Poll != nil:
		lines := []string{m.Poll.Name}
		for _, o := range m.Poll.Options {
			lines = append(lines, "• "+o.Name)
		}
		return strings.Join(lines, "\n")
	case m.Attachment != nil:
		return m.Attachment.Caption
	}
	return ""
}

// copyText copies text to the clipboard and confirms with the mockup's plain
// toast. There is no Undo: a clipboard overwrite isn't something the app can
// meaningfully reverse, and the design's toast is a bare notice.
func copyText(overlay *adw.ToastOverlay, text string) {
	gdk.DisplayGetDefault().Clipboard().SetText(text)
	showToast(overlay, "Message copied to clipboard")
}

// showToast shows a short plain toast, a no-op without an overlay (tests,
// or a view built before main.go wires one).
func showToast(overlay *adw.ToastOverlay, text string) {
	if overlay == nil {
		return
	}
	toast := adw.NewToast(text)
	toast.SetTimeout(3)
	overlay.AddToast(toast)
}

// messageInfoRows are the "Message info" card's lines for m: when it was
// sent or received, and for an own message how far it got (the tick state),
// plus the flags the bubble shows. now is injected for deterministic tests.
func messageInfoRows(m client.Message, now time.Time) [][2]string {
	when := time.Unix(m.TS, 0).In(now.Location())
	stamp := when.Format("15:04")
	if !sameDay(m.TS, now.Unix(), now.Location()) {
		stamp = when.Format("02/01/2006 15:04")
	}
	var rows [][2]string
	if m.FromMe {
		rows = append(rows, [2]string{"Sent", stamp})
		delivered, read := "—", "—"
		if m.Status >= client.MessageStatusDelivered {
			delivered = "✓✓"
		}
		if m.Status >= client.MessageStatusRead {
			read = "✓✓"
		}
		rows = append(rows, [2]string{"Delivered", delivered}, [2]string{"Read", read})
	} else {
		rows = append(rows, [2]string{"Received", stamp})
	}
	if m.Starred {
		rows = append(rows, [2]string{"Starred", "Yes"})
	}
	if m.Edited {
		rows = append(rows, [2]string{"Edited", "Yes"})
	}
	if m.Forwarded {
		rows = append(rows, [2]string{"Forwarded", "Yes"})
	}
	return rows
}

// showMessageInfoDialog opens the ⋮ menu's "Message info" card: a settings
// card of label/value rows built by messageInfoRows.
func showMessageInfoDialog(parent *gtk.Window, m client.Message) {
	dialog := newCardDialog()
	dialog.SetTitle("Message info")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetDefaultSize(340, -1)

	body := dialogBody(10)
	card := newSettingsCard()
	for _, r := range messageInfoRows(m, time.Now()) {
		row, _ := newValueRow(r[0], "", r[1], nil)
		card.Add(row)
	}
	body.Append(card)
	dialog.SetChild(body)
	dialog.Present()
}

// bubbleHooks bundles what a bubble's affordances need from the view: the
// client (pin), the window (dialogs), the toast overlay, the search query
// (highlighting) and the per-action callbacks main.go wires.
type bubbleHooks struct {
	c           client.Client
	window      *gtk.Window
	toasts      *adw.ToastOverlay
	searchQuery string
	// host parents the bubble popovers: the conversation's root box, which
	// outlives any list row. A row is rebuilt whenever GtkListView recycles
	// it or a receipt/reaction for its message lands (refreshInPlace), and
	// a popover hung off a widget inside the row would go down with it.
	host gtk.Widgetter
	// ownJID tells a reaction pill whether it is ours (so a click removes it
	// instead of adding a second reaction).
	ownJID string

	onReply    func(client.Message)
	onReact    func(msg client.Message, emoji string)
	onVote     func(msg client.Message, options []string)
	onEdit     func(client.Message)
	onDelete   func(client.Message)
	onStar     func(client.Message)
	onForward  func(client.Message)
	onStopLive func(client.Message)
	// onOpenViewer opens msg's attachment in the viewer pane; nil falls
	// back to the standalone photo/video windows.
	onOpenViewer func(client.Message)
	// onLocalPath records that msgID's attachment now sits at path (the
	// bubble downloaded it), so later reads of the view's messages see it.
	onLocalPath func(msgID, path string)
	// names resolves the numeric user part of an @mention to a display
	// name ("" when unknown); nil leaves mentions as typed.
	names func(user string) string
	// avatars backs the sender avatar beside an incoming group bubble; nil
	// draws none.
	avatars *avatarCache
}

// VoteAt casts a vote for option on the message at idx — a dev/screenshot
// hook.
func (cv *ConversationView) VoteAt(idx int, option string) {
	if m, ok := cv.MessageAt(idx); ok && cv.onVote != nil {
		cv.onVote(m, []string{option})
	}
}

// mediaOpener is the click handler for msg's downloaded picture or video:
// the full-size viewer (forward, save, open; copy for a picture,
// fullscreen for a clip) over the view's window.
func (h bubbleHooks) mediaOpener(msg client.Message) func(path string) {
	return func(path string) {
		// The bubble downloaded the file itself: msg (captured at bind time)
		// and the view's list still say "not downloaded", so the viewer would
		// ask to fetch it again. Carry the path over first.
		if path != "" && msg.Attachment != nil && msg.Attachment.LocalPath != path {
			a := *msg.Attachment
			a.LocalPath = path
			msg.Attachment = &a
			if h.onLocalPath != nil {
				h.onLocalPath(msg.ID, path)
			}
		}
		if h.onOpenViewer != nil {
			h.onOpenViewer(msg)
			return
		}
		if msg.Attachment != nil && msg.Attachment.Kind == "video" {
			showVideoFullscreen(h.window, path, msg, nil)
			return
		}
		showImageViewer(h.window, path, msg, h.onForward)
	}
}

// hooks assembles the bubbleHooks for this view's current state.
func (cv *ConversationView) hooks() bubbleHooks {
	return bubbleHooks{
		c: cv.c, window: cv.window, toasts: cv.toastOverlay, searchQuery: cv.searchQuery, host: cv.Box,
		ownJID:  cv.c.OwnJID(),
		onReply: cv.onReply, onReact: cv.onReact, onVote: cv.onVote, onEdit: cv.onEdit,
		onDelete: cv.onDelete, onStar: cv.onStar, onForward: cv.onForward, onStopLive: cv.onStopLive,
		onOpenViewer: cv.onOpenViewer, onLocalPath: func(id, path string) { cv.setLocalPath(id, path) }, names: cv.mentionName, avatars: cv.avatarCache,
	}
}

// SetLocalPath records that msgID's attachment was downloaded to path (by
// the viewer pane) and rebinds its bubble, so the thread shows the picture
// instead of a download disc without a reload.
func (cv *ConversationView) SetLocalPath(msgID, path string) {
	if cv.setLocalPath(msgID, path) {
		for i := range cv.msgs {
			if cv.msgs[i].ID == msgID {
				cv.refillRow(i)
				return
			}
		}
	}
}

// setLocalPath updates the cached message's attachment path without
// touching the list. It reports whether anything changed.
func (cv *ConversationView) setLocalPath(msgID, path string) bool {
	if path == "" {
		return false
	}
	for i := range cv.msgs {
		m := &cv.msgs[i]
		if m.ID != msgID || m.Attachment == nil || m.Attachment.LocalPath == path {
			continue
		}
		a := *m.Attachment
		a.LocalPath = path
		m.Attachment = &a
		return true
	}
	return false
}

// pinMessage sends the pin and reports the outcome the way the mockup does
// ("Pinned for 7 days").
func (h bubbleHooks) pinMessage(msg client.Message) {
	go func() {
		err := h.c.PinMessage(context.Background(), msg.ChatJID, msg.ID, true)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: pin message failed: %v", err)
				showToast(h.toasts, "Couldn't pin the message")
				return
			}
			showToast(h.toasts, "Pinned for 7 days")
		})
	}()
}

// menuItemsFor is the bubble's ⋮ menu wired to msg. A row whose action this
// bubble can't offer (nothing to copy, someone else's message for Edit) is
// left inert rather than omitted, so the menu keeps the design's shape.
func (h bubbleHooks) menuItemsFor(msg client.Message, canEdit, canDelete bool) []menuItem {
	actions := messageMenuActions{}
	if h.onReply != nil {
		actions.Reply = func() { h.onReply(msg) }
	}
	if h.onForward != nil {
		actions.Forward = func() { h.onForward(msg) }
	}
	if h.onStar != nil {
		actions.Star = func() { h.onStar(msg) }
	}
	if text := copyableText(msg); text != "" {
		actions.Copy = func() { copyText(h.toasts, text) }
	}
	if canEdit && h.onEdit != nil {
		actions.Edit = func() { h.onEdit(msg) }
	}
	if h.c != nil {
		actions.Pin = func() { h.pinMessage(msg) }
	}
	actions.Info = func() { showMessageInfoDialog(h.window, msg) }
	if canDelete && h.onDelete != nil {
		actions.Delete = func() { h.onDelete(msg) }
	}
	items := messageMenuItems(msg, actions)
	if !canEdit || h.onEdit == nil {
		items = withoutMenuItem(items, "Edit message")
	}
	return items
}

// The hover affordances follow WhatsApp's own layout rather than the design's
// floating pill: hovering a bubble reveals a small ⌄ inside its top corner
// and a 🙂 just outside it. The 🙂 opens the quick-reaction row alone; the ⌄
// opens that same row over the full message menu, below the bubble. Both
// are built lazily on click, so binding a row costs no popovers.

// bubbleAffordances is a bubble's hover kit: the 🙂 and ⌄ that appear beside
// it and the actions they trigger (the dev hooks call the openers directly).
// Both buttons sit outside the bubble, WhatsApp's layout at the user's
// request; the 🙂 is always the one nearest it.
type bubbleAffordances struct {
	bubble    *gtk.Box
	host      gtk.Widgetter
	fromMe    bool
	actions   *gtk.Box
	chevron   *gtk.Button
	smiley    *gtk.Button
	openMenu  func()
	openReact func()
	// showing is set while a popover hangs off one of the buttons. Opening
	// a popover moves the pointer onto its own surface, which fires the
	// row's leave; hiding the buttons then would hide the popover's parent
	// and close it on the spot (the click "did nothing"), so leave is
	// ignored until the popover closes.
	showing *bool
}

func attachBubbleAffordances(bubble *gtk.Box, msg client.Message, vm bubbleView, h bubbleHooks, canEdit, canDelete bool) bubbleAffordances {
	actions := gtk.NewBox(gtk.OrientationHorizontal, 2)
	actions.AddCSSClass("chatot-hover-actions")
	actions.SetVAlign(gtk.AlignCenter)
	// Hidden by opacity, not visibility: the buttons keep their room, so a
	// bubble at full width neither shrinks nor re-wraps when they appear.
	actions.SetOpacity(0)
	actions.SetCanTarget(false)

	smiley := gtk.NewButtonWithLabel("🙂")
	smiley.AddCSSClass("chatot-hover-btn")
	smiley.SetTooltipText("React")
	// Pointer affordances: not in the Tab chain, where they would be
	// reached while invisible.
	smiley.SetFocusable(false)

	chevron := gtk.NewButton()
	chevron.SetChild(newChevronGlyph(14))
	chevron.AddCSSClass("chatot-hover-btn")
	chevron.AddCSSClass("chatot-hover-chevron")
	chevron.SetTooltipText("Message options")
	chevron.SetFocusable(false)

	if vm.FromMe {
		actions.Append(chevron)
		actions.Append(smiley)
	} else {
		actions.Append(smiley)
		actions.Append(chevron)
	}

	showing := false
	a := bubbleAffordances{bubble: bubble, host: h.host, fromMe: vm.FromMe, actions: actions, chevron: chevron, smiley: smiley, showing: &showing}
	a.openReact = func() {
		pop := a.popover(smiley, gtk.PosTop, bubbleMenuWidth)
		pop.SetChild(buildReactRow(bubble, msg, h, pop))
		pop.Popup()
	}
	a.openMenu = func() {
		pop := a.popover(chevron, gtk.PosBottom, bubbleMenuWidth)
		// Actions only: the mockup's ⋯ menu carries no reaction row, that is
		// what the 🙂 button beside it is for.
		pop.SetChild(buildMenuBox(h.menuItemsFor(msg, canEdit, canDelete), pop))
		pop.Popup()
	}
	smiley.ConnectClicked(a.openReact)
	chevron.ConnectClicked(a.openMenu)
	if h.onReact == nil {
		smiley.SetSensitive(false)
	}
	shotRegister(msg.ID, func(s *bubbleShot) { s.affordances = a })
	return a
}

// popover hangs a menu card of about width px off one of the hover buttons
// and keeps the buttons on screen until it closes. The card is aligned to
// the anchor's edge nearest the margin (left for an incoming message, right
// for an outgoing one), the mockup's placement, so a card under a bubble at
// the pane's edge grows inward instead of past the window.
func (a bubbleAffordances) popover(anchor gtk.Widgetter, pos gtk.PositionType, width int) *gtk.Popover {
	*a.showing = true
	pop := newBubblePopover(a.host, anchor, alignedRect(a.host, anchor, a.bubble, width, a.fromMe), func() {
		*a.showing = false
		a.setVisible(false)
	})
	pop.SetPosition(pos)
	return pop
}

// setVisible shows or hides the hover buttons; hiding waits while one of
// their popovers is open (see bubbleAffordances.showing).
func (a bubbleAffordances) setVisible(on bool) {
	if !on && *a.showing {
		return
	}
	if on {
		a.actions.SetOpacity(1)
	} else {
		a.actions.SetOpacity(0)
	}
	a.actions.SetCanTarget(on)
}

// newBubblePopover is a menu-card popover for one showing, pointing at
// anchor. It is parented on host (see bubbleHooks.host) and only aimed at
// the anchor, so rebuilding the anchor's row while it is open leaves it
// standing; with no host it falls back to the anchor itself. onClosed runs
// when it closes, after which it unparents itself.
func newBubblePopover(host, anchor gtk.Widgetter, rect *gdk.Rectangle, onClosed func()) *gtk.Popover {
	pop := gtk.NewPopover()
	pop.SetHasArrow(false)
	pop.AddCSSClass("chatot-menu")
	if host == nil {
		pop.SetParent(anchor)
	} else {
		pop.SetParent(host)
		if rect != nil {
			pop.SetPointingTo(rect)
		}
	}
	pop.ConnectClosed(func() {
		onClosed()
		glib.IdleAdd(func() { pop.Unparent() })
	})
	return pop
}

// buildReactRow is the quick-reaction pill: the six fixed reactions and a ＋
// that opens the full picker. Picking closes owner.
func buildReactRow(bubble *gtk.Box, msg client.Message, h bubbleHooks, owner *gtk.Popover) *gtk.Box {
	pill := gtk.NewBox(gtk.OrientationHorizontal, 2)
	pill.AddCSSClass("chatot-react-row")
	pill.SetHAlign(gtk.AlignCenter)
	for _, emoji := range reactEmojis {
		emoji := emoji
		b := gtk.NewButtonWithLabel(emoji)
		b.AddCSSClass("chatot-hover-react")
		b.SetTooltipText("React " + emoji)
		b.ConnectClicked(func() {
			owner.Popdown()
			h.onReact(msg, emoji)
		})
		pill.Append(b)
	}
	plus := gtk.NewButtonWithLabel("＋")
	plus.AddCSSClass("chatot-hover-more-reacts")
	plus.SetTooltipText("More reactions")
	plus.ConnectClicked(func() {
		owner.Popdown()
		openReactionPicker(bubble, msg, h)
	})
	pill.Append(plus)
	return pill
}

// openReactionPicker is the ＋'s full picker: the mockup's 322px "Pick a
// reaction" card with an eight-column grid. It is chatot's own grid rather
// than GtkEmojiChooser, which rebuilt its whole Unicode table on every open
// and lagged for seconds.
func openReactionPicker(bubble *gtk.Box, msg client.Message, h bubbleHooks) {
	pop := newBubblePopover(h.host, bubble, alignedRect(h.host, bubble, nil, reactPickerCardWidth, msg.FromMe), func() {})
	pop.AddCSSClass("chatot-react-picker")

	col := gtk.NewBox(gtk.OrientationVertical, 6)
	caption := gtk.NewLabel("PICK A REACTION")
	caption.SetXAlign(0)
	caption.AddCSSClass("chatot-card-caption")
	col.Append(caption)

	grid := gtk.NewFlowBox()
	grid.SetSelectionMode(gtk.SelectionNone)
	grid.SetMinChildrenPerLine(reactPickerCols)
	grid.SetMaxChildrenPerLine(reactPickerCols)
	grid.SetRowSpacing(2)
	grid.SetColumnSpacing(2)
	grid.SetHomogeneous(true)
	for _, glyph := range reactPickerEmojis {
		emoji := glyph
		b := gtk.NewButtonWithLabel(emoji)
		b.AddCSSClass("flat")
		b.AddCSSClass("chatot-picker-emoji")
		b.SetSizeRequest(-1, pickerEmojiCell)
		b.ConnectClicked(func() {
			pop.Popdown()
			h.onReact(msg, emoji)
		})
		grid.Insert(b, -1)
	}
	grid.SetSizeRequest(reactPickerWidth, -1)
	col.Append(grid)

	pop.SetChild(col)
	pop.Popup()
}

// reactPickerCols/Width are the mockup's picker grid: eight columns in a
// 322px card (306px inside its 8px padding). Its 32 emojis fill exactly four
// rows, so nothing scrolls.
const (
	reactPickerCols  = 8
	reactPickerWidth = 306
)

// reactPickerEmojis is the mockup's "Pick a reaction" palette, in its order:
// the six quick reactions' neighbours first, then faces, gestures and a few
// objects. It is deliberately not the composer's full emoji list — a
// reaction picker is a short menu, not a keyboard.
var reactPickerEmojis = []string{
	"👍", "👎", "❤️", "🔥", "🎉", "😂", "🙂", "😉",
	"😍", "🥰", "😘", "🤩", "😎", "🤔", "😐", "😴",
	"😢", "😭", "😤", "😡", "🙏", "💪", "👏", "✨",
	"🥳", "🤝", "☕", "🎵", "🐦", "🏔️", "📌", "✅",
}

// newChevronGlyph draws the hover button's ⌄ with cairo, in the widget's
// CSS colour. Not the ⌄ character, which fonts draw thin and low in its
// box, and not pan-down-symbolic either: GTK 4.22's own symbolic-SVG
// parser drops the <g> wrapper the icon themes on this system use
// ("Ignoring element in symbolic icon: <g>") and the button came up empty.
func newChevronGlyph(size int) *gtk.DrawingArea {
	area := gtk.NewDrawingArea()
	area.SetSizeRequest(size, size)
	area.SetHAlign(gtk.AlignCenter)
	area.SetVAlign(gtk.AlignCenter)
	area.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		c := area.Color()
		cr.SetSourceRGBA(float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha()))
		cr.SetLineWidth(1.7)
		cr.SetLineCap(cairo.LineCapRound)
		cr.SetLineJoin(cairo.LineJoinRound)
		cx, cy := float64(w)/2, float64(h)/2
		cr.MoveTo(cx-4, cy-2)
		cr.LineTo(cx, cy+2.5)
		cr.LineTo(cx+4, cy-2)
		cr.Stroke()
	})
	return area
}

// Popover card widths the pointing rectangles are sized for: the menu and
// reaction row are .chatot-menu > contents' 230px plus its padding and
// border; the picker is the mockup's 322px card plus the same.
const (
	bubbleMenuWidth      = 244
	reactPickerCardWidth = reactPickerWidth + 14
)

// alignedRect is the rectangle a popover of about width px should point at
// so it hangs off anchor aligned to the bubble's outer edge (see
// bubbleAffordances.popover); nil when nothing is allocated yet, in which
// case the popover falls back to GTK's default placement. A widget shown in
// the same frame (the hover buttons under a dev hook) has no allocation and
// reports 0×0, so fallback (the bubble) is measured instead.
func alignedRect(host, anchor, fallback gtk.Widgetter, width int, fromMe bool) *gdk.Rectangle {
	if host == nil {
		return nil
	}
	for _, w := range []gtk.Widgetter{anchor, fallback} {
		if w == nil {
			continue
		}
		b, ok := gtk.BaseWidget(w).ComputeBounds(host)
		if !ok || b.Width() <= 0 || b.Height() <= 0 {
			continue
		}
		x := alignPopoverX(int(b.X()), int(b.Width()), width, fromMe)
		rect := gdk.NewRectangle(x, int(b.Y()), width, int(b.Height()))
		return &rect
	}
	return nil
}

// alignPopoverX returns the x of a width-wide pointing rectangle that GTK,
// which centres a popover on its rectangle, will place flush with the
// anchor's left edge (incoming) or right edge (outgoing).
func alignPopoverX(anchorX, anchorW, width int, fromMe bool) int {
	if fromMe {
		return anchorX + anchorW - width
	}
	return anchorX
}

// reactionView is one pill under a bubble: the emoji, who sent it, and the
// count the pill shows (the mockup only prints a number past one).
type reactionView struct {
	Emoji    string
	Reactors []string
	Count    int
}

// reactionViews flattens a message's reactions into pills, emoji-sorted so
// the row is stable across rebinds.
func reactionViews(reactions map[string][]string) []reactionView {
	out := make([]reactionView, 0, len(reactions))
	for emoji, who := range reactions {
		if len(who) == 0 {
			continue
		}
		out = append(out, reactionView{Emoji: emoji, Reactors: who, Count: len(who)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Emoji < out[j].Emoji })
	return out
}

// reactionCountText is the pill's trailing number: nothing for a single
// reaction, the count otherwise.
func reactionCountText(count int) string {
	if count <= 1 {
		return ""
	}
	return strconv.Itoa(count)
}

// reactedBy reports whether own is among reactors. WhatsApp hands back
// device-suffixed sender JIDs (user:3@server), so only the user part counts.
func reactedBy(reactors []string, own string) bool {
	ownUser := bareJIDUser(own)
	if ownUser == "" {
		return false
	}
	for _, r := range reactors {
		if bareJIDUser(r) == ownUser {
			return true
		}
	}
	return false
}

// bareJIDUser strips a JID to its user part: the text before '@', minus any
// ':device' suffix.
func bareJIDUser(jid string) string {
	user, _, _ := strings.Cut(jid, "@")
	user, _, _ = strings.Cut(user, ":")
	return user
}

// nonADJID drops the device part of an addressed JID ("user:12@server"):
// a message names the exact device it came from, but names, avatars and
// caches go by the bare identity.
func nonADJID(jid string) string {
	user, server, ok := strings.Cut(jid, "@")
	if !ok {
		return jid
	}
	user, _, _ = strings.Cut(user, ":")
	return user + "@" + server
}

// newReactionPill is one of the mockup's white pills under a bubble: the
// emoji, then a small count once more than one person picked it. Clicking
// it toggles our own reaction — off if the pill is ours, otherwise it
// becomes our reaction (replacing whichever one we had, as WhatsApp allows
// only one per person).
func newReactionPill(r reactionView, msg client.Message, h bubbleHooks) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 3)
	emoji := gtk.NewLabel(r.Emoji)
	emoji.AddCSSClass("chatot-reaction-emoji")
	row.Append(emoji)
	if text := reactionCountText(r.Count); text != "" {
		count := gtk.NewLabel(text)
		count.AddCSSClass("chatot-reaction-count")
		row.Append(count)
	}

	pill := gtk.NewButton()
	pill.SetChild(row)
	pill.AddCSSClass("chatot-reaction-pill")
	pill.SetTooltipText(reactionTooltip(r, h.ownJID))
	mine := reactedBy(r.Reactors, h.ownJID)
	if mine {
		pill.AddCSSClass("chatot-reaction-pill-mine")
	}
	if h.onReact == nil {
		pill.SetSensitive(false)
		return pill
	}
	pick := r.Emoji
	if mine {
		pick = ""
	}
	pill.ConnectClicked(func() { h.onReact(msg, pick) })
	return pill
}

// reactionTooltip names the pill's action, since the pill itself is only an
// emoji and a number.
func reactionTooltip(r reactionView, ownJID string) string {
	if reactedBy(r.Reactors, ownJID) {
		return "Remove your reaction"
	}
	return "React " + r.Emoji
}

// Header is the conversation's AdwHeaderBar (identity, search, ⋮ and the
// window controls). It is built here but packed by the window, above the
// stack that swaps the thread for the media and starred pages, so those
// pages sit under the same header rather than replacing it.
func (cv *ConversationView) Header() gtk.Widgetter { return cv.header }
