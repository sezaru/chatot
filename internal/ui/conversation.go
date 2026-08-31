package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
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
	Reactions        []string
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
}

// starAffordanceVM derives the bubble star-toggle's glyph and tooltip from
// the message's current starred state.
func starAffordanceVM(starred bool) (glyph, tooltip string) {
	if starred {
		return "★", "Unstar"
	}
	return "☆", "Star"
}

// starMenuLabel is the bubble's "⋯" menu star-toggle label for the message's
// current starred state.
func starMenuLabel(starred bool) string {
	if starred {
		return "Unstar"
	}
	return "Star"
}

// undoClipboardValue is what the copy-with-undo toast's Undo button restores
// the clipboard to: the stashed pre-copy text if the async read of it landed
// in time, otherwise "" (best-effort clear — there's nothing to restore).
func undoClipboardValue(stashed string, stashOK bool) string {
	if stashOK {
		return stashed
	}
	return ""
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

	for emoji := range m.Reactions {
		v.Reactions = append(v.Reactions, emoji)
	}
	sort.Strings(v.Reactions)

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

// mediaChip renders a display-only placeholder for a media attachment; the
// actual download/inline rendering is F7.
func mediaChip(a client.Attachment) string {
	label := a.Caption
	if label == "" {
		label = a.Filename
	}
	if label == "" {
		return fmt.Sprintf("[%s]", a.Kind)
	}
	return fmt.Sprintf("[%s] %s", a.Kind, label)
}

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

	header        *adw.HeaderBar
	headerContent *gtk.Box // avatar+title box; hidden (not the whole bar) when no chat is open
	avatarSlot    *gtk.Box
	avatarCache   *avatarCache
	avatarJID     string // jid the avatar widget currently shows, "" until set
	titleLabel    *gtk.Label
	subtitleLabel *gtk.Label
	groupInfoBtn  *gtk.Button // shown only when the open chat is a group
	window        *gtk.Window // parent for the group-info dialog; set via SetWindow
	scroller      *gtk.ScrolledWindow
	listView      *gtk.ListView
	model         *gioutil.ListModel[client.Message]
	empty         *gtk.Label
	emptyBox      *gtk.Box

	// msgs mirrors model 1:1 and is the authoritative slice the bind factory
	// indexes by ListItem position (for the message and its predecessor, which
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

	onReply   func(client.Message)
	onReact   func(msg client.Message, emoji string)
	onVote    func(msg client.Message, options []string)
	onEdit    func(client.Message)
	onDelete  func(client.Message)
	onStar    func(client.Message)
	onForward func(client.Message)

	// onShowMedia/onExportChat/onClearChat are the header "⋮" menu's stubbed
	// seams for F43 (media/links/docs page) and F44 (export/clear chat);
	// onGlobalSearch is the in-chat search bar's "Search all chats" link.
	onShowMedia    func(jid string)
	onExportChat   func(jid string)
	onClearChat    func(jid string)
	onGlobalSearch func(query string)

	menuBtn *gtk.Button

	// In-chat search: searchQuery drives bindRow's highlight check (any
	// currently-bound row whose text contains it, case-insensitively, is
	// rendered with Pango highlight markup); searchHits is the ordered
	// (oldest-first) store match set for the open chat, searchIdx the
	// currently-selected hit ("-1" = none). highlightedPositions tracks which
	// cv.msgs positions were last force-rebound so a narrowing query can
	// un-highlight rows that no longer match.
	searchRevealer       *gtk.Revealer
	searchEntry          *gtk.SearchEntry
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

// OnShowMediaRequested registers f to be called when the user picks "Media,
// links and docs" from the header menu; STUBBED until F43 builds the page.
func (cv *ConversationView) OnShowMediaRequested(f func(jid string)) { cv.onShowMedia = f }

// OnExportRequested registers f to be called when the user picks "Export
// chat…" from the header menu; STUBBED until F44.
func (cv *ConversationView) OnExportRequested(f func(jid string)) { cv.onExportChat = f }

// OnClearRequested registers f to be called when the user picks "Clear
// chat…" from the header menu; STUBBED until F44.
func (cv *ConversationView) OnClearRequested(f func(jid string)) { cv.onClearChat = f }

// OnSearchAllChatsRequested registers f to be called when the user clicks
// "Search all chats" in the in-chat search bar; main.go wires this to the
// sidebar's global search.
func (cv *ConversationView) OnSearchAllChatsRequested(f func(query string)) { cv.onGlobalSearch = f }

// SetWindow supplies the parent window the group-info dialog needs; call
// once after NewConversationView.
func (cv *ConversationView) SetWindow(w *gtk.Window) { cv.window = w }

// SetToastOverlay supplies the overlay the copy-with-undo toast is shown on;
// call once after NewConversationView.
func (cv *ConversationView) SetToastOverlay(overlay *adw.ToastOverlay) { cv.toastOverlay = overlay }

// Messages returns the currently-loaded thread, for mark-read on open.
func (cv *ConversationView) Messages() []client.Message { return cv.msgs }

// CurrentJID returns the chat currently loaded, "" if none.
func (cv *ConversationView) CurrentJID() string { return cv.jid }

// conversationPageSize is how many messages Load fetches up front and each
// scroll-up page adds — small enough that opening a huge chat is instant.
const conversationPageSize = 40

// olderLoadThreshold is how close to the top (in px) the reader must scroll
// before the next older page is fetched.
const olderLoadThreshold = 300

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
	root.SetVExpand(true)
	root.SetHExpand(true)

	// The header is a real AdwHeaderBar so the window's min/max/close controls
	// render here (the app is a content-only AdwApplicationWindow with no
	// separate titlebar). The bar itself stays visible even with no chat open
	// — only its avatar+title content is hidden — so the controls never vanish.
	header := adw.NewHeaderBar()
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
	header.SetShowStartTitleButtons(false)
	// Suppress the AdwHeaderBar's default centered page title ("Conversation");
	// the chat identity lives in headerContent, packed at the start.
	header.SetTitleWidget(gtk.NewLabel(""))
	header.PackStart(headerContent)

	menuBtn := gtk.NewButtonFromIconName("view-more-symbolic")
	menuBtn.AddCSSClass("flat")
	menuBtn.SetTooltipText("Chat options")
	menuBtn.SetSensitive(false)
	header.PackEnd(menuBtn)

	groupInfoBtn := gtk.NewButtonFromIconName("dialog-information-symbolic")
	groupInfoBtn.SetTooltipText("Group info")
	groupInfoBtn.SetVisible(false)
	header.PackEnd(groupInfoBtn)

	root.Append(header)

	searchBar := gtk.NewBox(gtk.OrientationHorizontal, 6)
	searchBar.AddCSSClass("chatot-conv-searchbar")
	searchBar.SetMarginStart(10)
	searchBar.SetMarginEnd(10)
	searchBar.SetMarginBottom(6)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search in chat")
	searchEntry.SetHExpand(true)
	searchBar.Append(searchEntry)

	searchHitLabel := gtk.NewLabel("")
	searchHitLabel.AddCSSClass("chatot-conv-search-count")
	searchBar.Append(searchHitLabel)

	searchUpBtn := gtk.NewButtonFromIconName("go-up-symbolic")
	searchUpBtn.AddCSSClass("flat")
	searchUpBtn.SetTooltipText("Previous match")
	searchBar.Append(searchUpBtn)

	searchDownBtn := gtk.NewButtonFromIconName("go-down-symbolic")
	searchDownBtn.AddCSSClass("flat")
	searchDownBtn.SetTooltipText("Next match")
	searchBar.Append(searchDownBtn)

	searchAllBtn := gtk.NewButtonWithLabel("Search all chats")
	searchAllBtn.AddCSSClass("flat")
	searchBar.Append(searchAllBtn)

	searchCloseBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	searchCloseBtn.AddCSSClass("flat")
	searchCloseBtn.SetTooltipText("Close search")
	searchBar.Append(searchCloseBtn)

	searchRevealer := gtk.NewRevealer()
	searchRevealer.SetChild(searchBar)
	searchRevealer.SetRevealChild(false)
	root.Append(searchRevealer)

	joinBannerBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	joinBannerBox.AddCSSClass("chatot-join-banner")
	joinBannerBox.SetMarginStart(10)
	joinBannerBox.SetMarginEnd(10)
	joinBannerBox.SetMarginTop(6)
	joinBannerBox.SetMarginBottom(6)

	joinBannerLabel := gtk.NewLabel("")
	joinBannerLabel.SetXAlign(0)
	joinBannerLabel.SetHExpand(true)
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
	emptyIcon := gtk.NewImageFromIconName("chat-symbolic")
	emptyIcon.SetPixelSize(64)
	emptyIcon.AddCSSClass("chatot-placeholder")
	emptyBox.Append(emptyIcon)
	empty := gtk.NewLabel("Select a chat")
	empty.AddCSSClass("chatot-placeholder")
	emptyBox.Append(empty)
	root.Append(emptyBox)

	model := conversationModelType.New()

	// GtkListView virtualizes: it only realizes bubble widgets for rows in (or
	// near) the viewport and unrealizes the rest as they scroll off, so the
	// live widget count stays bounded no matter how far back history is paged.
	factory := gtk.NewSignalListItemFactory()
	scroller := gtk.NewScrolledWindow()

	cv := &ConversationView{
		Box:                  root,
		c:                    c,
		events:               c.Events(),
		header:               header,
		headerContent:        headerContent,
		avatarSlot:           avatarSlot,
		avatarCache:          newAvatarCache(),
		titleLabel:           titleLabel,
		subtitleLabel:        subtitleLabel,
		groupInfoBtn:         groupInfoBtn,
		menuBtn:              menuBtn,
		searchRevealer:       searchRevealer,
		searchEntry:          searchEntry,
		searchHitLabel:       searchHitLabel,
		searchIdx:            -1,
		highlightedPositions: make(map[int]bool),
		scroller:             scroller,
		model:                model,
		empty:                empty,
		emptyBox:             emptyBox,
		presence:             make(map[string]PresenceState),
		joinBanner:           joinBanner,
		joinBannerLabel:      joinBannerLabel,
	}

	groupInfoBtn.ConnectClicked(func() {
		if cv.jid != "" {
			showGroupInfoDialog(cv.window, cv.c, cv.jid)
		}
	})

	joinReviewBtn.ConnectClicked(func() {
		if cv.jid != "" {
			showJoinRequestsDialog(cv.window, cv.c, cv.jid, func() { cv.refreshJoinBanner() })
		}
	})

	cv.setupHeaderMenu(menuBtn)

	searchEntry.ConnectSearchChanged(func() {
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
	searchAllBtn.ConnectClicked(func() {
		query := cv.searchQuery
		cv.closeSearchBar()
		if cv.onGlobalSearch != nil {
			cv.onGlobalSearch(query)
		}
	})
	searchCloseBtn.ConnectClicked(func() { cv.closeSearchBar() })

	factory.ConnectSetup(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		row := gtk.NewBox(gtk.OrientationVertical, 0)
		row.SetMarginStart(20)
		row.SetMarginEnd(20)
		item.SetChild(row)
	})
	factory.ConnectBind(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		cv.bindRow(item)
	})

	cv.listView = gtk.NewListView(gtk.NewNoSelection(model), &factory.ListItemFactory)
	cv.listView.AddCSSClass("chatot-conv-list")

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
	scroller.SetChild(cv.listView)
	scroller.SetVisible(false)

	root.Append(scroller)

	scroller.VAdjustment().ConnectValueChanged(cv.onScroll)

	go cv.watchEvents()

	return cv
}

