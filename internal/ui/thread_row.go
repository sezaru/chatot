package ui

import (
	"chatot/internal/client"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// A thread row's chrome outlives the message inside it. GtkListView recycles
// one row widget through thousands of messages during a fling, and measuring
// (2026-09-10, CHATOT_PERF over a 12000 px/s bench) put the cost in layout
// rather than in painting, the store or the markup: every bind used to build a
// brand-new subtree that GTK had to CSS-match and measure from scratch, and
// frame gap tracked rows-bound-per-frame almost exactly. So everything that
// does not depend on which message is showing is built once, in the factory's
// setup, and a bind only refills the bubble's interior and flips the chrome.
//
// Permanent: the wrapper, both separators, the band, the row, the avatar slot,
// the overlay, the reactions strip and the hover pair.
// Rebuilt per bind: the bubble's interior, the reaction pills, the avatar.

type threadRow struct {
	// msgID is the message the row is showing, empty once it is unbound.
	msgID      string
	wrapper    *gtk.Box
	daySep     *gtk.Label
	unreadSep  *gtk.Label
	band       *gtk.Box
	row        *gtk.Box
	avatarSlot *gtk.Box
	stack      *gtk.Overlay
	typingSlot *gtk.Box
	bubble     *gtk.Box
	reactions  *gtk.Box
	hover      *hoverButtons
	// compact floats the hover pair over the bubble instead of beside it;
	// see hoverButtons.place.
	compact bool
	// The bubble's interior, appended once in this order and shown or hidden
	// per bind: author, forwarded marker, quote, the rich content, the body,
	// the read-more button, the footer. Rebuilding these was a quarter of
	// what a bind cost (BenchmarkRowReusedInterior). The two slots are the
	// exceptions: a picture, location, poll, contact, call, event or link
	// card is a different subtree every time and carries its own handlers,
	// and none of them is common enough on a fling to be worth keeping.
	author    *gtk.Label
	fwd       *gtk.Label
	quote     *gtk.Label
	content   gtk.Widgetter
	body      *gtk.Label
	more      *gtk.Button
	footer    *gtk.Box
	retry     *gtk.Button
	timeLabel *gtk.Label
	clock     *gtk.DrawingArea
	tick      *gtk.Label
	// What the quote and the Retry button act on. Their handlers are made
	// once and read these, so a bind never rewires a signal.
	quoteTo  string
	onJumpTo func(string)
	retryMsg client.Message
	onRetry  func(client.Message)
	// What the avatar slot is currently showing. A group thread runs long
	// stretches from one sender, and a recycled row often lands on the same
	// one, so the picture is kept unless the sender, the initial or the
	// sender's picture changed. The chat list's rows have always done this.
	avatarJID     string
	avatarInitial string
	avatarGen     int
}

func newThreadRow() *threadRow {
	r := &threadRow{
		wrapper:    gtk.NewBox(gtk.OrientationVertical, 4),
		daySep:     gtk.NewLabel(""),
		unreadSep:  gtk.NewLabel(""),
		band:       gtk.NewBox(gtk.OrientationHorizontal, 0),
		row:        gtk.NewBox(gtk.OrientationHorizontal, 6),
		avatarSlot: gtk.NewBox(gtk.OrientationVertical, 0),
		stack:      gtk.NewOverlay(),
		typingSlot: gtk.NewBox(gtk.OrientationVertical, 0),
		bubble:     gtk.NewBox(gtk.OrientationVertical, 2),
		reactions:  gtk.NewBox(gtk.OrientationHorizontal, 4),
		hover:      newHoverButtons(),
		author:     gtk.NewLabel(""),
		fwd:        gtk.NewLabel("↩ Forwarded"),
		quote:      gtk.NewLabel(""),
		body:       gtk.NewLabel(""),
		footer:     gtk.NewBox(gtk.OrientationHorizontal, 4),
		retry:      gtk.NewButtonWithLabel("↻ Retry"),
		timeLabel:  gtk.NewLabel(""),
		clock:      newClockGlyph(11),
		tick:       gtk.NewLabel(""),
	}
	r.buildBubbleInterior()

	r.daySep.AddCSSClass("chatot-day-separator")
	r.daySep.SetHAlign(gtk.AlignCenter)
	r.unreadSep.AddCSSClass("chatot-unread-separator")
	r.unreadSep.SetHAlign(gtk.AlignCenter)
	r.wrapper.Append(r.daySep)
	r.wrapper.Append(r.unreadSep)

	// band spans the pane; row inside it hugs the bubble at one margin.
	r.band.SetHExpand(true)
	r.row.SetHExpand(true)
	r.band.Append(r.row)
	r.wrapper.Append(r.band)
	r.typingSlot.SetVisible(false)
	r.wrapper.Append(r.typingSlot)

	r.avatarSlot.SetVAlign(gtk.AlignStart)
	r.stack.SetChild(r.bubble)

	// Reactions hang off the bubble's bottom edge as small white pills (the
	// mockup's bottom:-11px), so they overlay the bubble rather than growing
	// it. The row rather than the overlay carries the extra bottom room
	// (chatot-row-reacted), as WhatsApp does, so a pill never sits on top of
	// the next message and the hover icons stay centred on the bubble.
	r.reactions.AddCSSClass("chatot-bubble-reactions")
	r.reactions.SetVAlign(gtk.AlignEnd)
	r.stack.AddOverlay(r.reactions)

	// Fixed order, only ever reshuffled: the avatar slot, then the hover pair
	// and the bubble in whichever order keeps the buttons on the side away
	// from the margin.
	r.row.Append(r.avatarSlot)
	r.row.Append(r.hover.box)
	r.row.Append(r.stack)

	motion := gtk.NewEventControllerMotion()
	motion.ConnectEnter(func(_, _ float64) { r.hover.setVisible(true) })
	motion.ConnectMotion(func(_, _ float64) { r.hover.setVisible(true) })
	motion.ConnectLeave(func() { r.hover.setVisible(false) })
	// On the band, not the row: the row hugs the bubble, and the buttons must
	// come up from anywhere along the message's line.
	r.band.AddController(motion)
	return r
}

// buildBubbleInterior makes the parts of a bubble that every message has, in
// the order they stack, and wires the two handlers that need to outlive a
// bind. Everything here is configured once; a bind only sets text, toggles a
// class and flips visibility.
func (r *threadRow) buildBubbleInterior() {
	r.author.AddCSSClass("chatot-bubble-author")
	r.author.SetXAlign(0)
	r.author.SetEllipsize(pango.EllipsizeEnd)
	r.author.SetMaxWidthChars(40)

	r.fwd.AddCSSClass("chatot-forwarded")
	r.fwd.SetXAlign(0)

	r.quote.AddCSSClass("chatot-bubble-quote")
	r.quote.SetXAlign(0)
	r.quote.SetWrap(true)
	// The quote is the way back to what it answers.
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) {
		if r.onJumpTo != nil && r.quoteTo != "" {
			r.onJumpTo(r.quoteTo)
		}
	})
	r.quote.AddController(click)

	r.body.AddCSSClass("chatot-bubble-text")
	r.body.SetXAlign(0)
	r.body.SetWrap(true)
	// WrapWordChar so a long unbroken token (e.g. a URL) still breaks
	// instead of forcing the bubble wider than the pane.
	r.body.SetWrapMode(pango.WrapWordChar)
	// Cap the natural width so a long paragraph wraps into a hugging bubble
	// (~two-thirds of the pane) instead of stretching edge-to-edge, matching
	// the mockup's bubble sizing.
	r.body.SetMaxWidthChars(48)

	r.footer.SetHAlign(gtk.AlignEnd)
	// WhatsApp's failed send: a Retry beside the time and a red badge where
	// the tick would be. Retrying re-sends the same content as a fresh
	// message at the foot of the thread.
	r.retry.AddCSSClass("flat")
	r.retry.AddCSSClass("chatot-retry")
	r.retry.SetTooltipText("Send again")
	r.retry.SetVAlign(gtk.AlignCenter)
	r.retry.ConnectClicked(func() {
		if r.onRetry != nil {
			r.onRetry(r.retryMsg)
		}
	})
	r.timeLabel.AddCSSClass("chatot-bubble-time")
	r.timeLabel.SetXAlign(1)
	r.clock.AddCSSClass("chatot-bubble-tick")
	r.clock.SetTooltipText("Sending…")
	// One label for both the failed badge and the tick: they are never on
	// screen together, and they differ only in text and class.
	r.footer.Append(r.retry)
	r.footer.Append(r.timeLabel)
	r.footer.Append(r.clock)
	r.footer.Append(r.tick)

	r.bubble.Append(r.author)
	r.bubble.Append(r.fwd)
	r.bubble.Append(r.quote)
	r.bubble.Append(r.body)
	r.bubble.Append(r.footer)
}

