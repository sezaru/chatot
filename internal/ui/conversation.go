package ui

import (
	"context"
	"errors"
	"fmt"
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
	IsCall           bool
	Call             callView
	// Choices are the buttons a business message offers under its text,
	// nil for none.
	Choices *choicesView
	// Link is the card a text message shows for a link in it, nil for none.
	Link         *linkView
	Edited       bool
	EditedMarker string
	Deleted      bool
	// Forwarded marks a message that carried WhatsApp's forwarded flag; drives
	// the "↩ Forwarded" label above the bubble content.
	Forwarded bool
	// TickText is the own-message delivery/read indicator ("✓"/"✓✓"), set
	// only when FromMe; TickRead marks it should render in the accent color
	// (status == read) rather than the plain dim tick.
	TickText string
	TickRead bool
	// Pending marks an own message whose send is still in flight (a clock
	// where the tick goes); Failed one whose send errored (a red badge and
	// a Retry button). Neither is in the store yet, so the bubble carries
	// no affordances: there is no server-side message to act on.
	Pending bool
	Failed  bool
	// CaptionText is the text under a picture, video or document: the
	// attachment's caption, "" when it has none. Stickers and voice notes
	// never carry one.
	CaptionText string
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
			// A picture, voice note or poll has no Text: quote its kind
			// label ("📷 Photo") the way the chat list previews it.
			v.QuotedText = messageSnippet(q)
		} else {
			// Not among the loaded rows: the preview the reply carried,
			// or the one fillQuote read off the store.
			v.QuotedText = m.ReplyTo.Text
		}
		if v.QuotedText == "" {
			v.QuotedText = "↩ reply"
		}
	}

	v.Reactions = reactionViews(m.Reactions)

	if m.Edited {
		v.Edited = true
		v.EditedMarker = " · edited"
	}
	if m.LinkPreview != nil {
		v.Link = linkVM(m.LinkPreview)
	}

	if m.FromMe {
		v.TickText, v.TickRead = tickVM(m.Status)
		v.Pending = m.Status == client.MessageStatusPending
		v.Failed = m.Status == client.MessageStatusFailed
	}

	v.Choices = choicesVM(m)
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
	case m.CallLog != nil:
		v.IsCall = true
		v.Call = callVM(m)
	case m.Attachment != nil:
		v.IsMedia = true
		v.Media = mediaVM(m)
		v.MediaChip = v.Media.Chip
		v.CaptionText = captionText(*m.Attachment)
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
	if status < client.MessageStatusSent {
		// Not (yet) sent: the pending clock or failed badge stands in.
		return "", false
	}
	if status >= client.MessageStatusDelivered {
		return "✓✓", status >= client.MessageStatusRead
	}
	return "✓", false
}