// conversationModelType is the shared typed-model constructor for the
// conversation's message list; ObjectValue recovers a client.Message from a
// row's GObject inside the bind factory.
var conversationModelType = gioutil.NewListModelType[client.Message]()

// bindRow (re)renders one virtualized row: it rebuilds the bubble for the
// message at the item's position into the row's box. Called whenever a row
// scrolls into view or its position shifts (e.g. after an older page is
// prepended), so day separators and reply quotes always reflect the current
// neighbours.
func (cv *ConversationView) bindRow(item *gtk.ListItem) {
	box, ok := item.Child().(*gtk.Box)
	if !ok {
		return
	}
	removeAllChildren(box)

	pos := int(item.Position())
	if pos < 0 || pos >= len(cv.msgs) {
		return
	}
	msg := cv.msgs[pos]
	var prev *client.Message
	if pos > 0 {
		prev = &cv.msgs[pos-1]
	}
	vm := bubbleVM(msg, prev, cv.byID, time.Now())
	box.Append(buildBubble(msg, vm, cv.c, cv.onReply, cv.onReact, cv.onVote, cv.onEdit, cv.onDelete, cv.onStar, cv.onForward, cv.toastOverlay, cv.searchQuery))
}

// onScroll fetches the next older page when the reader nears the top. Runs on
// the GTK main loop (fired by the adjustment). loadingOlder debounces the
// burst of value-changed signals a single scroll gesture emits.
func (cv *ConversationView) onScroll() {
	if cv.loadingOlder || cv.historyInFlight || !cv.hasMore || cv.jid == "" {
		return
	}
	if cv.scroller.VAdjustment().Value() <= olderLoadThreshold {
		cv.loadOlder()
	}
}