// fillBubble shows msg in the bubble's interior, reusing every part of it.
func (r *threadRow) fillBubble(msg client.Message, vm bubbleView, h bubbleHooks) {
	defer perfStart("fillBubble")()

	// Hiding a label is not enough: a recycled row would keep the previous
	// message's author line, quote, body or tick sitting in the tree, one
	// slip in the visibility logic away from being shown under someone
	// else's message. Every one of them is cleared as it goes.
	r.author.SetVisible(vm.Author != "")
	if vm.Author != "" {
		r.author.SetLabel(vm.Author)
	} else {
		r.author.SetLabel("")
	}
	r.fwd.SetVisible(vm.Forwarded)

	r.quote.SetVisible(vm.HasQuote)
	if !vm.HasQuote {
		r.quote.SetLabel("")
	}
	if vm.HasQuote {
		r.quote.SetLabel(resolveMentionsPlain(vm.QuotedText, h.names))
		r.quoteTo, r.onJumpTo = "", h.onJumpTo
		if h.onJumpTo != nil && msg.ReplyTo != nil {
			r.quoteTo = msg.ReplyTo.MsgID
			r.quote.SetCursorFromName("pointer")
			r.quote.SetTooltipText("Go to the original message")
		} else {
			r.quote.SetCursorFromName("")
			r.quote.SetTooltipText("")
		}
	}

	r.fillContent(msg, vm, h)
	r.fillFooter(msg, vm, h)
}

