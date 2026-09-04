package ui

import (
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Following the foot of the thread, as Paper Plane does it. sticky is set
// while the reader is at the very bottom, from every scroll; while it
// holds, any growth of the content (a new row, a picture taking its
// height, the window shrinking) scrolls back to the bottom. A reader who
// has scrolled up is left alone until they come back down. autoScrolling
// marks a scroll of ours in flight, so the value changes it causes are
// not read as the reader moving, and so a GtkListView correcting its
// height estimate after the jump is followed to the real bottom.

// atBottom reports whether the thread is scrolled to its newest message.
func (cv *ConversationView) atBottom() bool {
	adj := cv.scroller.VAdjustment()
	return adj.Value()+adj.PageSize() >= adj.Upper()-1
}

// onScroll runs on every change of the scroll value.
func (cv *ConversationView) onScroll() {
	adj := cv.scroller.VAdjustment()
	if traceLevel > 0 {
		trace(1, "scroll value=%.0f upper=%.0f page=%.0f sticky=%v auto=%v loadingOlder=%v inFlight=%v hasMore=%v",
			adj.Value(), adj.Upper(), adj.PageSize(), cv.sticky, cv.autoScrolling, cv.loadingOlder, cv.historyInFlight, cv.hasMore)
	}
	// A value that changed along with the height or the page size is the
	// list re-laid out (the window resized, rows re-measured), not the
	// reader scrolling: GTK emits the value first and notifies the new
	// geometry after, so sticky is kept and the foot re-pinned here rather
	// than read off a position that is already stale.
	geometryChanged := adj.Upper() != cv.lastUpper || adj.PageSize() != cv.lastPage
	cv.lastUpper, cv.lastPage = adj.Upper(), adj.PageSize()
	if cv.autoScrolling {
		if cv.atBottom() {
			cv.autoScrolling = false
			cv.sticky = true
		}
		return
	}
	if geometryChanged {
		if cv.sticky && !cv.atBottom() {
			cv.scrollDown()
		}
		return
	}
	cv.sticky = cv.atBottom()
	cv.scheduleUnreadClear()
	cv.loadOlderIfNeeded()
}

// onUpperChanged runs when the thread's scrollable height or the
// viewport's height changed.
func (cv *ConversationView) onUpperChanged() {
	adj := cv.scroller.VAdjustment()
	cv.lastUpper, cv.lastPage = adj.Upper(), adj.PageSize()
	if cv.sticky || cv.autoScrolling {
		cv.scrollDown()
	}
	cv.loadOlderIfNeeded()
}

// scrollDown scrolls to the foot of the thread. The value is set first,
// then the last row is made the list's anchor: GtkListView anchors the
// row at the top of the viewport on any value change and re-derives the
// value from its anchor on every later layout, so with the top row as
// anchor a re-measure of the rows above (heights estimated for rows not
// yet realized, text re-wrapped on resize) would move the foot out of
// view without the height changing.
func (cv *ConversationView) scrollDown() {
	n := cv.model.Len()
	if n == 0 {
		return
	}
	cv.autoScrolling = true
	cv.autoGen++
	gen := cv.autoGen
	cv.stopFling()
	adj := cv.scroller.VAdjustment()
	adj.SetValue(adj.Upper() - adj.PageSize())
	cv.listView.ScrollTo(uint(n-1), gtk.ListScrollNone, nil)
	if cv.atBottom() {
		cv.autoScrolling = false
		cv.sticky = true
		return
	}
	// The bottom is reached once the list has laid out and re-measured;
	// if it never reports it, stop treating scrolls as ours.
	glib.TimeoutAdd(autoScrollGrace, func() bool {
		if gen == cv.autoGen && cv.autoScrolling {
			trace(1, "scrollDown: gave up waiting for the bottom")
			cv.autoScrolling = false
			cv.sticky = cv.atBottom()
		}
		return false
	})
}

// autoScrollGrace (ms) bounds how long a scroll of ours may take to land.
const autoScrollGrace = 500

// scrollToBottom scrolls to the newest message and follows the thread
// from there.
func (cv *ConversationView) scrollToBottom() {
	trace(1, "scrollToBottom")
	cv.sticky = true
	cv.scrollDown()
}

// loadOlderIfNeeded fetches the next older page when the reader is within
// two pages of the top, or the thread is shorter than that.
func (cv *ConversationView) loadOlderIfNeeded() {
	if cv.autoScrolling || cv.loadingOlder || cv.historyInFlight || !cv.hasMore || cv.jid == "" {
		return
	}
	adj := cv.scroller.VAdjustment()
	if adj.Value() < adj.PageSize()*2 || adj.Upper() <= adj.PageSize()*2 {
		cv.loadOlder()
	}
}