// captionText is what a media bubble prints under its picture, video or
// document: the caption the sender typed, trimmed. Stickers and audio have
// no caption field on WhatsApp, and a document's filename is its row
// title, not a caption.
func captionText(a client.Attachment) string {
	switch a.Kind {
	case "sticker", "audio":
		return ""
	case "document":
		// A nameless document's caption already serves as its row title.
		if a.Filename == "" {
			return ""
		}
	}
	return strings.TrimSpace(a.Caption)
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

	header *gtk.WindowHandle
	// backBtn and startControlsSlot show only while the split view is
	// collapsed: ← returns to the chat list, and the slot draws the start
	// side of the window controls that the hidden sidebar header would.
	backBtn           *gtk.Button
	startControlsSlot *gtk.Box
	onBack            func()
	headerContent     *gtk.Box // avatar+title box; hidden (not the whole bar) when no chat is open
	avatarSlot        *gtk.Box
	avatarCache       *avatarCache
	avatarJID         string // jid the avatar widget currently shows, "" until set
	titleLabel        *gtk.Label
	subtitleLabel     *gtk.Label

	headerMenuPop *gtk.Popover // ⋮ header menu; dev-hook popup target
	window        *gtk.Window  // parent for the group-info dialog; set via SetWindow
	scroller      *gtk.ScrolledWindow
	// threadOverlay wraps the scroller and floats jumpBtn over it.
	threadOverlay *gtk.Overlay
	jumpBtn       *gtk.Button
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
	// glide runs the thread's own flings (thread_input.go).
	glide    *glider
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
	// loadGen bumps on every Load: a page or a refresh the store hands
	// back for an earlier generation belongs to a thread no longer shown
	// and is dropped.
	loadGen int
	// refreshInFlight and refreshDirty coalesce refreshInPlace: one store
	// read at a time, and a request made while one is out runs once more
	// after it lands. refreshRetried marks the one re-read allowed when a
	// page does not line up with the loaded rows.
	refreshInFlight bool
	refreshDirty    bool
	refreshRetried  bool

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
	// composing is who is typing or recording in each chat; composingGen
	// counts the events per chat and sender (see composingKey) so the
	// stale-typing timer only ends the burst it was armed for.
	composing    composers
	composingGen map[string]int
	// quotes caches the preview of a quoted message that is not among the
	// loaded rows (paged out, or older than the store), by message id.
	quotes map[string]string
	// chatIsGroup caches whether the open chat is a group (sender names).
	chatIsGroup bool
	// chatInfo is the open chat's row as of the last Load: the chat list
	// query is one pass over every chat, too much to repeat for one name.
	chatInfo client.Chat
	// unreadAnchor is the id of the first message the reader hasn't seen
	// in the open chat ("" for none): the "N unread messages" pill sits
	// above it until the chat is left.
	unreadAnchor string
	// anchorRow is the recycled row currently showing the unread pill, so the
	// pill can be seen to be on screen; seenTimer counts down its stay once
	// it is. Each row knows which message it holds (threadRow.msgID).
	anchorRow *threadRow
	// rows is the persistent chrome for each row GtkListView recycles, keyed
	// by the wrapper's GObject (see widgetKey).
	rows map[uintptr]*threadRow
	// compactRows floats each row's hover pair over its bubble (the
	// collapsed window); see hoverButtons.place.
	compactRows bool
	// avatarGens counts each sender's picture changes, so a row that keeps a
	// cached avatar across binds still notices a new one.
	avatarGens map[string]int
	seenTimer  glib.SourceHandle
	// names memoizes ContactName lookups for senders and mentions; it is
	// dropped on EventChatUpdate, which the contact sync fires once new
	// names land.
	names map[string]string
	// thumbTried is every message whose high-quality preview this view
	// already asked for, so a rebound row does not ask again.
	thumbTried map[string]bool
	// transcripts is each voice note's transcription state this session
	// (a run in progress, its failure, whether the text is unfolded); the
	// text itself lives on the message's attachment. Nil until first used.
	transcripts map[string]transcriptState

	// expandedText holds the long bodies the reader unfolded this session
	// (the bubble's "Read more"), so a rebuilt row keeps them open. Nil
	// until first used.
	expandedText map[string]bool

	// unsent holds, per chat, the optimistic rows the store does not have:
	// sends in flight and sends that failed. Load appends them after the
	// stored page and refreshInPlace leaves them out of its comparison
	// with the store, so a receipt landing mid-send never drops them.
	unsent map[string][]client.Message

	onReply   func(client.Message)
	onRetry   func(client.Message)
	onReact   func(msg client.Message, emoji string)
	onVote    func(msg client.Message, options []string)
	onChoice  func(msg client.Message, sel client.ChoiceSelection)
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
	// flashID is the message whose row is being flashed after a jump from
	// a reply quote, so fillRow can tag it; cleared when the flash ends.
	flashID string
	// laidOut reports whether the thread has had a layout since Load
	// (its scrollable height changed); pendingJump is the message a
	// JumpTo asked for before that, run on the first one. See JumpTo.
	laidOut     bool
	pendingJump string

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

// OnRetryRequested registers f to be called with a failed optimistic
// message when the user presses its Retry; the row is gone by then, and f
// is expected to send the content afresh (the composer's Resend).
func (cv *ConversationView) OnRetryRequested(f func(client.Message)) { cv.onRetry = f }

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

// OnChoiceRequested registers f to be called when the user taps a reply
// button or picks a list row under a business message; sel is the pick.
func (cv *ConversationView) OnChoiceRequested(f func(msg client.Message, sel client.ChoiceSelection)) {
	cv.onChoice = f
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
	// Both lines give way to the pane: a long group name ends in an
	// ellipsis (the mockup's text-overflow) rather than holding the
	// collapsed window open past its 360px.
	titleLabel.SetEllipsize(pango.EllipsizeEnd)
	textCol.Append(titleLabel)

	subtitleLabel := gtk.NewLabel("")
	subtitleLabel.SetXAlign(0)
	subtitleLabel.SetEllipsize(pango.EllipsizeEnd)
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
	// Collapsed (one pane at a time, below the window's breakpoint) the
	// sidebar header is off screen, so its start-side window controls and
	// the mockup's ← back to the list move in here; both stay hidden while
	// the panes sit side by side.
	startControlsSlot := gtk.NewBox(gtk.OrientationHorizontal, 0)
	startControlsSlot.Append(newWindowControls(gtk.PackStart))
	startControlsSlot.SetVisible(false)
	headerRow.Append(startControlsSlot)
	backBtn := newPaneBackButton("Back to the list", nil)
	backBtn.SetVisible(false)
	headerRow.Append(backBtn)
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
		Box:               root,
		c:                 c,
		events:            c.Events(),
		rows:              map[uintptr]*threadRow{},
		avatarGens:        map[string]int{},
		unsent:            map[string][]client.Message{},
		header:            header,
		backBtn:           backBtn,
		startControlsSlot: startControlsSlot,
		headerContent:     headerContent,
		avatarSlot:        avatarSlot,
		avatarCache:       newAvatarCache(),
		titleLabel:        titleLabel,
		subtitleLabel:     subtitleLabel,

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
		composing:            make(composers),
		composingGen:         make(map[string]int),
		quotes:               make(map[string]string),
		names:                make(map[string]string),
		timers:               make(map[string]int64),
		joinBanner:           joinBanner,
		joinBannerLabel:      joinBannerLabel,
	}
	// ← steps back one level at a time: out of the search while it is
	// open, and only then out of the chat to the list.
	cv.backBtn.ConnectClicked(func() {
		if cv.headerStack.VisibleChildName() == "search" {
			cv.closeSearchBar()
			return
		}
		fire(cv.onBack)
	})

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

	// The thread sits under a floating "back to the newest message"
	// button at the pane's bottom-right, shown only while the reader has
	// scrolled up (see updateJumpButton).
	cv.threadOverlay = gtk.NewOverlay()
	// The thread is the join-request strip over the scroller. The chat
	// wallpaper (see wallpaper.go) paints on this box, behind both; the
	// empty state is a sibling of the overlay, so it stays plain.
	threadBox := gtk.NewBox(gtk.OrientationVertical, 0)
	threadBox.AddCSSClass("chatot-conv-thread")
	threadBox.Append(joinBanner)
	threadBox.Append(scroller)
	cv.threadOverlay.SetChild(threadBox)
	cv.threadOverlay.SetVExpand(true)
	cv.threadOverlay.SetVisible(false)
	cv.jumpBtn = gtk.NewButton()
	cv.jumpBtn.SetChild(newChevronGlyph(18))
	cv.jumpBtn.AddCSSClass("chatot-jump-bottom")
	cv.jumpBtn.SetTooltipText("Scroll to bottom")
	cv.jumpBtn.SetFocusable(false)
	cv.jumpBtn.SetHAlign(gtk.AlignEnd)
	cv.jumpBtn.SetVAlign(gtk.AlignEnd)
	cv.jumpBtn.SetMarginEnd(16)
	cv.jumpBtn.SetMarginBottom(14)
	cv.jumpBtn.SetVisible(false)
	cv.jumpBtn.ConnectClicked(cv.scrollToBottom)
	cv.threadOverlay.AddOverlay(cv.jumpBtn)
	root.Append(cv.threadOverlay)

	adj := scroller.VAdjustment()
	adj.ConnectValueChanged(cv.onScroll)
	adj.NotifyProperty("upper", cv.onUpperChanged)
	adj.NotifyProperty("page-size", cv.onUpperChanged)
	cv.installScrollInput()

	go cv.watchEvents()

	return cv
}