// setContent puts w between the quote and the body, taking out whatever the
// last message left there. A nil w leaves the bubble with no rich content.
func (r *threadRow) setContent(w gtk.Widgetter) {
	if r.content != nil {
		r.bubble.Remove(r.content)
		r.content = nil
	}
	if w != nil {
		r.bubble.InsertChildAfter(w, r.quote)
		r.content = w
	}
}

// setMore puts the read-more button under the body, or takes it away.
func (r *threadRow) setMore(b *gtk.Button) {
	if r.more != nil {
		r.bubble.Remove(r.more)
		r.more = nil
	}
	if b != nil {
		r.bubble.InsertChildAfter(b, r.body)
		r.more = b
	}
}

// fillContent builds whatever the message carries above its body, and puts
// the body itself in one of its three states: a caption under a picture, a
// tombstone, or the message text.
func (r *threadRow) fillContent(msg client.Message, vm bubbleView, h bubbleHooks) {
	switch {
	case vm.IsLocation:
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
		r.setContent(buildLocationContent(vm.Location, stop, open))
	case vm.IsContact:
		r.setContent(buildContactContent(vm.Contact))
	case vm.IsPoll:
		r.setContent(buildPollContent(msg, vm.Poll, h.onVote))
	case vm.IsEvent:
		r.setContent(buildEventContent(vm.Event))
	case vm.IsCall:
		r.setContent(buildCallContent(vm.Call))
	case vm.IsMedia:
		media := vm.Media
		if h.onFetchThumbnail != nil {
			m := msg
			media.fetchThumb = func() { h.onFetchThumbnail(m) }
		}
		media.voice = h.voice
		if h.transcriptOf != nil {
			media.TranscriptState = h.transcriptOf(msg.ID)
		}
		r.setContent(buildMediaContent(msg, media, h.c, h.mediaOpener(msg)))
	default:
		if vm.Link != nil {
			r.setContent(buildLinkCard(*vm.Link))
		} else {
			r.setContent(nil)
		}
	}
	r.fillBody(msg, vm, h)
}

