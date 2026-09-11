package ui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// forwardDialogAvatarSize matches chatRowAvatarSize; the forward picker's
// rows are a smaller version of the chat-list rows.
const forwardDialogAvatarSize = 32

// ForwardInitialPick pre-ticks these chat JIDs when the dialog opens; a
// screenshot hook, so the ticked state can be captured without clicks.
var ForwardInitialPick []string

// forwardSelectionLabel renders the dialog's footer for n selected chats.
func forwardSelectionLabel(n int) string {
	if n == 0 {
		return "Pick chats to forward to"
	}
	return fmt.Sprintf("%d selected", n)
}

// filterForwardChats returns the chats whose name contains query
// case-insensitively (query "" matches everything), preserving order.
func filterForwardChats(chats []client.Chat, query string) []client.Chat {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return chats
	}
	out := make([]client.Chat, 0, len(chats))
	for _, c := range chats {
		if strings.Contains(strings.ToLower(c.Name), query) {
			out = append(out, c)
		}
	}
	return out
}

// forwardItem is one row of the picker's model.
type forwardItem struct {
	chat client.Chat
	vm   chatRowView
}

var forwardModelType = gioutil.NewListModelType[forwardItem]()

// forwardRowWidget is one row of the picker, reused across the chats it
// scrolls through: the avatar slot, the name and the tick are rebound, and
// the tick is redrawn in place on a click.
type forwardRowWidget struct {
	root   *gtk.Button
	avatar *gtk.Box
	name   *gtk.Label
	check  *gtk.DrawingArea
	jid    string
	picked bool
}

func newForwardRowWidget() *forwardRowWidget {
	w := &forwardRowWidget{}
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	w.avatar = gtk.NewBox(gtk.OrientationVertical, 0)
	w.avatar.SetSizeRequest(forwardDialogAvatarSize, forwardDialogAvatarSize)
	row.Append(w.avatar)

	w.name = gtk.NewLabel("")
	w.name.SetXAlign(0)
	w.name.SetHExpand(true)
	w.name.SetEllipsize(pango.EllipsizeEnd)
	w.name.AddCSSClass("chatot-forward-name")
	row.Append(w.name)

	// A round tick disc, not a square GtkCheckButton: the design's forward
	// list uses the same 19px check as its other pickers.
	w.check = gtk.NewDrawingArea()
	w.check.SetSizeRequest(19, 19)
	w.check.SetHAlign(gtk.AlignCenter)
	w.check.SetVAlign(gtk.AlignCenter)
	w.check.AddCSSClass("chatot-forward-check")
	w.check.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		if !w.picked {
			return
		}
		col := area.Color()
		cr.SetSourceRGBA(float64(col.Red()), float64(col.Green()), float64(col.Blue()), float64(col.Alpha()))
		drawCheck(cr, float64(width), float64(height))
	})
	row.Append(w.check)

	w.root = gtk.NewButton()
	w.root.SetChild(row)
	w.root.AddCSSClass("flat")
	w.root.AddCSSClass("chatot-people-row")
	return w
}

// bind points the row at it: name, avatar and whether it is ticked.
func (w *forwardRowWidget) bind(c client.Client, cache *avatarCache, it forwardItem, picked bool) {
	w.jid = it.chat.JID
	w.name.SetText(it.vm.Name)
	removeAllChildren(w.avatar)
	w.avatar.Append(buildAvatar(c, cache, w.jid, it.vm.Initial, forwardDialogAvatarSize))
	w.setPicked(picked)
}

// setPicked draws the tick on or off.
func (w *forwardRowWidget) setPicked(on bool) {
	w.picked = on
	if on {
		w.check.AddCSSClass("chatot-forward-check-on")
	} else {
		w.check.RemoveCSSClass("chatot-forward-check-on")
	}
	w.check.QueueDraw()
}

