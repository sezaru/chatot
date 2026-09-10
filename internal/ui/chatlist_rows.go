package ui

import (
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// The sidebar's chat rows are a GtkListView over a list model of row
// view-models. Only the rows near the viewport exist as widgets, so
// scrolling 500 chats lays out and paints what scrolling 20 does (a
// GtkListBox realized every row: 5-9 ms of each frame on a 553-chat
// store), and a refresh touches only the model entries that changed; the
// view rebinds those rows, and a rebind updates the row's labels in place.
// Rows are 53 px whatever they show, so the view's height estimate is
// exact and nothing jumps.

// What the chat model holds; the rows are only kept across refreshes of
// the same kind. listOther means the ListBox page (search hits, the empty
// state) is showing instead.
const (
	listOther  = ""
	listChats  = "chats"
	listMerged = "merged"
)

// chatRowItem is one model entry: the row's identity, its view-model, the
// chat behind it and the client that answers for it (its own account's in
// merged mode). avatarGen changes when the chat's picture did, so the row
// swaps it in on rebind.
type chatRowItem struct {
	key       string
	vm        chatRowView
	chat      client.Chat
	client    client.Client
	avatarGen int
}

var chatRowModelType = gioutil.NewListModelType[chatRowItem]()

// wantRow is one row the list should show.
type wantRow struct {
	key    string
	vm     chatRowView
	chat   client.Chat
	client client.Client
}

// clearChatModel empties the chat model.
func (cl *ChatList) clearChatModel() {
	if n := cl.chatModel.Len(); n > 0 {
		cl.chatModel.Splice(0, n)
	}
}

// resetList shows the ListBox page, emptied, and forgets the chat rows.
// Every non-chat filler (search hits, the empty state) goes through here.
func (cl *ChatList) resetList() {
	cl.list.RemoveAll()
	cl.clearChatModel()
	cl.listKind = listOther
	cl.listStack.SetVisibleChildName("other")
}

// reconcileRows makes the chat model hold exactly want, in order, leaving
// every entry whose view-model is unchanged alone (its row keeps its
// widget and its place). kind guards against reusing rows built for
// another mode. Must run on the GTK main loop.
func (cl *ChatList) reconcileRows(kind string, want []wantRow) {
	if len(want) == 0 {
		cl.resetList()
		cl.list.Append(cl.newListEmptyState())
		return
	}
	if cl.listKind != kind {
		cl.list.RemoveAll()
		cl.clearChatModel()
		cl.listKind = kind
	}
	cl.listStack.SetVisibleChildName("chats")
	// Read before the model changes: the value only moves at the next
	// layout, so this is where the reader was.
	atTop := cl.listAtTop()
	defer func() {
		if atTop {
			cl.scrollListToTop()
		}
	}()

	m := cl.chatModel
	wanted := make(map[string]bool, len(want))
	for _, w := range want {
		wanted[w.key] = true
	}
	// Rows no longer wanted go first, from the end so positions hold.
	for i := m.Len() - 1; i >= 0; i-- {
		if !wanted[m.At(i).key] {
			m.Remove(i)
		}
	}
	for i, w := range want {
		item := chatRowItem{key: w.key, vm: w.vm, chat: w.chat, client: w.client, avatarGen: cl.avatarGens[w.vm.JID]}
		if i < m.Len() && m.At(i).key == w.key {
			cur := m.At(i)
			if cur.vm != item.vm || cur.avatarGen != item.avatarGen || cur.client != item.client {
				m.Splice(i, 1, item)
			}
			continue
		}
		// The row sits further down (it moved up), or it is new.
		for j := i + 1; j < m.Len(); j++ {
			if m.At(j).key == w.key {
				m.Remove(j)
				break
			}
		}
		m.Splice(i, 0, item)
	}
}

// Following the head of the chat list, the way the thread follows its
// foot (thread_scroll.go): a reader at the very top sees a chat that just
// got a message move up into first place; one who has scrolled down is
// left where they are. GtkListView anchors the row at the top of the
// viewport across model changes, so on its own a row spliced in above
// that anchor lands out of view and the list appears to have scrolled
// down one row.

// listAtTop reports whether the chat list is scrolled to its first row.
func (cl *ChatList) listAtTop() bool {
	return cl.listScroller.VAdjustment().Value() < 1
}

// scrollListToTop makes the first row the list's anchor, so the layout
// after a reorder keeps the head in view instead of the old first row.
func (cl *ChatList) scrollListToTop() {
	if cl.chatModel.Len() == 0 {
		return
	}
	cl.listScroller.VAdjustment().SetValue(0)
	cl.chatView.ScrollTo(0, gtk.ListScrollNone, nil)
}

// invalidateRow forces jid's row(s) to rebind with a fresh avatar on the
// next refresh, for a change the view-model doesn't carry (a new picture).
func (cl *ChatList) invalidateRow(jid string) {
	cl.avatarGens[jid]++
}

// rowVM is the view-model of jid's row, if the model has one (dev hooks).
func (cl *ChatList) rowVM(jid string) (chatRowView, bool) {
	for i := 0; i < cl.chatModel.Len(); i++ {
		if it := cl.chatModel.At(i); it.vm.JID == jid {
			return it.vm, true
		}
	}
	return chatRowView{}, false
}

// rowsInOrder reports whether the model lines up with rowJIDs (dev hooks).
func (cl *ChatList) rowsInOrder() bool {
	if cl.listKind != listChats {
		return true
	}
	if cl.chatModel.Len() != len(cl.rowJIDs) {
		return false
	}
	for i, jid := range cl.rowJIDs {
		if cl.chatModel.At(i).vm.JID != jid {
			return false
		}
	}
	return true
}

// newChatView builds the list view over cl.chatModel and its row factory.
// Setup builds a row's widgets once; bind fills them from the model entry
// at the row's position and teardown forgets them.
func (cl *ChatList) newChatView() *gtk.ListView {
	widgetOf := func(item *gtk.ListItem) *chatRowWidget {
		child := item.Child()
		if child == nil {
			return nil
		}
		return cl.rowWidgets[widgetKey(child)]
	}
	factory := gtk.NewSignalListItemFactory()
	factory.ConnectSetup(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		w := cl.newChatRowWidget()
		cl.rowWidgets[widgetKey(w.root)] = w
		item.SetChild(w.root)
	})
	factory.ConnectBind(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		w := widgetOf(item)
		pos := int(item.Position())
		if w == nil || pos < 0 || pos >= cl.chatModel.Len() {
			return
		}
		trace(2, "bind chat row %d", pos)
		w.bind(cl.chatModel.At(pos), cl.avatarCache)
		cl.boundRows[w.key] = w
	})
	factory.ConnectUnbind(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		if w := widgetOf(item); w != nil && cl.boundRows[w.key] == w {
			delete(cl.boundRows, w.key)
		}
	})
	factory.ConnectTeardown(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		if w := widgetOf(item); w != nil {
			delete(cl.rowWidgets, widgetKey(w.root))
		}
	})
	lv := gtk.NewListView(cl.chatSel, &factory.ListItemFactory)
	lv.AddCSSClass("navigation-sidebar")
	lv.SetVExpand(true)
	return lv
}