func (r *threadRow) fillBody(msg client.Message, vm bubbleView, h bubbleHooks) {
	// A caption reads like a text bubble's body, under the picture: links
	// open, mentions resolve, the text copies.
	caption := vm.IsMedia && vm.CaptionText != ""
	rich := vm.IsLocation || vm.IsContact || vm.IsPoll || vm.IsEvent || vm.IsCall || (vm.IsMedia && !caption)
	r.body.SetVisible(!rich)
	setCSSClass(r.body, "chatot-bubble-caption", caption)
	setCSSClass(r.body, "chatot-bubble-deleted", !caption && vm.Deleted)
	setCSSClass(r.body, "chatot-emoji-only", !caption && vm.IsEmojiOnly)
	r.body.SetSelectable(!vm.Deleted)
	if rich {
		r.body.SetLabel("")
		r.setMore(nil)
		return
	}

	if caption {
		r.body.SetMarkup(messageMarkup(vm.CaptionText, h.names, vm.FromMe, mentionAccentFor(), h.searchQuery))
		r.setMore(nil)
		return
	}
	if vm.Deleted {
		r.body.SetLabel(vm.Text)
		r.setMore(nil)
		return
	}

	// A long body is folded behind a "Read more", as WhatsApp folds one. The
	// clip happens on the plain text, before the markup, so no tag is ever
	// cut in half.
	long := !vm.IsEmojiOnly && isLongText(vm.Text)
	open := long && h.textExpandedOf != nil && h.textExpandedOf(msg.ID)
	body := func() string {
		if long && !open {
			return clipText(vm.Text)
		}
		return vm.Text
	}
	// Links open on click (GtkLabel's own activate-link opens the URI), and
	// the text can be swept and copied.
	r.body.SetMarkup(messageMarkup(body(), h.names, vm.FromMe, mentionAccentFor(), h.searchQuery))
	if !long {
		r.setMore(nil)
		return
	}
	id := msg.ID
	more := gtk.NewButtonWithLabel(readMoreLabel(open))
	more.AddCSSClass("flat")
	more.AddCSSClass("chatot-read-more")
	more.SetHAlign(gtk.AlignStart)
	more.SetFocusOnClick(false)
	more.ConnectClicked(func() {
		open = !open
		r.body.SetMarkup(messageMarkup(body(), h.names, vm.FromMe, mentionAccentFor(), h.searchQuery))
		more.SetLabel(readMoreLabel(open))
		if h.onExpandText != nil {
			h.onExpandText(id, open)
		}
	})
	r.setMore(more)
}

func (r *threadRow) fillFooter(msg client.Message, vm bubbleView, h bubbleHooks) {
	r.retry.SetVisible(vm.Failed)
	if vm.Failed {
		r.retryMsg, r.onRetry = msg, h.onRetry
		r.retry.SetSensitive(h.onRetry != nil)
	}
	r.timeLabel.SetLabel(vm.TimeText + vm.EditedMarker)
	r.clock.SetVisible(vm.Pending)
	switch {
	case vm.Pending:
		r.hideTick()
	case vm.Failed:
		r.tick.SetLabel("!")
		r.tick.SetTooltipText("Not sent")
		setCSSClass(r.tick, "chatot-bubble-failed", true)
		setCSSClass(r.tick, "chatot-bubble-tick", false)
		setCSSClass(r.tick, "chatot-tick-read", false)
		r.tick.SetVisible(true)
	case vm.FromMe && vm.TickText != "":
		r.tick.SetLabel(vm.TickText)
		r.tick.SetTooltipText("")
		setCSSClass(r.tick, "chatot-bubble-failed", false)
		setCSSClass(r.tick, "chatot-bubble-tick", true)
		setCSSClass(r.tick, "chatot-tick-read", vm.TickRead)
		r.tick.SetVisible(true)
	default:
		r.hideTick()
	}
}

// hideTick puts the badge/tick label back to how a row that never showed one
// keeps it: no text, no tooltip, none of the three classes. Leaving any of
// them behind would make a recycled row differ from a fresh one, and a read
// tick is exactly the sort of thing that must not survive onto someone
// else's message.
func (r *threadRow) hideTick() {
	r.tick.SetVisible(false)
	r.tick.SetLabel("")
	r.tick.SetTooltipText("")
	setCSSClass(r.tick, "chatot-bubble-failed", false)
	setCSSClass(r.tick, "chatot-bubble-tick", false)
	setCSSClass(r.tick, "chatot-tick-read", false)
}