// ShowForwardDialog opens the "Forward to" picker for msg: a searchable
// multi-select list of the user's chats and a Send button that dispatches
// ForwardMessage to every checked chat in the background, reporting the
// outcome via toastOverlay (may be nil). cache is the caller's avatar
// memo, normally the chat list's, so the rows show the pictures it already
// has without asking again; nil makes a private one.
func ShowForwardDialog(parent *gtk.Window, c client.Client, cache *avatarCache, msg client.Message, toastOverlay *adw.ToastOverlay) {
	t0 := time.Now()
	chats, err := c.Chats(0)
	if err != nil {
		log.Printf("chatot: forward: load chats failed: %v", err)
		return
	}

	dialog := newCardDialog()
	dialog.SetTitle("Forward to…")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(360, 480)

	box := gtk.NewBox(gtk.OrientationVertical, 0)

	// The mockup drops the quoted-message preview: the ⋯ menu you came from
	// already showed which message this is, and the row list needs the height.
	search := sidebarSearchEntry("Search chats")
	searchRow := gtk.NewBox(gtk.OrientationVertical, 0)
	searchRow.AddCSSClass("chatot-forward-search")
	searchRow.Append(search)
	box.Append(searchRow)

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetMinContentHeight(0)
	scroller.SetSizeRequest(-1, 120)
	box.Append(scroller)

	// A single footer bar, per the mockup: the count at the left, one green
	// Send at the right. Closing is the title row's ✕.
	btnRow := gtk.NewBox(gtk.OrientationHorizontal, 10)
	btnRow.AddCSSClass("chatot-dialog-footer")
	footer := gtk.NewLabel(forwardSelectionLabel(0))
	footer.SetXAlign(0)
	footer.SetHExpand(true)
	footer.SetVAlign(gtk.AlignCenter)
	footer.AddCSSClass("chatot-card-value")
	btnRow.Append(footer)
	forwardBtn := gtk.NewButtonWithLabel("Send")
	forwardBtn.AddCSSClass("chatot-primary-btn")
	forwardBtn.SetSensitive(false)
	btnRow.Append(forwardBtn)
	box.Append(btnRow)

	selected := make(map[string]bool)
	for _, jid := range ForwardInitialPick {
		selected[jid] = true
	}
	if cache == nil {
		cache = newAvatarCache()
	}

	updateFooter := func() {
		footer.SetText(forwardSelectionLabel(len(selected)))
		forwardBtn.SetSensitive(len(selected) > 0)
	}

	// The rows are a GtkListView over model: a widget exists only for a
	// row on screen, a tick flips in place, and a search refills the model
	// rather than the widgets. Building a widget per chat, and again on
	// every click, was what made a 500-chat account's picker open late and
	// answer a tap late.
	model := forwardModelType.New()
	fill := func(query string) {
		shown := filterForwardChats(chats, query)
		items := make([]forwardItem, len(shown))
		now := time.Now()
		for i, chat := range shown {
			items[i] = forwardItem{chat: chat, vm: chatRowVM(chat, now)}
		}
		model.Splice(0, model.Len(), items...)
	}
	rows := map[uintptr]*forwardRowWidget{}
	widgetOf := func(item *gtk.ListItem) *forwardRowWidget {
		child := item.Child()
		if child == nil {
			return nil
		}
		return rows[widgetKey(child)]
	}
	factory := gtk.NewSignalListItemFactory()
	factory.ConnectSetup(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		w := newForwardRowWidget()
		w.root.ConnectClicked(func() {
			if w.jid == "" {
				return
			}
			if selected[w.jid] {
				delete(selected, w.jid)
			} else {
				selected[w.jid] = true
			}
			w.setPicked(selected[w.jid])
			updateFooter()
		})
		rows[widgetKey(w.root)] = w
		item.SetChild(w.root)
	})
	factory.ConnectBind(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		w := widgetOf(item)
		pos := int(item.Position())
		if w == nil || pos < 0 || pos >= model.Len() {
			return
		}
		it := model.At(pos)
		w.bind(c, cache, it, selected[it.chat.JID])
	})
	factory.ConnectTeardown(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		if w := widgetOf(item); w != nil {
			delete(rows, widgetKey(w.root))
		}
	})
	list := gtk.NewListView(gtk.NewNoSelection(model), &factory.ListItemFactory)
	list.AddCSSClass("chatot-forward-list")
	list.SetVExpand(true)
	scroller.SetChild(list)
	smoothWheel(scroller)

	fill("")
	updateFooter()
	search.ConnectSearchChanged(func() { fill(search.Text()) })

	forwardBtn.ConnectClicked(func() {
		targets := make([]string, 0, len(selected))
		for jid := range selected {
			targets = append(targets, jid)
		}
		dialog.Close()
		if len(targets) == 0 {
			return
		}
		go func() {
			ok := 0
			for _, jid := range targets {
				if _, err := c.ForwardMessage(context.Background(), msg, jid); err != nil {
					log.Printf("chatot: forward to %s failed: %v", jid, err)
					continue
				}
				ok++
			}
			if toastOverlay == nil {
				return
			}
			glib.IdleAdd(func() {
				toastOverlay.AddToast(adw.NewToast(fmt.Sprintf("Forwarded to %d chat(s)", ok)))
			})
		}()
	})

	dialog.SetChild(box)
	dialog.SetDefaultWidget(forwardBtn)
	dialog.Present()
	trace(1, "forward dialog: %d chats, up in %s", len(chats), time.Since(t0).Round(time.Millisecond))
}