// chatRowAvatarSize is the chat-list row avatar's fixed square size in px.
const chatRowAvatarSize = 38

// chatRowTimeClass returns the extra CSS class the row's timestamp carries,
// or "" for none. The mockup renders an unread chat's timestamp in accent
// green at full opacity instead of the usual dim grey.
func chatRowTimeClass(showUnread bool) string {
	if showUnread {
		return "chatot-chat-time-unread"
	}
	return ""
}

// chatRowWidget is one chat row's widgets, built once per list item and
// refilled from a chatRowItem on every bind. Everything a row may show
// (stripe, flags, badge) exists from the start and is shown or hidden.
type chatRowWidget struct {
	root       *gtk.Box
	stripe     *gtk.Box
	avatarSlot *gtk.Box
	name       *gtk.Label
	preview    *gtk.Label
	// previewFull is the untruncated preview, shown as a tooltip when the
	// label ellipsizes it. Kept here because the row widget is recycled and
	// the tooltip is asked for long after the text was set.
	previewFull string
	time        *gtk.Label
	flags       [3]*gtk.Label // pinned, muted, blocked
	badge       *gtk.Label

	key         string
	chat        client.Chat
	stripeClass string
	timeClass   string
	// What the avatar in avatarSlot was built for.
	avatarJID     string
	avatarInitial string
	avatarClient  client.Client
	avatarGen     int
}