// fillRow shows the message at pos in its row. The row's chrome is reused
// (thread_row.go); only the bubble's interior is rebuilt. Called when a row is
// bound or its message (or its predecessor, for day separators and reply
// quotes) changed.
func (cv *ConversationView) fillRow(box *gtk.Box, pos int) {
	defer perfStart("fillRow")()
	trace(2, "fill row %d of %d", pos, len(cv.msgs))
	r := cv.rows[widgetKey(box)]
	if r == nil || pos < 0 || pos >= len(cv.msgs) {
		return
	}
	msg := cv.msgs[pos]
	if msg.ID == typingSentinelID {
		r.msgID = msg.ID
		r.renderTyping()
		return
	}
	var prev *client.Message
	if pos > 0 {
		prev = &cv.msgs[pos-1]
	}
	cv.fillQuote(&msg)
	vm := bubbleVM(msg, prev, cv.byID, time.Now())
	if cv.chatIsGroup && !msg.FromMe {
		vm.Author = cv.senderName(msg.FromJID)
	}
	r.msgID = msg.ID
	box.RemoveCSSClass("chatot-row-flash")
	if cv.flashID != "" && msg.ID == cv.flashID {
		box.AddCSSClass("chatot-row-flash")
	}
	if cv.unreadAnchor != "" && msg.ID == cv.unreadAnchor {
		vm.ShowUnreadSeparator = true
		vm.UnreadText = unreadSeparatorText(cv.threadLen() - pos)
		cv.anchorRow = r
		glib.IdleAdd(cv.scheduleUnreadClear)
	}
	r.render(msg, vm, cv.hooks())
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
	if row == nil || row.msgID != cv.unreadAnchor {
		return false
	}
	b, ok := row.wrapper.ComputeBounds(cv.scroller)
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
	cv.pendingJump = ""
	cv.laidOut = false
	setWallpaperChat(jid)
	chat := chatByJID(cv.c, jid)
	cv.chatInfo = chat
	cv.chatIsGroup = chat.IsGroup
	cv.loadingOlder = false
	cv.historyRequested = false
	cv.historyInFlight = false
	cv.loadGen++
	cv.refreshDirty = false
	cv.refreshRetried = false
	cv.refreshHeader()
	cv.refreshJoinBanner()

	msgs, err := cv.c.Messages(jid, conversationPageSize)
	if err != nil {
		msgs = nil
	}
	cv.hasMore = len(msgs) == conversationPageSize
	if len(msgs) > 0 {
		cv.oldestID = msgs[0].ID
	} else {
		cv.oldestID = ""
	}
	if cv.unreadAnchor == "" {
		cv.unreadAnchor = unreadAnchorFor(msgs, chat.UnreadCount)
	}
	// The chat's sends in flight and failed sends live only here; they
	// go after the stored page, where they were appended.
	msgs = cv.withUnsent(jid, msgs)
	cv.msgs = msgs
	cv.byID = indexByID(msgs)

	// Replace every row with the new page; the typing sentinel (if any)
	// goes with it and is re-added below.
	cv.typingShown = false
	cv.model.Splice(0, cv.model.Len(), msgs...)

	if len(msgs) == 0 {
		cv.empty.SetLabel("No messages yet")
		cv.emptyBox.SetVisible(true)
		cv.setThreadVisible(false)
		return
	}

	cv.emptyBox.SetVisible(false)
	cv.setThreadVisible(true)
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
	if m.Played {
		// A listened-to voice note recolours its row.
		b.WriteByte('P')
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
	if m.CallLog != nil {
		// An accept turns a missed call into an answered one in place.
		b.WriteByte('|')
		b.WriteString(m.CallLog.Outcome)
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
	// One read at a time: the store is read off the main loop, and every
	// request that arrives meanwhile is served by one more read after.
	if cv.refreshInFlight {
		cv.refreshDirty = true
		return
	}
	cv.refreshInFlight = true
	// Only the stored rows are compared with the store: an optimistic
	// row (a send in flight, or a failed one) has no counterpart there.
	n := len(cv.storedPositions())
	if n < conversationPageSize {
		n = conversationPageSize
	}
	jid, gen := cv.jid, cv.loadGen
	go func() {
		msgs, err := cv.c.Messages(jid, n)
		glib.IdleAdd(func() {
			cv.refreshInFlight = false
			if gen != cv.loadGen || jid != cv.jid {
				cv.refreshDirty = false
				return
			}
			cv.applyRefresh(msgs, err)
			if cv.refreshDirty {
				cv.refreshDirty = false
				cv.refreshInPlace()
			}
		})
	}()
}

// applyReceipt advances the status of the loaded messages r names and
// re-renders their rows, the way the store does (a status never goes
// back), without reading the store: on a busy group every member's
// delivery and read receipt is one such event. It reports false for a
// receipt that carries no status, which the caller resolves from the
// store instead.
func (cv *ConversationView) applyReceipt(r client.Receipt) bool {
	if r.Status == 0 {
		return false
	}
	for _, id := range r.MsgIDs {
		pos := cv.positionOf(id)
		if pos < 0 || cv.msgs[pos].Status >= r.Status {
			continue
		}
		cv.msgs[pos].Status = r.Status
		cv.byID[id] = cv.msgs[pos]
		cv.refillRow(pos)
	}
	return true
}

// applyRefresh patches the loaded rows from msgs, the store's newest page
// as long as the thread was when the read started. A page that no longer
// lines up with the rows is read once more before the thread is reloaded
// outright: a message appended while the read was out is the usual
// reason, a genuine add or remove the rare one.
func (cv *ConversationView) applyRefresh(msgs []client.Message, err error) {
	stored := cv.storedPositions()
	aligned := err == nil && len(msgs) == len(stored)
	for k := 0; aligned && k < len(stored); k++ {
		aligned = msgs[k].ID == cv.msgs[stored[k]].ID
	}
	if !aligned {
		if cv.refreshRetried {
			cv.refreshRetried = false
			cv.Load(cv.jid)
			return
		}
		cv.refreshRetried = true
		cv.refreshDirty = true
		return
	}
	cv.refreshRetried = false
	changed := 0
	for k, i := range stored {
		if bubbleSig(msgs[k]) == bubbleSig(cv.msgs[i]) {
			continue
		}
		cv.msgs[i] = msgs[k]
		cv.byID[msgs[k].ID] = msgs[k]
		cv.refillRow(i)
		changed++
	}
	trace(1, "refreshInPlace: %d rows changed", changed)
}

// storedPositions lists the positions in cv.msgs of the rows the store
// holds: everything but the typing sentinel and the optimistic rows.
func (cv *ConversationView) storedPositions() []int {
	out := make([]int, 0, len(cv.msgs))
	for i, m := range cv.msgs {
		if m.ID == typingSentinelID || isUnsent(m) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// loadOlder fetches the next older page off the main loop and prepends it
// once it lands; a page read for a thread no longer open is dropped. The
// store read used to run on the main loop, and on a long thread it was the
// frame that stalled at every page boundary of a fling. Must run on the
// GTK main loop.
func (cv *ConversationView) loadOlder() {
	cv.loadingOlder = true
	jid, oldestID, gen := cv.jid, cv.oldestID, cv.loadGen
	go func() {
		older, err := cv.c.MessagesBefore(jid, oldestID, conversationPageSize)
		glib.IdleAdd(func() {
			if gen != cv.loadGen || jid != cv.jid {
				return
			}
			cv.loadingOlder = false
			cv.olderPageLanded(older, err)
		})
	}()
}

// olderPageLanded applies what loadOlder read: an error ends paging, an
// empty page asks the phone for more history (once), a real page is
// prepended.
func (cv *ConversationView) olderPageLanded(older []client.Message, err error) {
	if err != nil {
		cv.hasMore = false
		return
	}
	if len(older) == 0 {
		request, exhausted := nextHistoryAction(len(older), cv.historyRequested)
		trace(1, "loadOlder: store floor reached; request=%v exhausted=%v", request, exhausted)
		cv.hasMore = !exhausted
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
}

// prependOlder splices older (oldest-first) onto the front of the currently
// loaded thread, updating the pagination cursor/index. Shared by loadOlder
// (scroll-up paging) and jumpToMessage (loading pages synchronously to reach
// a search hit that isn't loaded yet). historyRequested is cleared so hitting
// the store's floor again re-requests the next batch from the phone, one
// page at a time rather than stopping after one. Must run on the GTK main
// loop.
func (cv *ConversationView) prependOlder(older []client.Message) {
	defer perfStart("prependOlder")()
	cv.historyRequested = false
	cv.msgs = append(older, cv.msgs...)
	cv.oldestID = cv.msgs[0].ID
	stopIdx := perfStart("indexByID")
	cv.byID = indexByID(cv.msgs[:cv.threadLen()])
	stopIdx()
	cv.hasMore = len(older) == conversationPageSize
	stopSplice := perfStart("modelSplice")
	cv.model.Splice(0, 0, older...)
	stopSplice()
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
			r := *ev.Receipt
			glib.IdleAdd(func() {
				if r.ChatJID != cv.jid {
					return
				}
				if !cv.applyReceipt(r) {
					cv.refreshInPlace()
				}
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
			chatJID, msgID := ev.Revoke.ChatJID, ev.Revoke.MsgID
			glib.IdleAdd(func() {
				cv.applyRevoke(chatJID, msgID)
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
				kind := composingKind(cp.State, cp.Media)
				cv.composing.set(cp.ChatJID, cp.JID, kind)
				key := composingKey(cp.ChatJID, cp.JID)
				cv.composingGen[key]++
				if kind != "" {
					gen := cv.composingGen[key]
					chat, sender := cp.ChatJID, cp.JID
					glib.TimeoutSecondsAdd(composingStaleSecs, func() bool {
						if cv.composingGen[key] == gen {
							cv.composing.set(chat, sender, "")
							cv.syncComposing(chat)
						}
						return false
					})
				}
				cv.syncComposing(cp.ChatJID)
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
				// Bump before repainting: the rows read the generation to
				// decide their cached picture is stale.
				cv.avatarGens[nonADJID(jid)]++
				cv.refreshSenderAvatars(jid)
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
	p := cv.presence[cv.jid]
	cv.subtitleLabel.SetLabel(presenceSubtitle(p, time.Now()))
	// Typing and recording read the way they do in the chat list, rather
	// than as one more grey line beside "online" and "last seen".
	setCSSClass(cv.subtitleLabel, "chatot-conv-typing", p.Typing || p.Recording)

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
	if isUnsent(msg) {
		cv.unsent[msg.ChatJID] = append(cv.unsent[msg.ChatJID], msg)
	}
	if msg.ChatJID != cv.jid {
		return
	}
	cv.appendMessage(msg)
}

// isUnsent reports whether msg is an optimistic row the store does not
// hold: a send in flight or a failed one.
func isUnsent(msg client.Message) bool { return msg.Status < client.MessageStatusSent }

// ResolveSent settles the optimistic row localID once its send returns:
// on success the row becomes the stored message (real id, sent status), on
// failure it turns into the failed bubble with its Retry. Must run on the
// GTK main loop.
func (cv *ConversationView) ResolveSent(localID string, msg client.Message, err error) {
	if err != nil {
		msg.Status = client.MessageStatusFailed
		msg.ID = localID
	} else {
		msg.Status = client.MessageStatusSent
	}
	rows := cv.unsent[msg.ChatJID]
	for i := range rows {
		if rows[i].ID != localID {
			continue
		}
		if isUnsent(msg) {
			rows[i] = msg
		} else {
			cv.unsent[msg.ChatJID] = append(rows[:i], rows[i+1:]...)
		}
		break
	}
	if msg.ChatJID != cv.jid {
		return
	}
	pos := cv.positionOf(localID)
	if pos < 0 {
		return
	}
	// A reload (a receipt's refreshInPlace falling back to Load, a chat
	// re-open) that ran after the client stored the sent message but
	// before this callback already shows it under its real id, with the
	// pending copy appended after it. The pending copy is the one to go.
	if err == nil && cv.positionOf(msg.ID) >= 0 {
		cv.removeRow(pos)
		return
	}
	cv.msgs[pos] = msg
	delete(cv.byID, localID)
	cv.byID[msg.ID] = msg
	if r := cv.rowFor(localID); r != nil {
		r.msgID = msg.ID
	}
	cv.refillRow(pos)
}

// retrySend is a failed bubble's Retry: the failed row goes, and the
// content is handed back to the composer to send as a fresh message at the
// foot of the thread (WhatsApp moves a retried message to the end too).
func (cv *ConversationView) retrySend(msg client.Message) {
	if cv.onRetry == nil {
		return
	}
	cv.dropUnsent(msg)
	cv.onRetry(msg)
}

// dropUnsent removes the optimistic row msg from the thread and from the
// per-chat unsent list. Must run on the GTK main loop.
func (cv *ConversationView) dropUnsent(msg client.Message) {
	rows := cv.unsent[msg.ChatJID]
	for i := range rows {
		if rows[i].ID == msg.ID {
			cv.unsent[msg.ChatJID] = append(rows[:i], rows[i+1:]...)
			break
		}
	}
	if msg.ChatJID != cv.jid {
		return
	}
	if pos := cv.positionOf(msg.ID); pos >= 0 {
		cv.removeRow(pos)
	}
}

// removeRow takes the row at pos out of the thread and its model. Must run
// on the GTK main loop.
func (cv *ConversationView) removeRow(pos int) {
	if pos < 0 || pos >= len(cv.msgs) {
		return
	}
	delete(cv.byID, cv.msgs[pos].ID)
	cv.msgs = append(cv.msgs[:pos], cv.msgs[pos+1:]...)
	cv.model.Splice(pos, 1)
	// The row after the removed one may have leaned on it for its day
	// separator.
	cv.refillRow(pos)
}

// withUnsent appends the chat's optimistic rows after its stored page.
func (cv *ConversationView) withUnsent(jid string, msgs []client.Message) []client.Message {
	if rows := cv.unsent[jid]; len(rows) > 0 {
		msgs = append(msgs[:len(msgs):len(msgs)], rows...)
	}
	return msgs
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
	// A redelivery of a loaded message (the store keeps one row per id)
	// updates its row rather than adding a second one: a duplicate row
	// would keep rendering the stale copy after the real one changes.
	if pos := cv.positionOf(msg.ID); pos >= 0 {
		if cv.msgs[pos].Deleted {
			// A revoke is sticky, as in the store.
			msg.Deleted = true
		}
		cv.msgs[pos] = msg
		cv.byID[msg.ID] = msg
		cv.refillRow(pos)
		return
	}
	if !cv.typingShown {
		cv.msgs = append(cv.msgs, msg)
	}
	cv.byID[msg.ID] = msg

	if cv.emptyBox.Visible() {
		cv.emptyBox.SetVisible(false)
		cv.setThreadVisible(true)
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

// applyRevoke turns every loaded copy of msgID into a tombstone, and the
// chat's optimistic copies too, without going through the store: a row the
// in-place refresh cannot line up with the store (a send still settling,
// a duplicate) would otherwise keep showing the message. Must run on the
// GTK main loop.
func (cv *ConversationView) applyRevoke(chatJID, msgID string) {
	rows := cv.unsent[chatJID]
	for i := range rows {
		if rows[i].ID == msgID {
			rows[i].Deleted = true
		}
	}
	if chatJID != cv.jid {
		return
	}
	for i := range cv.msgs {
		if cv.msgs[i].ID != msgID || cv.msgs[i].Deleted {
			continue
		}
		cv.msgs[i].Deleted = true
		cv.byID[msgID] = cv.msgs[i]
		cv.refillRow(i)
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
	if cv.composing.kind(jid) == "" {
		return
	}
	for _, sender := range cv.composing.senders(jid) {
		cv.composingGen[composingKey(jid, sender)]++
	}
	cv.composing.clear(jid)
	cv.syncComposing(jid)
}

// syncComposing derives jid's typing/recording presence from who is
// composing there (named in a group, where the header says who) and
// redraws the header and the typing bubble when it is the open chat.
func (cv *ConversationView) syncComposing(jid string) {
	state := cv.presence[jid]
	kind := cv.composing.kind(jid)
	state.Typing, state.Recording = kind == "typing", kind == "recording"
	state.Composers = nil
	if kind != "" && strings.HasSuffix(jid, "@g.us") {
		for _, sender := range cv.composing.senders(jid) {
			state.Composers = append(state.Composers, cv.senderName(sender))
		}
	}
	cv.presence[jid] = state
	if jid == cv.jid {
		cv.refreshHeader()
		cv.refreshTypingBubble()
	}
}

// fillQuote gives a reply whose quoted message is neither loaded nor
// carried with it the store's preview of that message, once per target:
// the quote of a reply to something paged out reads like any other.
func (cv *ConversationView) fillQuote(m *client.Message) {
	if m.ReplyTo == nil || m.ReplyTo.Text != "" {
		return
	}
	if _, ok := cv.byID[m.ReplyTo.MsgID]; ok {
		return
	}
	text, ok := cv.quotes[m.ReplyTo.MsgID]
	if !ok {
		stop := perfStart("quoteRead")
		text, _ = cv.c.MessagePreview(m.ReplyTo.ChatJID, m.ReplyTo.MsgID)
		stop()
		cv.quotes[m.ReplyTo.MsgID] = text
	}
	if text != "" {
		ref := *m.ReplyTo
		ref.Text = text
		m.ReplyTo = &ref
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

// bubbleAvatarSize is the sender avatar beside a group bubble.
const bubbleAvatarSize = 28

// copyableText is what "Copy text" puts on the clipboard for msg: the body
// for a text message, the caption for media, and a plain-text rendering for
// the rich kinds (a location's name, address and map link; a contact's name
// and numbers; a poll's question and options). Empty means there is nothing
// worth copying and the row stays inert.
func copyableText(m client.Message) string {
	switch {
	case m.Deleted:
		return ""
	case m.Choices != nil:
		return choicesCopyText(m)
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
	case m.CallLog != nil:
		return client.CallText(m.CallLog.Video, m.CallLog.Outcome)
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
	onChoice   func(msg client.Message, sel client.ChoiceSelection)
	onEdit     func(client.Message)
	onDelete   func(client.Message)
	onStar     func(client.Message)
	onForward  func(client.Message)
	onStopLive func(client.Message)
	// onOpenViewer opens msg's attachment in the viewer pane; nil falls
	// back to the standalone photo/video windows.
	onOpenViewer func(client.Message)
	// onRetry re-sends a failed optimistic message (the bubble's Retry).
	onRetry func(client.Message)
	// onJumpTo scrolls the thread to msgID (the message a quote answers);
	// nil leaves the quote inert.
	onJumpTo func(msgID string)
	// reactorName names a reaction's sender for the pill's tooltip and
	// list ("You" for our own); nil prints the bare JID.
	reactorName func(jid string) string
	// onLocalPath records that msgID's attachment now sits at path (the
	// bubble downloaded it), so later reads of the view's messages see it.
	onLocalPath func(msgID, path string)
	// onFetchThumbnail asks for msg's high-quality preview (the tile only
	// has the message's ~100px stamp); nil leaves the stamp.
	onFetchThumbnail func(msg client.Message)
	// transcriptOf reports msgID's transcription state (see
	// transcriptState); nil leaves the voice rows without a transcript slot.
	transcriptOf func(msgID string) transcriptState
	// textExpandedOf reports whether msgID's long body is unfolded, and
	// onExpandText records a fold change. Both nil (the viewer, tests)
	// leaves long bodies folded with a working control.
	textExpandedOf func(msgID string) bool
	onExpandText   func(msgID string, open bool)
	// voice hears a voice note start, stop and end (played flag, resume
	// position, auto-advance); zero leaves the rows self-contained.
	voice voiceHooks
	// names resolves the numeric user part of an @mention to a display
	// name ("" when unknown); nil leaves mentions as typed.
	names func(user string) string
	// avatars backs the sender avatar beside an incoming group bubble; nil
	// draws none.
	avatars *avatarCache
	// avatarGen counts how often jid's picture has changed, so a row can tell
	// a sender it is already showing from the same sender with a new picture.
	// nil (the viewer, tests) reads as generation zero throughout.
	avatarGen func(jid string) int
}

// VoteAt casts a vote for option on the message at idx — a dev/screenshot
// hook.
func (cv *ConversationView) VoteAt(idx int, option string) {
	if m, ok := cv.MessageAt(idx); ok && cv.onVote != nil {
		cv.onVote(m, []string{option})
	}
}

// ChoiceAt taps the reply button numbered arg ("" = the first) under the
// business message at idx — a dev/screenshot hook.
func (cv *ConversationView) ChoiceAt(idx int, arg string) {
	m, ok := cv.MessageAt(idx)
	if !ok || m.Choices == nil || cv.onChoice == nil {
		return
	}
	n, _ := strconv.Atoi(arg)
	if n < 0 || n >= len(m.Choices.Buttons) {
		return
	}
	b := m.Choices.Buttons[n]
	cv.onChoice(m, client.ChoiceSelection{ID: b.ID, Label: b.Label, Index: b.Index})
}

// OpenChoiceListAt opens the list picker under the business message at
// idx, the way its button does — a dev/screenshot hook.
func (cv *ConversationView) OpenChoiceListAt(idx int) {
	m, ok := cv.MessageAt(idx)
	if !ok || m.Choices == nil || m.Choices.List == nil {
		return
	}
	list := *m.Choices.List
	showListPickerDialog(cv.window, list, func(r client.ChoiceRow) {
		if cv.onChoice != nil {
			cv.onChoice(m, client.ChoiceSelection{ID: r.ID, Label: r.Title, Description: r.Description, IsRow: true})
		}
	})
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
		onReply: cv.onReply, onReact: cv.onReact, onVote: cv.onVote, onChoice: cv.onChoice, onEdit: cv.onEdit,
		onDelete: cv.onDelete, onStar: cv.onStar, onForward: cv.onForward, onStopLive: cv.onStopLive,
		onOpenViewer: cv.onOpenViewer, onLocalPath: func(id, path string) { cv.setLocalPath(id, path) }, names: cv.mentionName, avatars: cv.avatarCache,
		avatarGen: func(jid string) int { return cv.avatarGens[jid] },
		onRetry:   cv.retrySend, reactorName: cv.senderName, onJumpTo: cv.jumpToQuoted,
		onFetchThumbnail: cv.fetchThumbnail,
		voice: voiceHooks{
			onPlay: cv.voicePlayed, onStop: cv.voiceStopped, onEnded: cv.voiceEnded,
			onTranscribe: cv.transcribe, onCancelTranscribe: cv.cancelTranscribe,
			onToggleTranscript: cv.setTranscriptOpen,
			onTranscriptMore:   cv.setTranscriptMore,
		},
		transcriptOf:   cv.transcriptOf,
		textExpandedOf: cv.textExpanded,
		onExpandText:   cv.setTextExpanded,
	}
}

// voicePlayed marks the voice note msgID as listened to the moment it
// starts: the row turns blue at once, the store and (receipts allowing) the
// sender hear of it in the background. Our own notes and ones already
// played need nothing. Must run on the GTK main loop.
func (cv *ConversationView) voicePlayed(msgID string) {
	pos := cv.positionOf(msgID)
	if pos < 0 {
		return
	}
	m := cv.msgs[pos]
	if m.FromMe || m.Played || m.Attachment == nil {
		return
	}
	m.Played = true
	cv.msgs[pos] = m
	cv.byID[msgID] = m
	trace(1, "voice played: %s", msgID)
	// The row is rebuilt once its own click handler has returned.
	glib.IdleAdd(func() {
		if pos := cv.positionOf(msgID); pos >= 0 {
			cv.refillRow(pos)
		}
	})
	c, jid := cv.c, m.ChatJID
	go func() {
		if err := c.MarkPlayed(context.Background(), jid, msgID, SendReadReceipts); err != nil {
			log.Printf("chatot: mark played failed: %v", err)
		}
	}()
}

// voiceStopped remembers where msgID's playback stopped (0 once it played
// through), in the view's copy and the store, so the next play resumes
// there. The row is not rebuilt: the shared player already sits at that
// position. Must run on the GTK main loop.
func (cv *ConversationView) voiceStopped(msgID string, ms int) {
	pos := cv.positionOf(msgID)
	if pos < 0 {
		return
	}
	m := cv.msgs[pos]
	if m.Attachment == nil {
		return
	}
	a := *m.Attachment
	a.PlayPosMS = ms
	m.Attachment = &a
	cv.msgs[pos] = m
	cv.byID[msgID] = m
	trace(1, "voice stopped: %s at %dms", msgID, ms)
	c, jid := cv.c, m.ChatJID
	go func() {
		if err := c.SetPlayPosition(jid, msgID, ms); err != nil {
			log.Printf("chatot: save play position failed: %v", err)
		}
	}()
}

// voiceEnded plays on into the next voice note when one directly follows
// msgID (WhatsApp keeps a run of notes going until something else comes
// between), fetching it first when it is not in the cache. Must run on the
// GTK main loop.
func (cv *ConversationView) voiceEnded(msgID string) {
	next, ok := nextVoiceMessage(cv.Messages(), msgID)
	if !ok {
		return
	}
	switch voiceChainStepFor(mediaVM(next)) {
	case voiceChainPlay:
		mv := mediaVM(next)
		mv.voice = cv.hooks().voice
		trace(1, "voice chain: %s -> %s", msgID, next.ID)
		playVoice(mv, mv.voice)
	case voiceChainFetch:
		trace(1, "voice chain: %s -> %s (fetching)", msgID, next.ID)
		cv.fetchAndPlayVoice(next)
	}
}

// fetchAndPlayVoice downloads msg's audio and plays it once it lands, which
// is how a run carries on into a note nobody has opened yet. The note is
// dropped if the reader moved on while the file was on its way — another
// note playing, another thread on screen, or this thread reloaded — so the
// run never speaks over whatever came after it. Must run on the GTK main
// loop; the download itself does not.
func (cv *ConversationView) fetchAndPlayVoice(msg client.Message) {
	jid, gen, c := cv.jid, cv.loadGen, cv.c
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), voiceChainFetchTimeout)
		defer cancel()
		path, err := c.DownloadMedia(ctx, msg.ID)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: voice chain download %s: %v", msg.ID, err)
				return
			}
			if cv.jid != jid || cv.loadGen != gen || anyVoicePlaying() {
				return
			}
			cv.setLocalPath(msg.ID, path)
			pos := cv.positionOf(msg.ID)
			if pos < 0 {
				return
			}
			cv.byID[msg.ID] = cv.msgs[pos]
			// The row is still the "not downloaded" one until it is
			// refilled, and the note is about to play in it.
			cv.refillByID(msg.ID)
			mv := mediaVM(cv.msgs[pos])
			if !mv.HasLocal {
				return
			}
			mv.voice = cv.hooks().voice
			trace(1, "voice chain: fetched %s, playing", msg.ID)
			playVoice(mv, mv.voice)
		})
	}()
}

// fetchThumbnail fetches msg's high-quality preview once per session and
// rebinds its bubble with it, so an undownloaded picture or clip shows a
// crisp frame instead of the stamp the message embeds. Must run on the
// GTK main loop.
func (cv *ConversationView) fetchThumbnail(msg client.Message) {
	if cv.thumbTried == nil {
		cv.thumbTried = make(map[string]bool)
	}
	if cv.thumbTried[msg.ID] || msg.Attachment == nil {
		return
	}
	cv.thumbTried[msg.ID] = true
	id, chatJID := msg.ID, msg.ChatJID
	go func() {
		jpeg, err := cv.c.DownloadThumbnail(context.Background(), id)
		if err != nil {
			if !errors.Is(err, client.ErrNoThumbnail) {
				log.Printf("chatot: thumbnail for %s: %v", id, err)
			}
			return
		}
		glib.IdleAdd(func() { cv.setThumbnail(chatJID, id, jpeg) })
	}()
}

// setThumbnail swaps the loaded message's attachment preview for jpeg and
// rebinds its row. Must run on the GTK main loop.
func (cv *ConversationView) setThumbnail(chatJID, msgID string, jpeg []byte) {
	if chatJID != cv.jid {
		return
	}
	pos := cv.positionOf(msgID)
	if pos < 0 || cv.msgs[pos].Attachment == nil || cv.msgs[pos].Attachment.LocalPath != "" {
		return
	}
	a := *cv.msgs[pos].Attachment
	a.Thumbnail = jpeg
	cv.msgs[pos].Attachment = &a
	cv.byID[msgID] = cv.msgs[pos]
	cv.refillRow(pos)
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

// addToStickers files msg's sticker in the picker's library, downloading
// it first when the bubble has not, and reports the outcome.
func (h bubbleHooks) addToStickers(msg client.Message) {
	go func() {
		path := ""
		if msg.Attachment != nil && fileExists(msg.Attachment.LocalPath) {
			path = msg.Attachment.LocalPath
		}
		var err error
		if path == "" {
			path, err = h.c.DownloadMedia(context.Background(), msg.ID)
			if err == nil && h.onLocalPath != nil {
				glib.IdleAdd(func() { h.onLocalPath(msg.ID, path) })
			}
		}
		if err == nil {
			_, err = h.c.AddSticker(path)
		}
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: add sticker to library: %v", err)
				showToast(h.toasts, "Couldn't add the sticker")
				return
			}
			showToast(h.toasts, "Added to your stickers")
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
		if isStickerMessage(msg) {
			actions.AddToStickers = func() { h.addToStickers(msg) }
		}
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

// popover hangs a menu card of about width px off one of the hover buttons
// and keeps the buttons on screen until it closes. The card is aligned to
// the anchor's edge nearest the margin (left for an incoming message, right
// for an outgoing one), the mockup's placement, so a card under a bubble at
// the pane's edge grows inward instead of past the window.
func (a bubbleAffordances) popover(anchor gtk.Widgetter, pos gtk.PositionType, width int) *gtk.Popover {
	return a.popoverAt(alignedRect(a.host, anchor, a.bubble, width, a.fromMe), pos)
}

// popoverAt is popover pointed at an already computed rectangle (nil for
// GTK's default placement).
func (a bubbleAffordances) popoverAt(at *gdk.Rectangle, pos gtk.PositionType) *gtk.Popover {
	*a.showing = true
	pop := newBubblePopover(a.host, a.bubble, at, func() {
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
// that opens the full picker. Picking closes owner, the popover pointed
// at at (see popoverChildRect) that shows the pill.
func buildReactRow(bubble *gtk.Box, msg client.Message, h bubbleHooks, owner *gtk.Popover, at *gdk.Rectangle) *gtk.Box {
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
	shotRegister(msg.ID, func(s *bubbleShot) { s.reactPlus = plus })
	plus.ConnectClicked(func() {
		// Measured before the row pops down: the picker hangs under the
		// ＋ itself, its right edge on the button's (the mockup's anchor).
		under := popoverChildRect(owner, at, plus, reactPickerCardWidth)
		owner.Popdown()
		openReactionPicker(bubble, msg, h, under)
	})
	pill.Append(plus)
	return pill
}

// openReactionPicker is the ＋'s full picker: the mockup's 322px "Pick a
// reaction" card with an eight-column grid, hung below the pointing
// rectangle at (the ＋ button; the bubble's edge when it has none). It is
// chatot's own grid rather than GtkEmojiChooser, which rebuilt its whole
// Unicode table on every open and lagged for seconds.
func openReactionPicker(bubble *gtk.Box, msg client.Message, h bubbleHooks, at *gdk.Rectangle) {
	if at == nil {
		at = alignedRect(h.host, bubble, nil, reactPickerCardWidth, msg.FromMe)
	}
	pop := newBubblePopover(h.host, bubble, at, func() {})
	pop.SetPosition(gtk.PosBottom)
	pop.AddCSSClass("chatot-react-picker")

	// The same catalogue the composer offers, in the reaction card's
	// narrower grid: a reaction is an emoji like any other, and hunting for
	// one in a 32-glyph menu was the complaint.
	pop.SetChild(newEmojiPanel(emojiPanelConfig{
		Columns: reactPickerCols,
		Height:  reactPickerHeight,
		Width:   reactPickerWidth,
		OnPick: func(glyph string) {
			pop.Popdown()
			h.onReact(msg, glyph)
		},
	}))
	pop.Popup()
}

// reactPickerCols/Width/Height are the mockup's reaction card: eight columns
// in a 322px card (306px inside its 8px padding), over a 168px window on the
// catalogue so the card stays a card and the emoji scroll inside it.
const (
	reactPickerCols   = 8
	reactPickerWidth  = 306
	reactPickerHeight = 168
)

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

// newClockGlyph is the pending send's mark where the tick goes: a small
// stroked clock face in the current colour, WhatsApp's "sending" clock.
// Stroked with cairo rather than a theme icon so it inherits the outgoing
// bubble's tick colour like the ✓ label does.
func newClockGlyph(size int) *gtk.DrawingArea {
	area := gtk.NewDrawingArea()
	area.SetSizeRequest(size, size)
	area.SetHAlign(gtk.AlignCenter)
	area.SetVAlign(gtk.AlignCenter)
	area.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		c := area.Color()
		cr.SetSourceRGBA(float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha()))
		cr.SetLineWidth(1.2)
		cr.SetLineCap(cairo.LineCapRound)
		cx, cy := float64(w)/2, float64(h)/2
		r := float64(size)/2 - 1
		cr.Arc(cx, cy, r, 0, 2*3.141592653589793)
		cr.Stroke()
		// Hands at ten past ten: up to the twelve, out to the two.
		cr.MoveTo(cx, cy)
		cr.LineTo(cx, cy-r*0.6)
		cr.MoveTo(cx, cy)
		cr.LineTo(cx+r*0.45, cy)
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

// popoverChildRect is the pointing rectangle for a width px card hung under
// child, a widget inside the open popover pop, right edge on the child's,
// in host coordinates; nil when nothing is placed yet. pop was pointed at
// at (host coordinates) with position top. A popover draws on its own
// surface and ComputeBounds against the host stops there, so the child's
// place is reconstructed from how GTK lays a popover out: its contents
// (the box inside the shadow) centred on the pointing rectangle and
// resting on its top edge, or hanging under its bottom edge when there
// was no room above.
func popoverChildRect(pop *gtk.Popover, at *gdk.Rectangle, child gtk.Widgetter, width int) *gdk.Rectangle {
	if pop == nil || at == nil || pop.Child() == nil {
		return nil
	}
	b, ok := gtk.BaseWidget(child).ComputeBounds(pop)
	if !ok || b.Width() <= 0 || b.Height() <= 0 {
		return nil
	}
	contents := gtk.BaseWidget(pop.Child()).Parent()
	if contents == nil {
		return nil
	}
	cb, ok := gtk.BaseWidget(contents).ComputeBounds(pop)
	if !ok || cb.Width() <= 0 {
		return nil
	}
	left := at.X() + (at.Width()-int(cb.Width()))/2
	top := at.Y() - int(cb.Height())
	if top < 0 {
		top = at.Y() + at.Height()
	}
	x := left + int(b.X()-cb.X())
	y := top + int(b.Y()-cb.Y())
	rect := gdk.NewRectangle(alignPopoverX(x, int(b.Width()), width, true), y, width, int(b.Height()))
	return &rect
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
	names := reactorNames(r.Reactors, h.reactorName)
	pill.SetTooltipText(reactionTooltip(names))
	mine := reactedBy(r.Reactors, h.ownJID)
	if mine {
		pill.AddCSSClass("chatot-reaction-pill-mine")
	}
	open := func() { openReactorList(pill, r, names, msg, h) }
	pill.ConnectClicked(open)
	shotRegister(msg.ID, func(s *bubbleShot) {
		if s.openReactors == nil {
			s.openReactors = open
		}
	})
	return pill
}

// reactorNames resolves each reactor JID through name ("You" for our own),
// falling back to the bare user part when there is no resolver.
func reactorNames(reactors []string, name func(jid string) string) []string {
	out := make([]string, 0, len(reactors))
	for _, jid := range reactors {
		n := ""
		if name != nil {
			n = name(jid)
		}
		if n == "" {
			n = bareJIDUser(jid)
		}
		out = append(out, n)
	}
	return out
}

// reactionTooltip is the hover text of a pill: who reacted with it, the way
// WhatsApp lists them ("You, Ana and Marco"). The names in order of
// reacting, so the tooltip and the list read the same.
func reactionTooltip(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// openReactorList is a pill's click: WhatsApp's reactions sheet for that
// emoji, one row per person with their avatar and name. Our own row says
// so and removes the reaction when clicked; the others are only a list.
func openReactorList(pill gtk.Widgetter, r reactionView, names []string, msg client.Message, h bubbleHooks) {
	at := alignedRect(h.host, pill, nil, reactorListWidth, msg.FromMe)
	pop := newBubblePopover(h.host, pill, at, func() {})
	pop.SetPosition(gtk.PosBottom)
	pop.AddCSSClass("chatot-reactor-list")

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetSizeRequest(reactorListWidth, -1)
	caption := gtk.NewLabel(strings.ToUpper(reactorListCaption(r)))
	caption.SetXAlign(0)
	caption.AddCSSClass("chatot-card-caption")
	col.Append(caption)

	for i, jid := range r.Reactors {
		own := isOwnJID(jid, h.ownJID)
		line := gtk.NewBox(gtk.OrientationHorizontal, 10)
		line.AddCSSClass("chatot-reactor-row")
		if h.avatars != nil {
			line.Append(buildAvatar(h.c, h.avatars, nonADJID(jid), initialFor(names[i]), reactorAvatarSize))
		}
		text := gtk.NewBox(gtk.OrientationVertical, 0)
		text.SetHExpand(true)
		name := gtk.NewLabel(names[i])
		name.SetXAlign(0)
		name.SetEllipsize(pango.EllipsizeEnd)
		name.AddCSSClass("chatot-reactor-name")
		text.Append(name)
		if own {
			hint := gtk.NewLabel("Click to remove")
			hint.SetXAlign(0)
			hint.AddCSSClass("chatot-reactor-hint")
			text.Append(hint)
		}
		line.Append(text)
		emoji := gtk.NewLabel(r.Emoji)
		emoji.AddCSSClass("chatot-reactor-emoji")
		line.Append(emoji)

		if own && h.onReact != nil {
			b := gtk.NewButton()
			b.SetChild(line)
			b.AddCSSClass("flat")
			b.AddCSSClass("chatot-reactor-btn")
			b.ConnectClicked(func() {
				pop.Popdown()
				h.onReact(msg, "")
			})
			col.Append(b)
		} else {
			col.Append(line)
		}
	}
	pop.SetChild(col)
	pop.Popup()
}

// reactorListCaption heads the reactions sheet: the emoji and how many
// picked it.
func reactorListCaption(r reactionView) string {
	if r.Count == 1 {
		return r.Emoji + " · 1 reaction"
	}
	return fmt.Sprintf("%s · %d reactions", r.Emoji, r.Count)
}

// reactorListWidth is the reactions sheet's card; reactorAvatarSize its
// row avatars (the chat list's small size).
const (
	reactorListWidth  = 240
	reactorAvatarSize = 28
)

// Header is the conversation's AdwHeaderBar (identity, search, ⋮ and the
// window controls). It is built here but packed by the window, above the
// stack that swaps the thread for the media and starred pages, so those
// pages sit under the same header rather than replacing it.
func (cv *ConversationView) Header() gtk.Widgetter { return cv.header }

// SetCollapsed follows the split view: collapsed, the header shows the ←
// back to the chat list and the start side of the window controls.
func (cv *ConversationView) SetCollapsed(collapsed bool) {
	cv.backBtn.SetVisible(collapsed)
	cv.startControlsSlot.SetVisible(collapsed)
	cv.setCompactRows(collapsed)
	setCSSClass(cv.header, "chatot-collapsed", collapsed)
	// The root too: the thread list's side padding shrinks with it.
	setCSSClass(cv, "chatot-collapsed", collapsed)
}

// setCompactRows switches every realized row, and the rows made after,
// between the side-by-side and floating hover layouts.
func (cv *ConversationView) setCompactRows(on bool) {
	cv.compactRows = on
	for _, r := range cv.rows {
		r.setCompact(on)
	}
}

// OnBackRequested registers the collapsed header's ← handler.
func (cv *ConversationView) OnBackRequested(f func()) { cv.onBack = f }
