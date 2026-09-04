package ui

import (
	"sort"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Keeping the thread still while content above the reader changes height
// (an older page streaming in, a placeholder filling back, a day separator
// going) is done after GTK's layout and before paint, from the frame
// clock's layout phase. Nothing is predicted: holdView notes which row sits
// under the viewport's top edge and how far below it, afterLayout reads
// where that row actually landed, sets the scroll value so it is back in
// place, and re-allocates the viewport so the frame about to be painted is
// already right. Measuring the new rows beforehand was tried and is not
// exact: a widget's natural height before it is styled and allocated is
// not always the height it ends up with.

// viewAnchor is the row under the viewport's top edge and the distance
// from the row's top down to that edge.
type viewAnchor struct {
	row    *gtk.Box
	offset float64
}

// holdView pins the view on the row under its top edge until the next
// layout. A hold already taken this frame is kept.
func (cv *ConversationView) holdView() {
	if cv.anchor != nil || len(cv.rows) == 0 {
		return
	}
	cv.hookLayout()
	adj := cv.scroller.VAdjustment()
	value := adj.Value()
	i := sort.Search(len(cv.rows), func(i int) bool {
		y, h, ok := cv.rowBounds(cv.rows[i])
		return ok && y+h > value
	})
	if i >= len(cv.rows) {
		return
	}
	y, _, _ := cv.rowBounds(cv.rows[i])
	cv.anchor = &viewAnchor{row: cv.rows[i], offset: value - y}
}

// hookLayout connects afterLayout to the frame clock once; GTK's own
// layout runs first on the same signal, so the allocation is fresh.
func (cv *ConversationView) hookLayout() {
	if cv.layoutHooked {
		return
	}
	clock := cv.scroller.FrameClock()
	if clock == nil {
		return
	}
	gdk.BaseFrameClock(clock).ConnectLayout(cv.afterLayout)
	cv.layoutHooked = true
}

// afterLayout puts the held row back where it was (or, at the foot of the
// thread, keeps the foot in view) and re-allocates the viewport with the
// corrected value, inside the same frame.
func (cv *ConversationView) afterLayout() {
	adj := cv.scroller.VAdjustment()
	a := cv.anchor
	if a == nil {
		if cv.followBottom {
			if max := adj.Upper() - adj.PageSize(); adj.Value() < max-0.5 {
				adj.SetValue(max)
				cv.reallocateViewport()
			}
		}
		return
	}
	cv.anchor = nil
	y, _, ok := cv.rowBounds(a.row)
	if !ok {
		return
	}
	if traceLevel >= 2 {
		lh, up := cv.list.AllocatedHeight(), cv.scroller.VAdjustment().Upper()
		trace(2, "afterLayout: list height %d upper %.0f anchor y %.0f offset %.0f", lh, up, y, a.offset)
		glib.IdleAdd(func() {
			y2, _, _ := cv.rowBounds(a.row)
			trace(2, "  post-frame: list height %d upper %.0f anchor y %.0f", cv.list.AllocatedHeight(), cv.scroller.VAdjustment().Upper(), y2)
		})
	}
	adj = cv.scroller.VAdjustment()
	target := y + a.offset
	if d := target - adj.Value(); d > -0.5 && d < 0.5 {
		return
	}
	before := adj.Value()
	adj.SetValue(target)
	cv.reallocateViewport()
	trace(1, "held view: value %.0f→%.0f", before, adj.Value())
}

// reallocateViewport re-runs the viewport's allocation with the current
// scroll value, so a value set after GTK's layout is painted this frame.
func (cv *ConversationView) reallocateViewport() {
	if vp, ok := cv.list.Parent().(*gtk.Viewport); ok {
		vp.SizeAllocate(vp.Allocation(), -1)
	}
}