// newChatRowWidget builds an empty row. The avatar renders the initial
// immediately and swaps in the real picture asynchronously (buildAvatar).
func (cl *ChatList) newChatRowWidget() *chatRowWidget {
	w := &chatRowWidget{}
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	// Mockup padding: 7px vertical, 8px horizontal, giving a 53px row around
	// the 38px avatar. The list item around this box contributes none.
	row.SetMarginTop(7)
	row.SetMarginBottom(7)
	row.SetMarginStart(8)
	row.SetMarginEnd(8)
	w.root = row

	// Merged mode only: a 3px account-coloured stripe at the row's leading
	// edge, so two chats from different accounts are told apart at a glance.
	w.stripe = gtk.NewBox(gtk.OrientationVertical, 0)
	w.stripe.AddCSSClass("chatot-row-stripe")
	w.stripe.SetSizeRequest(3, chatRowAvatarSize)
	w.stripe.SetVAlign(gtk.AlignCenter)
	w.stripe.SetVisible(false)
	row.Append(w.stripe)

	w.avatarSlot = gtk.NewBox(gtk.OrientationVertical, 0)
	w.avatarSlot.SetVAlign(gtk.AlignCenter)
	row.Append(w.avatarSlot)

	textCol := gtk.NewBox(gtk.OrientationVertical, 2)
	textCol.SetHExpand(true)

	w.name = gtk.NewLabel("")
	w.name.SetXAlign(0)
	w.name.SetEllipsize(pango.EllipsizeEnd)
	w.name.SetMaxWidthChars(1)
	w.name.SetHExpand(true)
	w.name.AddCSSClass("chatot-chat-name")
	textCol.Append(w.name)

	// Single line, ellipsized. MaxWidthChars(1) keeps the label's natural
	// width tiny so a long message can't stretch the row wider than the
	// sidebar; HExpand lets it fill whatever width the sidebar does give.
	// SingleLineMode as well as Ellipsize: Pango ellipsizes per line, so a
	// preview holding a newline still rendered as two lines and grew the row.
	w.preview = gtk.NewLabel("")
	w.preview.SetXAlign(0)
	w.preview.SetSingleLineMode(true)
	w.preview.SetEllipsize(pango.EllipsizeEnd)
	w.preview.SetMaxWidthChars(1)
	w.preview.SetHExpand(true)
	w.preview.AddCSSClass("chatot-chat-preview")
	// The row is a single line, so a long message is cut with an ellipsis.
	// Hovering the cut text shows all of it. A preview that fits gets no
	// tooltip, which is why this asks the layout whether it actually
	// ellipsized rather than guessing from the length.
	w.preview.SetHasTooltip(true)
	w.preview.ConnectQueryTooltip(func(x, y int, keyboard bool, tip *gtk.Tooltip) bool {
		text, show := previewTooltip(w.previewFull, w.preview.Layout().IsEllipsized())
		if !show {
			return false
		}
		tip.SetText(text)
		return true
	})
	textCol.Append(w.preview)

	row.Append(textCol)

	metaCol := gtk.NewBox(gtk.OrientationVertical, 4)
	metaCol.SetVAlign(gtk.AlignStart)

	// Mockup: pin/mute/block are small glyphs beside the timestamp on the
	// right, never prefixes on the chat name.
	metaTop := gtk.NewBox(gtk.OrientationHorizontal, 3)
	metaTop.SetHAlign(gtk.AlignEnd)
	for i, glyph := range [...]string{"📌", "🔇", "🚫"} {
		flag := gtk.NewLabel(glyph)
		flag.AddCSSClass("chatot-chat-flag")
		flag.SetVisible(false)
		metaTop.Append(flag)
		w.flags[i] = flag
	}
	w.time = gtk.NewLabel("")
	w.time.AddCSSClass("chatot-chat-time")
	metaTop.Append(w.time)
	metaCol.Append(metaTop)

	w.badge = gtk.NewLabel("")
	w.badge.AddCSSClass("chatot-unread-badge")
	w.badge.SetHAlign(gtk.AlignEnd)
	w.badge.SetVisible(false)
	metaCol.Append(w.badge)

	row.Append(metaCol)

	// The right-click menu reads the chat bound at the time of the click.
	gesture := gtk.NewGestureClick()
	gesture.SetButton(gdk.BUTTON_SECONDARY)
	gesture.ConnectPressed(func(_ int, x, y float64) {
		showChatContextMenu(cl, row, cl.rowMenuItems(w.chat), x, y)
	})
	row.AddController(gesture)
	return w
}