// render shows msg in this row, reusing every widget that can be reused.
func (r *threadRow) render(msg client.Message, vm bubbleView, h bubbleHooks) {
	defer perfStart("renderRow")()

	r.band.SetVisible(true)
	r.typingSlot.SetVisible(false)
	// A row that was the typing sentinel keeps its dots otherwise, animating
	// out of sight for as long as the row lives.
	if r.typingSlot.FirstChild() != nil {
		removeAllChildren(r.typingSlot)
	}
	r.daySep.SetVisible(vm.ShowDaySeparator)
	if vm.ShowDaySeparator {
		r.daySep.SetLabel(vm.DayText)
	}
	r.unreadSep.SetVisible(vm.ShowUnreadSeparator)
	if vm.ShowUnreadSeparator {
		r.unreadSep.SetLabel(vm.UnreadText)
	}

	if vm.FromMe {
		r.row.SetHAlign(gtk.AlignEnd)
	} else {
		r.row.SetHAlign(gtk.AlignStart)
	}

	// A sticker or an emoji-only text renders bare — no bubble background or
	// padding, per the mockup — while still keeping the quote/footer/reactions
	// affordances every other bubble gets.
	isSticker := vm.IsMedia && vm.Media.Kind == "sticker"
	noChrome := isSticker || vm.IsEmojiOnly
	setCSSClass(r.bubble, "chatot-bubble-bare", noChrome)
	setCSSClass(r.bubble, "chatot-bubble", !noChrome)
	setCSSClass(r.bubble, "chatot-bubble-out", !noChrome && vm.FromMe)
	setCSSClass(r.bubble, "chatot-bubble-in", !noChrome && !vm.FromMe)

	r.fillBubble(msg, vm, h)
	r.renderReactions(msg, vm, h)
	r.renderAvatar(msg, vm, h)
	r.renderHover(msg, vm, h)
}

func (r *threadRow) renderReactions(msg client.Message, vm bubbleView, h bubbleHooks) {
	removeAllChildren(r.reactions)
	on := len(vm.Reactions) > 0
	setCSSClass(r.row, "chatot-row-reacted", on)
	r.reactions.SetVisible(on)
	if !on {
		return
	}
	// Same side as the hover pill: reactions hug the bubble's outer edge.
	if vm.FromMe {
		r.reactions.SetHAlign(gtk.AlignEnd)
	} else {
		r.reactions.SetHAlign(gtk.AlignStart)
	}
	for _, x := range vm.Reactions {
		r.reactions.Append(newReactionPill(x, msg, h))
	}
}

// renderAvatar fills the slot beside an incoming group bubble (WhatsApp's
// group thread; the mockup names the sender only), keeping the picture the
// slot already holds when it is still the right one.
func (r *threadRow) renderAvatar(msg client.Message, vm bubbleView, h bubbleHooks) {
	on := vm.Author != "" && !vm.FromMe && h.avatars != nil
	r.avatarSlot.SetVisible(on)
	if !on {
		if r.avatarSlot.FirstChild() != nil {
			removeAllChildren(r.avatarSlot)
			r.avatarJID, r.avatarInitial = "", ""
		}
		return
	}
	jid, initial := nonADJID(msg.FromJID), initialFor(vm.Author)
	gen := avatarGenOf(h, jid)
	if r.avatarSlot.FirstChild() != nil && r.avatarJID == jid && r.avatarInitial == initial && r.avatarGen == gen {
		return
	}
	r.setAvatar(h, jid, initial, gen)
}

// rebuildAvatar replaces the slot's picture without disturbing what the row is
// showing, for a sender who changed their picture while the row is on screen.
func (r *threadRow) rebuildAvatar(h bubbleHooks) {
	if r.avatarJID == "" || h.avatars == nil {
		return
	}
	r.setAvatar(h, r.avatarJID, r.avatarInitial, avatarGenOf(h, r.avatarJID))
}

func (r *threadRow) setAvatar(h bubbleHooks, jid, initial string, gen int) {
	removeAllChildren(r.avatarSlot)
	avatar := buildAvatar(h.c, h.avatars, jid, initial, bubbleAvatarSize)
	avatar.AddCSSClass("chatot-bubble-avatar")
	avatar.SetVAlign(gtk.AlignStart)
	r.avatarSlot.Append(avatar)
	r.avatarJID, r.avatarInitial, r.avatarGen = jid, initial, gen
}

