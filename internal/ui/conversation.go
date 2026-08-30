package ui

import (
	"fmt"
	"sort"
	"time"

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
}

// bubbleVM derives the display view-model for a single message. prev is the
// previous message in the thread (nil for the first), used to decide
// whether to show a day separator. byID resolves reply targets among the
// loaded messages. now is injected so Today/Yesterday are deterministic.
func bubbleVM(m client.Message, prev *client.Message, byID map[string]client.Message, now time.Time) bubbleView {
	v := bubbleView{
		TimeText: time.Unix(m.TS, 0).In(now.Location()).Format("15:04"),
		FromMe:   m.FromMe,
	}

	if prev == nil || !sameDay(prev.TS, m.TS, now.Location()) {
		v.ShowDaySeparator = true
		v.DayText = dayText(m.TS, now)
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

	if m.Attachment != nil {
		v.IsMedia = true
		v.Media = mediaVM(m)
		v.MediaChip = v.Media.Chip
	} else {
		v.Text = m.Text
	}

	return v
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
	titleLabel    *gtk.Label
	subtitleLabel *gtk.Label
	scroller      *gtk.ScrolledWindow
	list          *gtk.Box
	empty         *gtk.Label

	msgs []client.Message
	byID map[string]client.Message

	// presence is the UI's own view of contact/chat presence, built from
	// EventPresence/EventChatPresence on Events() — chosen over growing the
	// Client interface with a Presence(jid) getter, since ConversationView
	// (and ChatList, for the typing preview override) already consume that
	// stream directly. Keyed by contact JID for EventPresence (online/
	// last-seen) and by chat JID for EventChatPresence (typing); those
	// coincide for 1:1 chats, which is the only case the header's
	// online/last-seen line is expected to cover.
	presence map[string]PresenceState

	onReply func(client.Message)
	onReact func(msg client.Message, emoji string)
}

// OnReplyRequested registers f to be called when the user picks the reply
// affordance on a bubble; the composer wires this to StartReply.
func (cv *ConversationView) OnReplyRequested(f func(client.Message)) { cv.onReply = f }

// OnReactRequested registers f to be called when the user picks an emoji
// from a bubble's react affordance; msg carries the ChatJID needed to send.
func (cv *ConversationView) OnReactRequested(f func(msg client.Message, emoji string)) {
	cv.onReact = f
}

// Messages returns the currently-loaded thread, for mark-read on open.
func (cv *ConversationView) Messages() []client.Message { return cv.msgs }

// CurrentJID returns the chat currently loaded, "" if none.
func (cv *ConversationView) CurrentJID() string { return cv.jid }

const conversationLimit = 200

// NewConversationView builds an empty ConversationView backed by c and
// subscribes to c.Events() for live append.
func NewConversationView(c client.Client) *ConversationView {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.SetVExpand(true)
	root.SetHExpand(true)

	header := gtk.NewBox(gtk.OrientationVertical, 0)
	header.AddCSSClass("chatot-conv-header")
	header.SetMarginTop(6)
	header.SetMarginBottom(6)
	header.SetMarginStart(10)
	header.SetMarginEnd(10)

	titleLabel := gtk.NewLabel("")
	titleLabel.SetXAlign(0)
	titleLabel.AddCSSClass("chatot-conv-title")
	header.Append(titleLabel)

	subtitleLabel := gtk.NewLabel("")
	subtitleLabel.SetXAlign(0)
	subtitleLabel.AddCSSClass("chatot-conv-subtitle")
	header.Append(subtitleLabel)

	header.SetVisible(false)
	root.Append(header)

	empty := gtk.NewLabel("Select a chat")
	empty.AddCSSClass("chatot-placeholder")
	empty.SetVExpand(true)
	root.Append(empty)

	list := gtk.NewBox(gtk.OrientationVertical, 4)
	list.SetMarginTop(8)
	list.SetMarginBottom(8)
	list.SetMarginStart(8)
	list.SetMarginEnd(8)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetHExpand(true)
	scroller.SetChild(list)
	scroller.SetVisible(false)

	root.Append(scroller)

	cv := &ConversationView{
		Box:           root,
		c:             c,
		events:        c.Events(),
		header:        header,
		titleLabel:    titleLabel,
		subtitleLabel: subtitleLabel,
		scroller:      scroller,
		list:          list,
		empty:         empty,
		presence:      make(map[string]PresenceState),
	}

	go cv.watchEvents()

	return cv
}

// Load queries client.Messages for jid and renders the full thread,
// replacing whatever was shown before. Must run on the GTK main loop.
func (cv *ConversationView) Load(jid string) {
	cv.jid = jid
	cv.refreshHeader()

	msgs, err := cv.c.Messages(jid, conversationLimit)
	if err != nil {
		msgs = nil
	}
	cv.msgs = msgs
	cv.byID = indexByID(msgs)

	removeAllChildren(cv.list)

	if len(msgs) == 0 {
		cv.empty.SetLabel("No messages yet")
		cv.empty.SetVisible(true)
		cv.scroller.SetVisible(false)
		return
	}

	cv.empty.SetVisible(false)
	cv.scroller.SetVisible(true)

	now := time.Now()
	var prev *client.Message
	for i := range msgs {
		vm := bubbleVM(msgs[i], prev, cv.byID, now)
		cv.list.Append(buildBubble(msgs[i], vm, cv.c, cv.onReply, cv.onReact))
		prev = &msgs[i]
	}

	cv.scrollToBottom()
}

// watchEvents listens for client events and, for the currently-loaded chat,
// schedules a UI update on the GTK main loop via glib.IdleAdd. New messages
// are appended in place; reactions trigger a full reload (simpler, and the
// thread sizes here don't warrant a targeted patch). Presence/chat-presence
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
				cv.appendMessage(msg)
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
		}
	}
}

