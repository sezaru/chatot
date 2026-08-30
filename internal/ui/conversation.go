package ui

import (
	"fmt"
	"sort"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

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
	Edited           bool
	EditedMarker     string
	Deleted          bool
	// TickText is the own-message delivery/read indicator ("✓"/"✓✓"), set
	// only when FromMe; TickRead marks it should render in the accent color
	// (status == read) rather than the plain dim tick.
	TickText string
	TickRead bool
	// StarGlyph/StarTooltip drive the bubble's star toggle; see starAffordanceVM.
	StarGlyph   string
	StarTooltip string
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
	case m.Attachment != nil:
		v.IsMedia = true
		v.Media = mediaVM(m)
		v.MediaChip = v.Media.Chip
	default:
		v.Text = m.Text
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

	header        *gtk.Box
	avatarSlot    *gtk.Box
	avatarCache   *avatarCache
	avatarJID     string // jid the avatar widget currently shows, "" until set
	titleLabel    *gtk.Label
	subtitleLabel *gtk.Label
	scroller      *gtk.ScrolledWindow
	listView      *gtk.ListView
	model         *gioutil.ListModel[client.Message]
	empty         *gtk.Label

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

	// presence is the UI's own view of contact/chat presence, built from
	// EventPresence/EventChatPresence on Events() — chosen over growing the
	// Client interface with a Presence(jid) getter, since ConversationView
	// (and ChatList, for the typing preview override) already consume that
	// stream directly. Keyed by contact JID for EventPresence (online/
	// last-seen) and by chat JID for EventChatPresence (typing); those
	// coincide for 1:1 chats, which is the only case the header's
	// online/last-seen line is expected to cover.
	presence map[string]PresenceState

	onReply  func(client.Message)
	onReact  func(msg client.Message, emoji string)
	onVote   func(msg client.Message, options []string)
	onEdit   func(client.Message)
	onDelete func(client.Message)
	onStar   func(client.Message)
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

// NewConversationView builds an empty ConversationView backed by c and
// subscribes to c.Events() for live append.
func NewConversationView(c client.Client) *ConversationView {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.SetVExpand(true)
	root.SetHExpand(true)

	header := gtk.NewBox(gtk.OrientationHorizontal, 10)
	header.AddCSSClass("chatot-conv-header")
	header.SetMarginTop(6)
	header.SetMarginBottom(6)
	header.SetMarginStart(10)
	header.SetMarginEnd(10)

	avatarSlot := gtk.NewBox(gtk.OrientationVertical, 0)
	header.Append(avatarSlot)

	textCol := gtk.NewBox(gtk.OrientationVertical, 0)
	header.Append(textCol)

	titleLabel := gtk.NewLabel("")
	titleLabel.SetXAlign(0)
	titleLabel.AddCSSClass("chatot-conv-title")
	textCol.Append(titleLabel)

	subtitleLabel := gtk.NewLabel("")
	subtitleLabel.SetXAlign(0)
	subtitleLabel.AddCSSClass("chatot-conv-subtitle")
	textCol.Append(subtitleLabel)

	header.SetVisible(false)
	root.Append(header)

	empty := gtk.NewLabel("Select a chat")
	empty.AddCSSClass("chatot-placeholder")
	empty.SetVExpand(true)
	root.Append(empty)

	model := conversationModelType.New()

	// GtkListView virtualizes: it only realizes bubble widgets for rows in (or
	// near) the viewport and unrealizes the rest as they scroll off, so the
	// live widget count stays bounded no matter how far back history is paged.
	factory := gtk.NewSignalListItemFactory()
	scroller := gtk.NewScrolledWindow()

	cv := &ConversationView{
		Box:           root,
		c:             c,
		events:        c.Events(),
		header:        header,
		avatarSlot:    avatarSlot,
		avatarCache:   newAvatarCache(),
		titleLabel:    titleLabel,
		subtitleLabel: subtitleLabel,
		scroller:      scroller,
		model:         model,
		empty:         empty,
		presence:      make(map[string]PresenceState),
	}

	factory.ConnectSetup(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		row := gtk.NewBox(gtk.OrientationVertical, 0)
		row.SetMarginStart(8)
		row.SetMarginEnd(8)
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
	box.Append(buildBubble(msg, vm, cv.c, cv.onReply, cv.onReact, cv.onVote, cv.onEdit, cv.onDelete, cv.onStar))
}

// onScroll fetches the next older page when the reader nears the top. Runs on
// the GTK main loop (fired by the adjustment). loadingOlder debounces the
// burst of value-changed signals a single scroll gesture emits.
func (cv *ConversationView) onScroll() {
	if cv.loadingOlder || !cv.hasMore || cv.jid == "" {
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
	cv.jid = jid
	cv.loadingOlder = false
	cv.refreshHeader()

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
		cv.empty.SetVisible(true)
		cv.scroller.SetVisible(false)
		return
	}

	cv.empty.SetVisible(false)
	cv.scroller.SetVisible(true)
	cv.scrollToBottom()
}

// loadOlder prepends the next older page. GtkListView keeps the viewport
// anchored on a model splice-at-front only loosely, so we re-scroll to the
// message that was at the top (now shifted down by len(older)) once the rows
// relayout. Must run on the GTK main loop.
func (cv *ConversationView) loadOlder() {
	cv.loadingOlder = true

	older, err := cv.c.MessagesBefore(cv.jid, cv.oldestID, conversationPageSize)
	if err != nil || len(older) == 0 {
		cv.hasMore = false
		cv.loadingOlder = false
		return
	}

	anchor := len(older)
	cv.msgs = append(older, cv.msgs...)
	cv.oldestID = cv.msgs[0].ID
	cv.byID = indexByID(cv.msgs)
	cv.hasMore = len(older) == conversationPageSize
	cv.model.Splice(0, 0, older...)

	glib.IdleAdd(func() {
		cv.listView.ScrollTo(uint(anchor), gtk.ListScrollNone, nil)
		cv.loadingOlder = false
	})
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
				cv.Load(cv.jid)
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
				cv.Load(cv.jid)
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
				cv.Load(cv.jid)
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
				cv.Load(cv.jid)
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
				state.Typing = cp.State == "composing"
				cv.presence[cp.ChatJID] = state
				if cp.ChatJID == cv.jid {
					cv.refreshHeader()
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
		cv.header.SetVisible(false)
		return
	}
	name := cv.chatName(cv.jid)
	cv.titleLabel.SetLabel(name)
	cv.subtitleLabel.SetLabel(presenceSubtitle(cv.presence[cv.jid], time.Now()))
	cv.header.SetVisible(true)

	if cv.avatarJID != cv.jid {
		cv.avatarJID = cv.jid
		removeAllChildren(cv.avatarSlot)
		cv.avatarSlot.Append(buildAvatar(cv.c, cv.avatarCache, cv.jid, initialFor(name), conversationAvatarSize))
	}
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
	cv.Load(cv.jid)
}

// appendMessage adds msg to the end of the currently-loaded thread. The
// factory renders the new row when it realizes at the bottom. Must run on the
// GTK main loop.
func (cv *ConversationView) appendMessage(msg client.Message) {
	cv.msgs = append(cv.msgs, msg)
	cv.byID[msg.ID] = msg

	if cv.empty.Visible() {
		cv.empty.SetVisible(false)
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
func buildBubble(msg client.Message, vm bubbleView, c client.Client, onReply func(client.Message), onReact func(msg client.Message, emoji string), onVote func(msg client.Message, options []string), onEdit func(client.Message), onDelete func(client.Message), onStar func(client.Message)) *gtk.Box {
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

	bubble := gtk.NewBox(gtk.OrientationVertical, 2)
	bubble.AddCSSClass("chatot-bubble")
	if vm.FromMe {
		bubble.AddCSSClass("chatot-bubble-out")
	} else {
		bubble.AddCSSClass("chatot-bubble-in")
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
	} else if vm.IsMedia {
		bubble.Append(buildMediaContent(msg, vm.Media, c))
	} else {
		text := gtk.NewLabel(vm.Text)
		text.AddCSSClass("chatot-bubble-text")
		if vm.Deleted {
			text.AddCSSClass("chatot-bubble-deleted")
		}
		text.SetXAlign(0)
		text.SetWrap(true)
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
	canEdit := !vm.Deleted && msg.FromMe && !vm.IsMedia && !vm.IsLocation && !vm.IsContact && !vm.IsPoll
	canDelete := !vm.Deleted && msg.FromMe
	if !vm.Deleted && (onReply != nil || onReact != nil || (canEdit && onEdit != nil) || (canDelete && onDelete != nil) || onStar != nil) {
		bubble.Append(buildBubbleActions(msg, vm, onReply, onReact, onEdit, onDelete, onStar, canEdit, canDelete))
	}

	row.Append(bubble)
	wrapper.Append(row)

	return wrapper
}

// buildBubbleActions builds the small reply/react/edit/delete/star affordance
// row shown on non-deleted bubbles. Star applies to any message, own or
// theirs, unlike edit/delete which are own-message only.
func buildBubbleActions(msg client.Message, vm bubbleView, onReply func(client.Message), onReact func(msg client.Message, emoji string), onEdit func(client.Message), onDelete func(client.Message), onStar func(client.Message), canEdit, canDelete bool) *gtk.Box {
	actions := gtk.NewBox(gtk.OrientationHorizontal, 2)
	actions.AddCSSClass("chatot-bubble-actions")

	if onReply != nil {
		replyBtn := gtk.NewButtonWithLabel("↩")
		replyBtn.AddCSSClass("flat")
		replyBtn.ConnectClicked(func() { onReply(msg) })
		actions.Append(replyBtn)
	}

	if canEdit && onEdit != nil {
		editBtn := gtk.NewButtonWithLabel("✎")
		editBtn.AddCSSClass("flat")
		editBtn.ConnectClicked(func() { onEdit(msg) })
		actions.Append(editBtn)
	}

	if onReact != nil {
		menuBtn := gtk.NewButtonWithLabel("🙂+")
		menuBtn.AddCSSClass("flat")

		popover := gtk.NewPopover()
		picker := gtk.NewBox(gtk.OrientationHorizontal, 4)
		for _, emoji := range reactEmojis {
			emojiBtn := gtk.NewButtonWithLabel(emoji)
			emojiBtn.AddCSSClass("flat")
			emojiBtn.ConnectClicked(func() {
				onReact(msg, emoji)
				popover.Popdown()
			})
			picker.Append(emojiBtn)
		}
		popover.SetChild(picker)
		popover.SetParent(menuBtn)
		menuBtn.ConnectClicked(func() { popover.Popup() })
		actions.Append(menuBtn)
	}

	if canDelete && onDelete != nil {
		deleteBtn := gtk.NewButtonWithLabel("🗑")
		deleteBtn.AddCSSClass("flat")
		deleteBtn.ConnectClicked(func() { onDelete(msg) })
		actions.Append(deleteBtn)
	}

	if onStar != nil {
		starBtn := gtk.NewButtonWithLabel(vm.StarGlyph)
		starBtn.SetTooltipText(vm.StarTooltip)
		starBtn.AddCSSClass("flat")
		starBtn.ConnectClicked(func() { onStar(msg) })
		actions.Append(starBtn)
	}

	return actions
}
