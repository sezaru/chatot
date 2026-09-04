package ui

import (
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The thread is real row widgets in a vertical box, one per cv.msgs entry,
// not a GtkListView. A virtualized list can only estimate the height of
// rows it hasn't realized, and bubbles range from one line to a photo: as
// the reader scrolled up, every re-estimate moved the content and rows were
// realized and unrealized in storms (traced at 600+ binds a second), which
// read as the thread flickering up and down. Real rows have exact heights,
// so a prepended page is anchored to the pixel and nothing visible moves.
// Pages are 40 messages, so a long read back still means hundreds of
// rows; see thread_window.go for how far ones are emptied.

// newRowAt builds the row widget for cv.msgs[pos].
func (cv *ConversationView) newRowAt(pos int) *gtk.Box {
	// No side margins here: the thread's 18px inset lives on
	// .chatot-conv-list, and margins would compound with it.
	row := gtk.NewBox(gtk.OrientationVertical, 0)
	cv.fillRow(row, pos)
	return row
}

// refillRow re-renders the row at pos in place (its message changed). A
// placeholder is left alone: it is filled from cv.msgs when it comes back
// within reach.
func (cv *ConversationView) refillRow(pos int) {
	if pos < 0 || pos >= len(cv.rows) || !cv.live[cv.rows[pos]] {
		return
	}
	cv.fillRow(cv.rows[pos], pos)
}

// spliceRows mirrors a change already made to cv.msgs into the widgets:
// removed rows at pos go, rows for cv.msgs[pos:pos+added] come in, and the
// row after the block is refilled when the block changed size, since its
// predecessor (day separator, unread pill) did.
func (cv *ConversationView) spliceRows(pos, removed, added int) {
	for i := 0; i < removed && pos < len(cv.rows); i++ {
		row := cv.rows[pos]
		cv.forgetRow(row)
		cv.list.Remove(row)
		cv.rows = append(cv.rows[:pos], cv.rows[pos+1:]...)
	}
	var prev gtk.Widgetter
	if pos > 0 {
		prev = cv.rows[pos-1]
	}
	fresh := make([]*gtk.Box, 0, added)
	for i := 0; i < added; i++ {
		row := cv.newRowAt(pos + i)
		cv.list.InsertChildAfter(row, prev)
		prev = row
		fresh = append(fresh, row)
	}
	if added > 0 {
		rest := append([]*gtk.Box(nil), cv.rows[pos:]...)
		cv.rows = append(append(cv.rows[:pos], fresh...), rest...)
	}
	if next := pos + added; added != removed && next < len(cv.rows) {
		cv.refillRow(next)
	}
	cv.queueWindowUpdate()
}

// clearRows drops every row.
func (cv *ConversationView) clearRows() {
	for _, row := range cv.rows {
		cv.forgetRow(row)
	}
	removeAllChildren(cv.list)
	cv.rows = nil
	cv.live = map[*gtk.Box]bool{}
	cv.pending = map[*gtk.Box]bool{}
	cv.anchor = nil
	cv.threadGen++
}

// forgetRow drops the bookkeeping a row carried.
func (cv *ConversationView) forgetRow(row *gtk.Box) {
	delete(cv.rowMsg, row)
	delete(cv.live, row)
	delete(cv.pending, row)
	if cv.anchorRow == row {
		cv.anchorRow = nil
	}
}

// scrollToRow brings the row at pos to the top of the viewport once it has
// a layout.
func (cv *ConversationView) scrollToRow(pos int) {
	if pos < 0 || pos >= len(cv.rows) {
		return
	}
	row := cv.rows[pos]
	glib.IdleAdd(func() {
		b, ok := row.ComputeBounds(cv.list)
		if !ok {
			return
		}
		adj := cv.scroller.VAdjustment()
		adj.SetValue(float64(b.Y()) - rowScrollInset)
	})
}

// rowScrollInset keeps a sliver of the previous row visible above a row
// scrolled to, so it doesn't sit hard against the top edge.
const rowScrollInset = 12.0

// onAdjustmentChanged runs when the thread's scrollable height changed:
// the stick-to-bottom window is honoured and the live window re-checked.
func (cv *ConversationView) onAdjustmentChanged() {
	cv.stickToBottom()
	cv.queueWindowUpdate()
}
