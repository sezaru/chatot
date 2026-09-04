package ui

import (
	"sort"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The thread virtualizes on top of its real rows. A row that scrolls far
// from the viewport is emptied into a placeholder of the exact height it
// had, so the scroll geometry never changes and the number of live bubble
// widgets stays bounded however far back the reader goes; a placeholder
// that comes back within reach is filled again from cv.msgs. Unlike a
// GtkListView estimate, a recorded height is exact, so nothing shifts.

// threadWindowPages is how far past each edge of the viewport rows stay
// live, in viewport heights.
const threadWindowPages = 2.0

// queueWindowUpdate runs updateWindow once the current scroll or layout
// settles, at most once per main-loop turn.
func (cv *ConversationView) queueWindowUpdate() {
	if cv.windowQueued {
		return
	}
	cv.windowQueued = true
	glib.IdleAdd(func() {
		cv.windowQueued = false
		cv.updateWindow()
	})
}

// updateWindow empties the live rows that fell out of reach and fills the
// placeholders that came within it.
func (cv *ConversationView) updateWindow() {
	if len(cv.rows) == 0 {
		return
	}
	adj := cv.scroller.VAdjustment()
	page := adj.PageSize()
	if page <= 0 {
		return
	}
	top := adj.Value() - threadWindowPages*page
	bottom := adj.Value() + page + threadWindowPages*page

	for row := range cv.live {
		if row == cv.anchorRow {
			continue
		}
		y, h, ok := cv.rowBounds(row)
		if ok && (y+h < top || y > bottom) {
			cv.toPlaceholder(row)
		}
	}

	// The first row whose bottom edge is inside the reach, then every row
	// down to the reach's end.
	first := sort.Search(len(cv.rows), func(i int) bool {
		y, h, ok := cv.rowBounds(cv.rows[i])
		return !ok || y+h >= top
	})
	for i := first; i < len(cv.rows); i++ {
		row := cv.rows[i]
		y, _, ok := cv.rowBounds(row)
		if ok && y > bottom {
			break
		}
		if cv.live[row] || cv.pending[row] {
			continue
		}
		// A row above the reader that comes back a different height
		// (media that grew after it was recorded) would move the view.
		if y < adj.Value() {
			cv.holdView()
		}
		cv.materialize(i)
	}
	trace(1, "window: %d live of %d rows", len(cv.live), len(cv.rows))
}

// rowBounds is row's vertical extent in the thread's coordinates.
func (cv *ConversationView) rowBounds(row *gtk.Box) (y, h float64, ok bool) {
	b, ok := row.ComputeBounds(cv.list)
	if !ok {
		return 0, 0, false
	}
	return float64(b.Y()), float64(b.Height()), true
}

// toPlaceholder empties row, keeping its height.
func (cv *ConversationView) toPlaceholder(row *gtk.Box) {
	h := row.AllocatedHeight()
	if h <= 0 {
		return
	}
	removeAllChildren(row)
	row.SetSizeRequest(-1, h)
	delete(cv.live, row)
}

// materialize fills the placeholder (or pending row) at pos with its bubble.
func (cv *ConversationView) materialize(pos int) {
	row := cv.rows[pos]
	delete(cv.pending, row)
	row.SetSizeRequest(-1, -1)
	cv.fillRow(row, pos)
}
