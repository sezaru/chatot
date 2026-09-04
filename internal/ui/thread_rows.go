package ui

import (
	"chatot/internal/client"

	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The thread is a GtkListView over a list model that mirrors cv.msgs, the
// way Fractal, Paper Plane and Flare build theirs. GTK realizes a bubble
// widget only for the rows near the viewport, recycles it once it scrolls
// away, and keeps the row under the reader where it is when an older page
// is spliced in above or a row above changes height. Nothing here sets the
// scroll value to hold the view; the one piece of scrolling done by hand
// is following the foot of the thread (thread_scroll.go), and that is
// theirs too.

var threadModelType = gioutil.NewListModelType[client.Message]()

// newThreadList builds the list view over cv.model and its row factory.
func (cv *ConversationView) newThreadList() *gtk.ListView {
	factory := gtk.NewSignalListItemFactory()
	factory.ConnectSetup(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		// Rows are not activatable, selectable or focusable: hover and
		// selection chrome never apply, and arrow keys stay with the
		// composer. Bubbles' own buttons still take focus.
		item.SetActivatable(false)
		item.SetSelectable(false)
		item.SetFocusable(false)
		// No side margins here: the thread's 18px inset lives on
		// .chatot-conv-list, and margins would compound with it.
		item.SetChild(gtk.NewBox(gtk.OrientationVertical, 0))
	})
	factory.ConnectBind(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		if box, ok := item.Child().(*gtk.Box); ok {
			cv.fillRow(box, int(item.Position()))
		}
	})
	factory.ConnectUnbind(func(obj *glib.Object) {
		item := obj.Cast().(*gtk.ListItem)
		if box, ok := item.Child().(*gtk.Box); ok {
			removeAllChildren(box)
			cv.forgetRow(box)
		}
	})
	lv := gtk.NewListView(gtk.NewNoSelection(cv.model), &factory.ListItemFactory)
	lv.AddCSSClass("chatot-conv-list")
	// Rows are measured at their natural height (Fractal: "needed to use
	// the natural height of GtkPictures").
	lv.SetVScrollPolicy(gtk.ScrollNatural)
	// The list itself never takes focus: a GtkListView that does scrolls
	// to its current item, which after a chat opens is the first row of
	// the newest page, wherever the reader is by then. Bubble buttons and
	// the composer still do.
	lv.SetFocusable(false)
	lv.SetTabBehavior(gtk.ListTabItem)
	return lv
}

// rowFor is the bound row showing message id, nil when it is not realized.
func (cv *ConversationView) rowFor(id string) *gtk.Box {
	for box, mid := range cv.rowMsg {
		if mid == id {
			return box
		}
	}
	return nil
}

// refillRow re-renders the row at pos in place (its message, or its
// predecessor, changed). A row GTK has not realized is left alone: it is
// filled from cv.msgs when it is bound.
func (cv *ConversationView) refillRow(pos int) {
	if pos < 0 || pos >= len(cv.msgs) {
		return
	}
	if box := cv.rowFor(cv.msgs[pos].ID); box != nil {
		cv.fillRow(box, pos)
	}
}

// forgetRow drops the bookkeeping a row carried.
func (cv *ConversationView) forgetRow(row *gtk.Box) {
	delete(cv.rowMsg, row)
	if cv.anchorRow == row {
		cv.anchorRow = nil
	}
}

// scrollToRow brings the row at pos into view. Going to a row is leaving
// the foot of the thread: following it stops, or the growth from the
// pages a search jump loads would pull the view straight back down.
func (cv *ConversationView) scrollToRow(pos int) {
	if pos < 0 || pos >= cv.model.Len() {
		return
	}
	cv.sticky = false
	cv.autoScrolling = false
	cv.autoGen++
	cv.stopFling()
	cv.listView.ScrollTo(uint(pos), gtk.ListScrollNone, nil)
}