func avatarGenOf(h bubbleHooks, jid string) int {
	if h.avatarGen == nil {
		return 0
	}
	return h.avatarGen(jid)
}

func (r *threadRow) renderHover(msg client.Message, vm bubbleView, h bubbleHooks) {
	// A tombstone, a message still going out and a failed one have nothing to
	// react to or act on, so the pair parks itself.
	if vm.Deleted || vm.Pending || vm.Failed {
		r.hover.park(r)
		return
	}
	// Editing is a text-only, own-message affordance (WhatsApp only edits
	// text). Anyone's message can be deleted for this account; the prompt
	// offers "for everyone" only on our own.
	canEdit := msg.FromMe && !vm.IsMedia && !vm.IsLocation && !vm.IsContact && !vm.IsPoll && !vm.IsEvent && !vm.IsCall
	r.hover.bind(r, msg, vm, h, canEdit, true)
}

// renderTyping puts the row into the "…" state the typing sentinel stands for:
// no separators, no bubble, no affordances, just the animated dots where a
// message would be.
func (r *threadRow) renderTyping() {
	r.daySep.SetVisible(false)
	r.unreadSep.SetVisible(false)
	r.band.SetVisible(false)
	r.hover.park(r)
	removeAllChildren(r.typingSlot)
	r.typingSlot.Append(newTypingBubble())
	r.typingSlot.SetVisible(true)
}

// hoverButtons is the 🙂/⌄ pair. Its widgets and its two clicked handlers are
// made once per row; a bind only repoints cur, so recycling never rewires a
// signal and never builds a button.
// hoverPlacement is where the pair currently sits. Reordering a GtkBox's
// children queues a resize on it, so the side is only reshuffled when the
// message actually changed sides — on a fling almost every bind leaves it
// where it was.
type hoverPlacement int

const (
	hoverUnplaced hoverPlacement = iota
	hoverParked
	hoverIncoming
	hoverOutgoing
)

type hoverButtons struct {
	box     *gtk.Box
	smiley  *gtk.Button
	chevron *gtk.Button
	// showing is set while a popover hangs off one of the buttons; see
	// bubbleAffordances.showing, which points here.
	showing bool
	// cur is what a click acts on now; nil means the pair is parked.
	cur    *bubbleAffordances
	placed hoverPlacement
	// compact is the mode the box is currently parented for; see place.
	compact bool
}

func newHoverButtons() *hoverButtons {
	hb := &hoverButtons{
		box:     gtk.NewBox(gtk.OrientationHorizontal, 2),
		smiley:  gtk.NewButtonWithLabel("🙂"),
		chevron: gtk.NewButton(),
	}
	hb.box.AddCSSClass("chatot-hover-actions")
	hb.box.SetVAlign(gtk.AlignCenter)
	// Hidden by opacity, not visibility: the buttons keep their room, so a
	// bubble at full width neither shrinks nor re-wraps when they appear.
	// Hiding them outright buys nothing either way — GTK caches their size
	// request, so a bind never re-measures them (BenchmarkRowRenderAndMeasure).
	hb.box.SetOpacity(0)
	hb.box.SetCanTarget(false)

	hb.smiley.AddCSSClass("chatot-hover-btn")
	hb.smiley.SetTooltipText("React")
	// Pointer affordances: not in the Tab chain, where they would be reached
	// while invisible.
	hb.smiley.SetFocusable(false)

	hb.chevron.SetChild(newChevronGlyph(14))
	hb.chevron.AddCSSClass("chatot-hover-btn")
	hb.chevron.AddCSSClass("chatot-hover-chevron")
	hb.chevron.SetTooltipText("Message options")
	hb.chevron.SetFocusable(false)

	hb.box.Append(hb.smiley)
	hb.box.Append(hb.chevron)

	hb.smiley.ConnectClicked(func() {
		if hb.cur != nil {
			hb.cur.openReact()
		}
	})
	hb.chevron.ConnectClicked(func() {
		if hb.cur != nil {
			hb.cur.openMenu()
		}
	})
	return hb
}

