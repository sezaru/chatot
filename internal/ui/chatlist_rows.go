package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The sidebar's rows are reconciled, not rebuilt. On a live account every
// message and receipt reaches refresh, and rebuilding 500+ rows from
// scratch cost ~150 ms of main-loop time each time (measured on a 546-chat
// store): that was the scroll stutter in both panes. A row whose view-model
// is unchanged keeps its widget, a changed one gets a fresh child, and rows
// only move when the order did.

// What the ListBox holds; rows are only reused across refreshes of the
// same kind.
const (
	listOther  = ""
	listChats  = "chats"
	listMerged = "merged"
)

// rowEntry is one rendered chat row: what it was built from and its row.
type rowEntry struct {
	vm  chatRowView
	row *gtk.ListBoxRow
}

// wantRow is one row the list should show: its identity, view-model, and
// how to build the widget when the current one can't be kept.
type wantRow struct {
	key   string
	vm    chatRowView
	build func() *gtk.Box
}

// resetList empties the ListBox and forgets the reconciled rows. Every
// non-chat filler (search hits, the empty state) goes through here.
func (cl *ChatList) resetList() {
	cl.list.RemoveAll()
	cl.rows = nil
	cl.listKind = listOther
}

// reconcileRows makes the ListBox show exactly want, in order, keeping
// every row whose view-model is unchanged. kind guards against reusing
// rows built for another mode. Must run on the GTK main loop.
func (cl *ChatList) reconcileRows(kind string, want []wantRow) {
	if len(want) == 0 {
		cl.resetList()
		cl.list.Append(cl.newListEmptyState())
		return
	}
	if cl.listKind != kind || cl.rows == nil {
		cl.resetList()
		cl.rows = make(map[string]*rowEntry, len(want))
		cl.listKind = kind
	}
	keep := make(map[string]bool, len(want))
	for _, w := range want {
		keep[w.key] = true
	}
	for key, e := range cl.rows {
		if !keep[key] {
			cl.list.Remove(e.row)
			delete(cl.rows, key)
		}
	}
	for i, w := range want {
		e := cl.rows[w.key]
		if e == nil {
			row := gtk.NewListBoxRow()
			row.SetChild(w.build())
			cl.list.Insert(row, i)
			cl.rows[w.key] = &rowEntry{vm: w.vm, row: row}
			continue
		}
		if e.vm != w.vm {
			e.row.SetChild(w.build())
			e.vm = w.vm
		}
		if e.row.Index() != i {
			cl.list.Remove(e.row)
			cl.list.Insert(e.row, i)
		}
	}
}

// invalidateRow forces jid's row(s) to be rebuilt on the next refresh, for
// a change the view-model doesn't carry (a new avatar picture).
func (cl *ChatList) invalidateRow(jid string) {
	for _, e := range cl.rows {
		if e.vm.JID == jid {
			e.vm = chatRowView{}
		}
	}
}

// rowsInOrder reports whether every reconciled row sits where rowJIDs says
// (dev hooks).
func (cl *ChatList) rowsInOrder() bool {
	if cl.listKind != listChats {
		return true
	}
	for i, jid := range cl.rowJIDs {
		e := cl.rows[jid]
		if e == nil || e.row.Index() != i {
			return false
		}
	}
	return cl.list.RowAtIndex(len(cl.rowJIDs)) == nil
}