// bind fills the row from item. The avatar is rebuilt only when the chat,
// its initial, its account or its picture changed: a rebind for a new
// preview keeps the picture already showing.
func (w *chatRowWidget) bind(item chatRowItem, cache *avatarCache) {
	vm := item.vm
	w.key, w.chat = item.key, item.chat

	if vm.AccountColor != w.stripeClass {
		if w.stripeClass != "" {
			w.stripe.RemoveCSSClass(w.stripeClass)
		}
		if vm.AccountColor != "" {
			w.stripe.AddCSSClass(vm.AccountColor)
		}
		w.stripeClass = vm.AccountColor
	}
	w.stripe.SetVisible(vm.AccountColor != "")

	if w.avatarSlot.FirstChild() == nil || w.avatarJID != vm.JID || w.avatarInitial != vm.Initial ||
		w.avatarClient != item.client || w.avatarGen != item.avatarGen {
		removeAllChildren(w.avatarSlot)
		w.avatarSlot.Append(buildAvatar(item.client, cache, vm.JID, vm.Initial, chatRowAvatarSize))
		w.avatarJID, w.avatarInitial, w.avatarClient, w.avatarGen = vm.JID, vm.Initial, item.client, item.avatarGen
	}

	w.name.SetText(vm.Name)
	previewText := vm.Preview
	if !ShowMessagePreviews && !vm.Typing {
		previewText = ""
	}
	w.preview.SetText(previewText)
	w.previewFull = previewText
	setCSSClass(w.preview, "chatot-chat-typing", vm.Typing)

	for i, on := range [...]bool{vm.Pinned, vm.Muted, vm.Blocked} {
		w.flags[i].SetVisible(on)
	}
	w.time.SetText(vm.TimeText)
	if cls := chatRowTimeClass(vm.ShowUnread); cls != w.timeClass {
		if w.timeClass != "" {
			w.time.RemoveCSSClass(w.timeClass)
		}
		if cls != "" {
			w.time.AddCSSClass(cls)
		}
		w.timeClass = cls
	}
	w.badge.SetText(vm.UnreadText)
	w.badge.SetVisible(vm.ShowUnread)
}

// previewTooltip decides what a chat row's preview shows on hover: the whole
// message when the row had to cut it, and nothing at all when it fits, since
// a tooltip repeating text already on screen is just noise. ellipsized comes
// from the label's own layout rather than a length guess, because how much
// fits depends on the sidebar width and the glyphs.
func previewTooltip(full string, ellipsized bool) (string, bool) {
	if full == "" || !ellipsized {
		return "", false
	}
	return full, true
}

// setCSSClass adds or removes class on w.
func setCSSClass(w gtk.Widgetter, class string, on bool) {
	if on {
		gtk.BaseWidget(w).AddCSSClass(class)
	} else {
		gtk.BaseWidget(w).RemoveCSSClass(class)
	}
}

// widgetKey identifies a widget by its object pointer, for the row maps.
func widgetKey(w gtk.Widgetter) uintptr { return glib.BaseObject(w).Native() }
