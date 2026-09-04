package ui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// An older page is streamed in rather than built in one go: forty bubbles
// took over 100 ms on the main loop and read as a hitch every time the
// reader hit the top. The page's rows go in at once as empty, zero-height
// placeholders (cheap, and they keep rows and msgs aligned), then are
// filled a few per frame from the one nearest the reader upward, each
// frame holding the view (thread_anchor.go).

// prependChunk is how many rows one frame builds.
const prependChunk = 6

// streamPrepend fills the pending rows[0:count], nearest the reader first,
// one chunk per frame. loadingOlder stays set until the page is in, so the
// next page is not asked for while this one is still landing.
func (cv *ConversationView) streamPrepend(count int) {
	gen := cv.threadGen
	next := count
	gtk.BaseWidget(cv.list).AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		if gen != cv.threadGen {
			return false
		}
		if next == 0 {
			cv.loadingOlder = false
			cv.queueWindowUpdate()
			// The reader may still be at the top: see whether another page
			// is wanted without waiting for the next scroll event.
			cv.onScroll()
			return false
		}
		lo := next - prependChunk
		if lo < 0 {
			lo = 0
		}
		cv.holdView()
		for i := next - 1; i >= lo; i-- {
			cv.materialize(i)
		}
		trace(1, "stream: built rows %d..%d", lo, next-1)
		next = lo
		return true
	})
}

// insertPending puts count empty rows at the top of the thread, to be
// filled by streamPrepend.
func (cv *ConversationView) insertPending(count int) {
	cv.holdView()
	fresh := make([]*gtk.Box, 0, count)
	var prev gtk.Widgetter
	for i := 0; i < count; i++ {
		row := gtk.NewBox(gtk.OrientationVertical, 0)
		row.SetSizeRequest(-1, 0)
		cv.pending[row] = true
		cv.list.InsertChildAfter(row, prev)
		prev = row
		fresh = append(fresh, row)
	}
	cv.rows = append(fresh, cv.rows...)
	// The old first row's predecessor changed (its day separator goes).
	if count < len(cv.rows) {
		cv.refillRow(count)
	}
}

// cancelFling stops the scrolled window's deceleration animation. GTK keeps
// a fling moving along its own timeline and ignores a value set from
// outside, so a page landing under a fling fought it every frame. Turning
// kinetic scrolling off and on again is GTK's only public way to cancel
// it; wheel and touchpad scrolling are unaffected.
func (cv *ConversationView) cancelFling() {
	cv.scroller.SetKineticScrolling(false)
	cv.scroller.SetKineticScrolling(true)
}