// bind points the pair at msg and puts it on the side away from the margin:
// the 🙂 and ⌄ sit just outside the bubble, packed after an incoming bubble
// and before an outgoing one, so the bubble never moves when they appear.
func (hb *hoverButtons) bind(r *threadRow, msg client.Message, vm bubbleView, h bubbleHooks, canEdit, canDelete bool) {
	hb.box.SetVisible(true)
	want := hoverIncoming
	if vm.FromMe {
		want = hoverOutgoing
	}
	hb.place(r, want)
	hb.smiley.SetSensitive(h.onReact != nil)

	a := bubbleAffordances{
		bubble:  r.bubble,
		host:    h.host,
		fromMe:  vm.FromMe,
		actions: hb.box,
		chevron: hb.chevron,
		smiley:  hb.smiley,
		showing: &hb.showing,
	}
	a.openReact = func() {
		at := alignedRect(a.host, hb.smiley, a.bubble, bubbleMenuWidth, a.fromMe)
		pop := a.popoverAt(at, gtk.PosTop)
		pop.SetChild(buildReactRow(a.bubble, msg, h, pop, at))
		pop.Popup()
	}
	a.openMenu = func() {
		pop := a.popover(hb.chevron, gtk.PosBottom, bubbleMenuWidth)
		// Actions only: the mockup's ⋯ menu carries no reaction row, that is
		// what the 🙂 button beside it is for.
		pop.SetChild(buildMenuBox(h.menuItemsFor(msg, canEdit, canDelete), pop))
		pop.Popup()
	}
	hb.cur = &a
	shotRegister(msg.ID, func(s *bubbleShot) { s.affordances = a })
}

// park takes the pair out of a row that must not offer it, and puts it back
// where a never-bound row keeps it. Without the reorder a recycled row would
// leave the hidden box on whichever side the previous message put it, so the
// next bind that parks again would differ from a fresh row.
func (hb *hoverButtons) park(r *threadRow) {
	hb.cur = nil
	hb.box.SetOpacity(0)
	hb.box.SetCanTarget(false)
	hb.box.SetVisible(false)
	hb.place(r, hoverParked)
}

// place puts the pair where want says, in the row's current mode. Side by
// side (the wide layout) the box is the bubble's sibling in r.row, on the
// side away from the margin, and it keeps its 58px even while hidden so a
// bubble never shifts when the buttons come up. Compact (the collapsed
// window) that reservation would push a 280px card past a 360px pane, so
// the box floats over the bubble's top corner instead, the way the
// mockup's hover bar does, and takes no room in the row.
func (hb *hoverButtons) place(r *threadRow, want hoverPlacement) {
	if hb.compact != r.compact {
		if r.compact {
			r.row.Remove(hb.box)
			hb.box.SetVAlign(gtk.AlignStart)
			hb.box.AddCSSClass("chatot-hover-float")
			r.stack.AddOverlay(hb.box)
		} else {
			r.stack.RemoveOverlay(hb.box)
			hb.box.RemoveCSSClass("chatot-hover-float")
			// Centre, as built: Fill would stretch the pair to the row's
			// height, and the compact mode set Start.
			hb.box.SetVAlign(gtk.AlignCenter)
			hb.box.SetHAlign(gtk.AlignFill)
			r.row.Append(hb.box)
		}
		hb.compact = r.compact
		hb.placed = hoverUnplaced
	}
	if hb.placed == want {
		return
	}
	outgoing := want == hoverOutgoing
	switch {
	case hb.compact && outgoing:
		hb.box.SetHAlign(gtk.AlignStart)
	case hb.compact:
		hb.box.SetHAlign(gtk.AlignEnd)
	case outgoing:
		r.row.ReorderChildAfter(hb.box, r.avatarSlot)
	default:
		r.row.ReorderChildAfter(hb.box, r.stack)
	}
	if outgoing {
		hb.box.ReorderChildAfter(hb.smiley, hb.chevron)
	} else {
		hb.box.ReorderChildAfter(hb.chevron, hb.smiley)
	}
	hb.placed = want
}

// setCompact switches the row between the side-by-side and floating hover
// layouts; see hoverButtons.place.
func (r *threadRow) setCompact(on bool) {
	if r.compact == on {
		return
	}
	r.compact = on
	r.hover.place(r, r.hover.placed)
}

func (hb *hoverButtons) setVisible(on bool) {
	if hb.cur == nil {
		return
	}
	hb.cur.setVisible(on)
}
