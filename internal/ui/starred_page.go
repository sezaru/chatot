package ui

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// starredCountLabel is the dim count beside the page title, e.g. "1 starred".
func starredCountLabel(n int) string {
	return fmt.Sprintf("%d starred", n)
}

// StarredPage is the mockup's right-pane starred-messages list: a ← header
// with the count, then one row per starred message (avatar, chat name over
// the text, time and an unstar ⭐).
//
// It replaces a sidebar mode that could only say "No starred messages" — the
// design puts starred in the content pane, where a row has room for its chat,
// its text and its time.
type StarredPage struct {
	*gtk.Box

	c     client.Client
	cache *avatarCache

	count *gtk.Label
	list  *gtk.Box

	onOpen func(jid string)
}

// NewStarredPage builds the page. onBack closes it; onOpen jumps to a chat.
func NewStarredPage(c client.Client, onBack func(), onOpen func(jid string)) *StarredPage {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	p := &StarredPage{Box: root, c: c, cache: newAvatarCache(), onOpen: onOpen}

	header := gtk.NewBox(gtk.OrientationHorizontal, 10)
	header.AddCSSClass("chatot-pane-header")

	back := gtk.NewButtonWithLabel("←")
	back.AddCSSClass("flat")
	back.AddCSSClass("chatot-pane-back")
	back.ConnectClicked(onBack)
	header.Append(back)

	title := gtk.NewLabel("Starred messages")
	title.SetXAlign(0)
	title.SetHExpand(true)
	title.AddCSSClass("chatot-pane-title")
	header.Append(title)

	p.count = gtk.NewLabel("")
	p.count.AddCSSClass("chatot-pane-count")
	header.Append(p.count)
	root.Append(header)

	p.list = gtk.NewBox(gtk.OrientationVertical, 0)
	p.list.AddCSSClass("chatot-pane-body")

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetChild(p.list)
	root.Append(scroller)

	return p
}

// Reload re-reads the starred messages from the store and rebuilds the list.
// Called each time the page is shown, so an unstar elsewhere can't leave a
// stale row behind.
func (p *StarredPage) Reload() {
	removeAllChildren(p.list)

	msgs, err := p.c.StarredMessages(200)
	if err != nil {
		log.Printf("chatot: load starred messages failed: %v", err)
	}
	p.count.SetLabel(starredCountLabel(len(msgs)))
	if len(msgs) == 0 {
		p.list.Append(newStarredEmptyState())
		return
	}

	names := chatNames(p.c)
	for _, msg := range msgs {
		p.list.Append(p.newStarredRow(msg, names[msg.ChatJID]))
	}
}

// chatNames maps every known chat's JID to its display name, for labelling
// starred rows with the conversation they came from.
func chatNames(c client.Client) map[string]string {
	names := map[string]string{}
	chats, err := c.Chats(0)
	if err != nil {
		return names
	}
	for _, chat := range chats {
		names[chat.JID] = chat.Name
	}
	return names
}

// newStarredEmptyState is the mockup's centred ⭐ disc with its two lines of
// explanation, replacing a bare italic "No starred messages".
func newStarredEmptyState() gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.AddCSSClass("chatot-pane-empty")
	box.SetVAlign(gtk.AlignCenter)
	box.SetHAlign(gtk.AlignCenter)

	disc := gtk.NewLabel("⭐")
	disc.AddCSSClass("chatot-pane-empty-disc")
	disc.SetSizeRequest(56, 56)
	disc.SetHAlign(gtk.AlignCenter)
	box.Append(disc)

	title := gtk.NewLabel("No starred messages")
	title.AddCSSClass("chatot-pane-empty-title")
	box.Append(title)

	hint := gtk.NewLabel("Star a message from its ⋯ menu to keep it here.")
	hint.AddCSSClass("chatot-pane-empty-hint")
	hint.SetWrap(true)
	hint.SetJustify(gtk.JustifyCenter)
	hint.SetMaxWidthChars(34)
	box.Append(hint)
	return box
}

// newStarredRow builds one row: avatar, chat name over the message text, then
// the time and an unstar button.
func (p *StarredPage) newStarredRow(msg client.Message, chatName string) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)

	if chatName == "" {
		chatName = msg.ChatJID
	}
	row.Append(buildAvatar(p.c, p.cache, msg.ChatJID, contactInitial(chatName), 32))

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(chatName)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.AddCSSClass("chatot-starred-chat")
	col.Append(name)
	text := gtk.NewLabel(starredSnippet(msg))
	text.SetXAlign(0)
	text.SetEllipsize(pango.EllipsizeEnd)
	text.AddCSSClass("chatot-starred-text")
	col.Append(text)
	row.Append(col)

	tail := gtk.NewBox(gtk.OrientationHorizontal, 6)
	tail.SetVAlign(gtk.AlignCenter)
	when := gtk.NewLabel(formatChatTime(msg.TS, time.Now()))
	when.AddCSSClass("chatot-starred-time")
	tail.Append(when)

	unstar := gtk.NewButtonWithLabel("⭐")
	unstar.AddCSSClass("flat")
	unstar.AddCSSClass("chatot-starred-unstar")
	unstar.SetTooltipText("Unstar")
	target := msg
	unstar.ConnectClicked(func() {
		go func() {
			if err := p.c.StarMessage(context.Background(), target.ChatJID, target.ID, false); err != nil {
				log.Printf("chatot: unstar message failed: %v", err)
				return
			}
			glib.IdleAdd(p.Reload)
		}()
	})
	tail.Append(unstar)
	row.Append(tail)

	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-pane-row")
	jid := msg.ChatJID
	btn.ConnectClicked(func() {
		if p.onOpen != nil {
			p.onOpen(jid)
		}
	})
	return btn
}