// Load fetches the newest page of jid's thread and renders it, replacing
// whatever was shown before; older messages are pulled in on scroll-up (see
// loadOlder). Must run on the GTK main loop.
func (cv *ConversationView) Load(jid string) {
	// A reload of the currently-open chat (receipts/reactions/revokes/poll
	// votes all trigger one) keeps the search bar open; only an actual chat
	// switch resets it.
	if cv.jid != jid {
		cv.closeSearchBar()
	}
	cv.jid = jid
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

	// Replace the whole model with the new page in one splice.
	cv.model.Splice(0, cv.model.Len(), msgs...)

	if len(msgs) == 0 {
		cv.empty.SetLabel("No messages yet")
		cv.emptyBox.SetVisible(true)
		cv.scroller.SetVisible(false)
		return
	}

	cv.emptyBox.SetVisible(false)
	cv.scroller.SetVisible(true)
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
		emojis := make([]string, 0, len(m.Reactions))
		for e := range m.Reactions {
			emojis = append(emojis, e)
		}
		sort.Strings(emojis)
		b.WriteByte('|')
		for _, e := range emojis {
			b.WriteString(e)
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
	n := len(cv.msgs)
	if n < conversationPageSize {
		n = conversationPageSize
	}
	msgs, err := cv.c.Messages(cv.jid, n)
	if err != nil || len(msgs) != len(cv.msgs) {
		cv.Load(cv.jid)
		return
	}
	for i := range msgs {
		if msgs[i].ID != cv.msgs[i].ID {
			cv.Load(cv.jid)
			return
		}
	}
	for i := range msgs {
		if bubbleSig(msgs[i]) == bubbleSig(cv.msgs[i]) {
			continue
		}
		cv.msgs[i] = msgs[i]
		cv.byID[msgs[i].ID] = msgs[i]
		cv.model.Splice(i, 1, msgs[i])
	}
}

// loadOlder prepends the next older page. GtkListView keeps the viewport
// anchored on a model splice-at-front only loosely, so we re-scroll to the
// message that was at the top (now shifted down by len(older)) once the rows
// relayout. Must run on the GTK main loop.
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

	// A genuine older page arrived (from local store or a landed history sync).
	anchor := len(older)
	cv.prependOlder(older)

	glib.IdleAdd(func() {
		cv.listView.ScrollTo(uint(anchor), gtk.ListScrollNone, nil)
		cv.loadingOlder = false
	})
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
	cv.byID = indexByID(cv.msgs)
	cv.hasMore = len(older) == conversationPageSize
	cv.model.Splice(0, 0, older...)
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
				if cp.ChatJID == cv.jid {
					cv.refreshHeader()
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
				if jid == cv.jid {
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
const conversationAvatarSize = 40

// refreshHeader repaints the title/subtitle/avatar for the currently-open
// chat (hides the whole header if none is open). Must run on the GTK main
// loop. The avatar widget is only rebuilt when the open jid changes (tracked
// via avatarJID), so a presence-driven refreshHeader doesn't restart the
// async fetch or cause flicker on every presence update.
func (cv *ConversationView) refreshHeader() {
	if cv.jid == "" {
		cv.headerContent.SetVisible(false)
		cv.groupInfoBtn.SetVisible(false)
		cv.menuBtn.SetSensitive(false)
		return
	}
	name := cv.chatName(cv.jid)
	cv.titleLabel.SetLabel(name)
	cv.subtitleLabel.SetLabel(presenceSubtitle(cv.presence[cv.jid], time.Now()))
	cv.groupInfoBtn.SetVisible(strings.HasSuffix(cv.jid, "@g.us"))
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
	cv.msgs = append(cv.msgs, msg)
	cv.byID[msg.ID] = msg

	if cv.emptyBox.Visible() {
		cv.emptyBox.SetVisible(false)
		cv.scroller.SetVisible(true)
	}

	cv.model.Append(msg)
	cv.scrollToBottom()
}

// scrollToBottom scrolls the list to the newest message. Deferred via
// glib.IdleAdd since the list view hasn't laid out the just-appended row (so
// can't scroll to it) until after this turn of the main loop.
func (cv *ConversationView) scrollToBottom() {
	glib.IdleAdd(func() {
		n := cv.model.Len()
		if n == 0 {
			return
		}
		cv.listView.ScrollTo(uint(n-1), gtk.ListScrollNone, nil)
	})
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

// buildBubble constructs the GTK widget tree for a single message from its
// pre-computed view-model, wiring the reply/react affordances (if the
// callbacks are set) to msg.
func buildBubble(msg client.Message, vm bubbleView, c client.Client, onReply func(client.Message), onReact func(msg client.Message, emoji string), onVote func(msg client.Message, options []string), onEdit func(client.Message), onDelete func(client.Message), onStar func(client.Message), onForward func(client.Message), toastOverlay *adw.ToastOverlay, searchQuery string) *gtk.Box {
	wrapper := gtk.NewBox(gtk.OrientationVertical, 4)

	if vm.ShowDaySeparator {
		sep := gtk.NewLabel(vm.DayText)
		sep.AddCSSClass("chatot-day-separator")
		sep.SetHAlign(gtk.AlignCenter)
		wrapper.Append(sep)
	}

	row := gtk.NewBox(gtk.OrientationHorizontal, 0)
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

	if vm.Forwarded {
		fwd := gtk.NewLabel("↩ Forwarded")
		fwd.AddCSSClass("chatot-forwarded")
		fwd.SetXAlign(0)
		bubble.Append(fwd)
	}

	if vm.HasQuote {
		quote := gtk.NewLabel(vm.QuotedText)
		quote.AddCSSClass("chatot-bubble-quote")
		quote.SetXAlign(0)
		quote.SetWrap(true)
		bubble.Append(quote)
	}

	if vm.IsLocation {
		bubble.Append(buildLocationContent(vm.Location))
	} else if vm.IsContact {
		bubble.Append(buildContactContent(vm.Contact))
	} else if vm.IsPoll {
		bubble.Append(buildPollContent(msg, vm.Poll, onVote))
	} else if vm.IsEvent {
		bubble.Append(buildEventContent(vm.Event))
	} else if vm.IsMedia {
		bubble.Append(buildMediaContent(msg, vm.Media, c))
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
		if !vm.Deleted && searchQuery != "" && len(findMatches(vm.Text, searchQuery)) > 0 {
			text.SetMarkup(highlightMarkup(vm.Text, searchQuery))
		} else {
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

	if len(vm.Reactions) > 0 {
		reactions := gtk.NewBox(gtk.OrientationHorizontal, 2)
		reactions.AddCSSClass("chatot-bubble-reactions")
		for _, emoji := range vm.Reactions {
			r := gtk.NewLabel(emoji)
			reactions.Append(r)
		}
		bubble.Append(reactions)
	}

	// Editing is a text-only, own-message affordance (WhatsApp only edits text);
	// a deleted bubble gets no affordances at all (nothing left to act on).
	canEdit := !vm.Deleted && msg.FromMe && !vm.IsMedia && !vm.IsLocation && !vm.IsContact && !vm.IsPoll && !vm.IsEvent
	canDelete := !vm.Deleted && msg.FromMe
	if !vm.Deleted && (onReply != nil || onReact != nil || (canEdit && onEdit != nil) || (canDelete && onDelete != nil) || onStar != nil || onForward != nil) {
		// Hover-reveal beside the bubble, per the mockup — NOT stacked below it.
		// Placing the actions inside the bubble (below) forced the bubble taller
		// on hover, which reflowed the list and read as flicker. Here they sit in
		// the horizontal row next to the bubble (left of an own message, right of
		// an incoming one) and toggle opacity so the reserved gutter never shifts
		// layout. Popovers parent to the always-present bubble so an open menu
		// survives the pointer leaving the row.
		actions := buildBubbleActions(bubble, msg, vm, onReply, onReact, onEdit, onDelete, onStar, onForward, toastOverlay, canEdit, canDelete)
		actions.SetVAlign(gtk.AlignEnd)
		// Hidden until hover. The row is edge-anchored (start for incoming, end
		// for own) with empty space beside the bubble, so revealing the actions
		// fills that gap without moving the bubble — no reflow, no flicker.
		actions.SetVisible(false)

		motion := gtk.NewEventControllerMotion()
		motion.ConnectEnter(func(_, _ float64) { actions.SetVisible(true) })
		motion.ConnectLeave(func() { actions.SetVisible(false) })
		row.AddController(motion)

		if vm.FromMe {
			row.Append(actions)
			row.Append(bubble)
		} else {
			row.Append(bubble)
			row.Append(actions)
		}
	} else {
		row.Append(bubble)
	}
	wrapper.Append(row)

	return wrapper
}

// openEmojiChooser pops a native emoji picker parented to parent, routing
// the chosen emoji to onReact for msg. Shared by the reaction quick-row's
// "+" button and the "⋯" menu's "React…" item so there's one codepath.
func openEmojiChooser(parent gtk.Widgetter, msg client.Message, onReact func(msg client.Message, emoji string)) {
	chooser := gtk.NewEmojiChooser()
	chooser.SetParent(parent)
	chooser.ConnectClosed(func() { chooser.Unparent() })
	chooser.ConnectEmojiPicked(func(text string) {
		onReact(msg, text)
	})
	chooser.Popup()
}

// copyTextWithUndo copies text to the clipboard and, if overlay is set,
// shows a toast offering to undo it. It stashes the clipboard's current
// contents via an async read started before the overwrite; if that read
// hasn't landed by the time Undo is clicked, Undo just clears the clipboard
// instead of restoring (see undoClipboardValue).
func copyTextWithUndo(overlay *adw.ToastOverlay, text string) {
	clipboard := gdk.DisplayGetDefault().Clipboard()

	var stashed string
	var stashOK bool
	clipboard.ReadTextAsync(context.Background(), func(res gio.AsyncResulter) {
		if prev, err := clipboard.ReadTextFinish(res); err == nil {
			stashed, stashOK = prev, true
		}
	})

	clipboard.SetText(text)

	if overlay == nil {
		return
	}
	toast := adw.NewToast("Message copied to clipboard")
	toast.SetButtonLabel("Undo")
	toast.SetTimeout(4)
	toast.ConnectButtonClicked(func() {
		clipboard.SetText(undoClipboardValue(stashed, stashOK))
	})
	overlay.AddToast(toast)
}

// buildBubbleActions builds the hover affordances shown beside a non-deleted
// bubble, laid out per the mockup: a horizontal quick-react pill
// (👍❤️😂😮😢🙏 ＋) on top, and a row of three icon buttons below — ↩ reply,
// 🙂 react (full picker), ⋯ more (reply/forward/copy/star/edit/delete). Edit is
// own-message-only and lives in the ⋯ menu; star applies to any message.
func buildBubbleActions(parent gtk.Widgetter, msg client.Message, vm bubbleView, onReply func(client.Message), onReact func(msg client.Message, emoji string), onEdit func(client.Message), onDelete func(client.Message), onStar func(client.Message), onForward func(client.Message), toastOverlay *adw.ToastOverlay, canEdit, canDelete bool) *gtk.Box {
	icons := gtk.NewBox(gtk.OrientationHorizontal, 2)
	icons.AddCSSClass("chatot-bubble-actions")

	iconBtn := func(label string) *gtk.Button {
		b := gtk.NewButtonWithLabel(label)
		b.AddCSSClass("flat")
		b.AddCSSClass("chatot-bubble-iconbtn")
		return b
	}

	if onReply != nil {
		reply := iconBtn("↩")
		reply.SetTooltipText("Reply")
		reply.ConnectClicked(func() { onReply(msg) })
		icons.Append(reply)
	}
	if onReact != nil {
		// The 🙂 button opens the quick-react pill on click (NOT on hover); the
		// pill's ＋ opens the full emoji picker. Parented to the bubble so it
		// survives the pointer leaving the (hover-hidden) action row.
		react := iconBtn("🙂")
		react.SetTooltipText("React")

		reactPop := gtk.NewPopover()
		pill := gtk.NewBox(gtk.OrientationHorizontal, 1)
		pill.AddCSSClass("chatot-react-pill")
		for _, emoji := range reactEmojis {
			b := gtk.NewButtonWithLabel(emoji)
			b.AddCSSClass("flat")
			b.AddCSSClass("chatot-react-emoji")
			b.ConnectClicked(func() {
				onReact(msg, emoji)
				reactPop.Popdown()
			})
			pill.Append(b)
		}
		plus := gtk.NewButtonWithLabel("＋")
		plus.AddCSSClass("flat")
		plus.AddCSSClass("chatot-react-emoji")
		plus.ConnectClicked(func() {
			reactPop.Popdown()
			openEmojiChooser(parent, msg, onReact)
		})
		pill.Append(plus)
		reactPop.SetChild(pill)
		reactPop.SetParent(parent)
		react.ConnectClicked(func() { reactPop.Popup() })
		icons.Append(react)
	}

	if onReply != nil || onForward != nil || onReact != nil || onStar != nil || (canEdit && onEdit != nil) || (canDelete && onDelete != nil) {
		moreMenuBtn := iconBtn("⋯")
		moreMenuBtn.SetTooltipText("More")

		popover := gtk.NewPopover()
		menu := gtk.NewBox(gtk.OrientationVertical, 0)

		addItem := func(label string, destructive bool, onClick func()) {
			btn := gtk.NewButtonWithLabel(label)
			btn.AddCSSClass("flat")
			if destructive {
				btn.AddCSSClass("destructive-action")
			}
			btn.ConnectClicked(func() {
				popover.Popdown()
				onClick()
			})
			menu.Append(btn)
		}

		if onReply != nil {
			addItem("Reply", false, func() { onReply(msg) })
		}
		if onForward != nil {
			addItem("Forward", false, func() { onForward(msg) })
		}
		if msg.Text != "" {
			addItem("Copy text", false, func() { copyTextWithUndo(toastOverlay, msg.Text) })
		}
		if canEdit && onEdit != nil {
			addItem("Edit", false, func() { onEdit(msg) })
		}
		if onStar != nil {
			addItem(starMenuLabel(msg.Starred), false, func() { onStar(msg) })
		}
		if canDelete && onDelete != nil {
			addItem("Delete for me", true, func() { onDelete(msg) })
		}

		popover.SetChild(menu)
		popover.ConnectClosed(func() { popover.Unparent() })
		moreMenuBtn.ConnectClicked(func() {
			popover.SetParent(parent)
			popover.Popup()
		})
		icons.Append(moreMenuBtn)
	}

	return icons
}