// refreshHeader repaints the title/subtitle for the currently-open chat
// (hides the whole header if none is open). Must run on the GTK main loop.
func (cv *ConversationView) refreshHeader() {
	if cv.jid == "" {
		cv.header.SetVisible(false)
		return
	}
	cv.titleLabel.SetLabel(cv.chatName(cv.jid))
	cv.subtitleLabel.SetLabel(presenceSubtitle(cv.presence[cv.jid], time.Now()))
	cv.header.SetVisible(true)
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

// appendMessage adds msg to the end of the currently-loaded thread without
// reloading the whole list. Must run on the GTK main loop.
func (cv *ConversationView) appendMessage(msg client.Message) {
	var prev *client.Message
	if n := len(cv.msgs); n > 0 {
		prev = &cv.msgs[n-1]
	}

	cv.msgs = append(cv.msgs, msg)
	cv.byID[msg.ID] = msg

	if cv.empty.Visible() {
		cv.empty.SetVisible(false)
		cv.scroller.SetVisible(true)
	}

	vm := bubbleVM(msg, prev, cv.byID, time.Now())
	cv.list.Append(buildBubble(msg, vm, cv.c, cv.onReply, cv.onReact))
	cv.scrollToBottom()
}

// scrollToBottom scrolls the message list to the newest message. Deferred
// via glib.IdleAdd since the scrolled window's adjustment upper bound isn't
// updated until after the just-appended widget is laid out.
func (cv *ConversationView) scrollToBottom() {
	glib.IdleAdd(func() {
		adj := cv.scroller.VAdjustment()
		adj.SetValue(adj.Upper())
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
func buildBubble(msg client.Message, vm bubbleView, c client.Client, onReply func(client.Message), onReact func(msg client.Message, emoji string)) *gtk.Box {
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

	if vm.IsMedia {
		bubble.Append(buildMediaContent(msg, vm.Media, c))
	} else {
		text := gtk.NewLabel(vm.Text)
		text.AddCSSClass("chatot-bubble-text")
		text.SetXAlign(0)
		text.SetWrap(true)
		bubble.Append(text)
	}

	timeLabel := gtk.NewLabel(vm.TimeText)
	timeLabel.AddCSSClass("chatot-bubble-time")
	timeLabel.SetXAlign(1)
	bubble.Append(timeLabel)

	if len(vm.Reactions) > 0 {
		reactions := gtk.NewBox(gtk.OrientationHorizontal, 2)
		reactions.AddCSSClass("chatot-bubble-reactions")
		for _, emoji := range vm.Reactions {
			r := gtk.NewLabel(emoji)
			reactions.Append(r)
		}
		bubble.Append(reactions)
	}

	if onReply != nil || onReact != nil {
		bubble.Append(buildBubbleActions(msg, onReply, onReact))
	}

	row.Append(bubble)
	wrapper.Append(row)

	return wrapper
}

// buildBubbleActions builds the small reply/react affordance row shown on
// every bubble.
func buildBubbleActions(msg client.Message, onReply func(client.Message), onReact func(msg client.Message, emoji string)) *gtk.Box {
	actions := gtk.NewBox(gtk.OrientationHorizontal, 2)
	actions.AddCSSClass("chatot-bubble-actions")

	if onReply != nil {
		replyBtn := gtk.NewButtonWithLabel("↩")
		replyBtn.AddCSSClass("flat")
		replyBtn.ConnectClicked(func() { onReply(msg) })
		actions.Append(replyBtn)
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

	return actions
}
