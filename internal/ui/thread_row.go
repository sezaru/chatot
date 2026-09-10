package ui

import (
	"chatot/internal/client"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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
	}

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

	removeAllChildren(r.bubble)
	fillBubble(r.bubble, msg, vm, h)
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
type hoverButtons struct {
	box     *gtk.Box
	smiley  *gtk.Button
	chevron *gtk.Button
	// showing is set while a popover hangs off one of the buttons; see
	// bubbleAffordances.showing, which points here.
	showing bool
	// cur is what a click acts on now; nil means the pair is parked.
	cur *bubbleAffordances
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
	if vm.FromMe {
		r.row.ReorderChildAfter(hb.box, r.avatarSlot)
		hb.box.ReorderChildAfter(hb.smiley, hb.chevron)
	} else {
		r.row.ReorderChildAfter(hb.box, r.stack)
		hb.box.ReorderChildAfter(hb.chevron, hb.smiley)
	}
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
	r.row.ReorderChildAfter(hb.box, r.avatarSlot)
	hb.box.ReorderChildAfter(hb.chevron, hb.smiley)
}

func (hb *hoverButtons) setVisible(on bool) {
	if hb.cur == nil {
		return
	}
	hb.cur.setVisible(on)
}
