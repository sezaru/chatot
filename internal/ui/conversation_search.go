package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// setupHeaderMenu wires the header "⋮" menu to a popover-of-buttons (same
// pattern as the per-message "⋯" menu in buildBubbleActions), disabled until
// a chat is open (see refreshHeader).
func (cv *ConversationView) setupHeaderMenu(menuBtn *gtk.Button) {
	popover := gtk.NewPopover()
	menu := gtk.NewBox(gtk.OrientationVertical, 0)

	addItem := func(label string, onClick func(jid string)) {
		btn := gtk.NewButtonWithLabel(label)
		btn.AddCSSClass("flat")
		btn.ConnectClicked(func() {
			popover.Popdown()
			if cv.jid != "" {
				onClick(cv.jid)
			}
		})
		menu.Append(btn)
	}

	addItem("Search in chat", func(string) { cv.openSearchBar() })
	addItem("Media, links and docs", func(jid string) {
		if cv.onShowMedia != nil {
			cv.onShowMedia(jid)
		}
	})
	addItem("Export chat…", func(jid string) {
		if cv.onExportChat != nil {
			cv.onExportChat(jid)
		}
	})
	addItem("Clear chat…", func(jid string) {
		if cv.onClearChat != nil {
			cv.onClearChat(jid)
		}
	})

	popover.SetChild(menu)
	popover.SetParent(menuBtn)
	menuBtn.ConnectClicked(func() { popover.Popup() })
}

// openSearchBar reveals the in-chat search bar and focuses its entry.
func (cv *ConversationView) openSearchBar() {
	cv.searchRevealer.SetRevealChild(true)
	cv.searchEntry.GrabFocus()
}

// closeSearchBar hides the in-chat search bar and clears any active search
// state (and its highlights).
func (cv *ConversationView) closeSearchBar() {
	cv.searchRevealer.SetRevealChild(false)
	cv.searchEntry.SetText("")
}

// runSearch re-queries the store for query scoped to the open chat, updates
// the hit counter, jumps to the most recent match, and (un)highlights
// matching bubbles. Must run on the GTK main loop.
func (cv *ConversationView) runSearch(query string) {
	query = strings.TrimSpace(query)
	cv.searchQuery = query

	if query == "" || cv.jid == "" {
		cv.searchHits = nil
		cv.searchIdx = -1
		cv.searchHitLabel.SetLabel("")
		cv.applyHighlights(nil)
		return
	}

	hits, err := cv.c.SearchInChat(cv.jid, query, 0)
	if err != nil {
		hits = nil
	}
	cv.searchHits = hits
	cv.applyHighlights(hits)

	if len(hits) == 0 {
		cv.searchIdx = -1
		cv.searchHitLabel.SetLabel("No matches")
		return
	}

	cv.searchIdx = len(hits) - 1 // most recent match first
	cv.searchHitLabel.SetLabel(searchHitCountText(cv.searchIdx, len(hits)))
	cv.jumpToMessage(hits[cv.searchIdx].MsgID)
}

// stepHit moves to the next (forward) or previous hit, wrapping around, and
// scrolls it into view. Must run on the GTK main loop.
func (cv *ConversationView) stepHit(forward bool) {
	if len(cv.searchHits) == 0 {
		return
	}
	cv.searchIdx = searchHitStep(cv.searchIdx, len(cv.searchHits), forward)
	cv.searchHitLabel.SetLabel(searchHitCountText(cv.searchIdx, len(cv.searchHits)))
	cv.jumpToMessage(cv.searchHits[cv.searchIdx].MsgID)
}

// positionOf returns msgID's index in the currently-loaded cv.msgs, -1 if
// it isn't loaded.
func (cv *ConversationView) positionOf(msgID string) int {
	for i, m := range cv.msgs {
		if m.ID == msgID {
			return i
		}
	}
	return -1
}

// jumpToMessage scrolls the thread to msgID, synchronously loading older
// pages (via MessagesBefore) until it's found or local history runs dry.
// LIMITATION: a hit older than everything MessagesBefore can return locally
// (i.e. it would need a RequestMoreHistory round-trip to the phone) can't be
// reached — search coverage matches whatever the store already has synced.
// Must run on the GTK main loop.
func (cv *ConversationView) jumpToMessage(msgID string) {
	pos := cv.positionOf(msgID)
	for pos < 0 && cv.hasMore && !cv.loadingOlder {
		older, err := cv.c.MessagesBefore(cv.jid, cv.oldestID, conversationPageSize)
		if err != nil || len(older) == 0 {
			cv.hasMore = len(older) == conversationPageSize
			break
		}
		cv.prependOlder(older)
		pos = cv.positionOf(msgID)
	}
	if pos < 0 {
		return
	}
	cv.listView.ScrollTo(uint(pos), gtk.ListScrollNone, nil)
}

// touchRow forces the row at pos to rebind (re-run bindRow), by splicing the
// same message back into the model at that position — the cheapest way to
// get GtkListView to re-render a row that isn't otherwise changing.
func (cv *ConversationView) touchRow(pos int) {
	if pos < 0 || pos >= len(cv.msgs) {
		return
	}
	cv.model.Splice(pos, 1, cv.msgs[pos])
}

// applyHighlights re-renders every row that was highlighted before (so a
// narrowed/cleared query drops stale highlights) or is highlighted now (so a
// widened query picks up new ones), then remembers the new set.
func (cv *ConversationView) applyHighlights(hits []client.SearchHit) {
	touched := make(map[int]bool, len(cv.highlightedPositions)+len(hits))
	for pos := range cv.highlightedPositions {
		touched[pos] = true
	}
	next := make(map[int]bool, len(hits))
	for _, h := range hits {
		if pos := cv.positionOf(h.MsgID); pos >= 0 {
			touched[pos] = true
			next[pos] = true
		}
	}
	for pos := range touched {
		cv.touchRow(pos)
	}
	cv.highlightedPositions = next
}

// searchHitCountText is the pure hit-counter label for idx (currently
// selected, 0-based) of total hits.
func searchHitCountText(idx, total int) string {
	if total == 0 {
		return "No matches"
	}
	return fmt.Sprintf("%d of %d matches in this chat", idx+1, total)
}

// searchHitStep returns the next hit index stepping from cur (which may be
// -1 for "no current hit") among total hits, wrapping around at either end.
func searchHitStep(cur, total int, forward bool) int {
	if total <= 0 {
		return -1
	}
	if cur < 0 {
		if forward {
			return 0
		}
		return total - 1
	}
	if forward {
		return (cur + 1) % total
	}
	return (cur - 1 + total) % total
}

// pangoEscapeText escapes text's Pango-markup-significant characters so it's
// safe to embed as literal content inside a <span> markup string.
func pangoEscapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\'':
			b.WriteString("&#39;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// matchRange is a half-open byte range [Start, End) in text matched by
// findMatches.
type matchRange struct{ Start, End int }

// findMatches locates every case-insensitive, non-overlapping occurrence of
// query in text, returning byte ranges into the ORIGINAL text. It walks text
// rune-by-rune and compares unicode.ToLower per rune, so it never slices a
// lower-cased copy whose byte length can differ from the original (e.g.
// Turkish "İ" U+0130 is 2 bytes but folds to 1-byte "i") — every boundary is
// a real rune boundary in text. An empty query matches nothing.
func findMatches(text, query string) []matchRange {
	qr := []rune(query)
	for i := range qr {
		qr[i] = unicode.ToLower(qr[i])
	}
	if len(qr) == 0 {
		return nil
	}

	var ranges []matchRange
	i := 0
	for i < len(text) {
		if end, ok := matchAt(text, i, qr); ok {
			ranges = append(ranges, matchRange{i, end})
			i = end
			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}
	return ranges
}

// matchAt reports whether the lower-cased query runes qr match text starting
// at byte offset start, and if so returns the byte offset just past the match
// in the original text.
func matchAt(text string, start int, qr []rune) (end int, ok bool) {
	pos := start
	for _, want := range qr {
		if pos >= len(text) {
			return 0, false
		}
		r, size := utf8.DecodeRuneInString(text[pos:])
		if unicode.ToLower(r) != want {
			return 0, false
		}
		pos += size
	}
	return pos, true
}

// highlightMarkup renders text as Pango markup with every case-insensitive
// occurrence of query wrapped in a highlight <span>, escaping everything
// else so the result is safe to pass to gtk.Label.SetMarkup.
func highlightMarkup(text, query string) string {
	ranges := findMatches(text, query)
	if len(ranges) == 0 {
		return pangoEscapeText(text)
	}

	var b strings.Builder
	prev := 0
	for _, r := range ranges {
		b.WriteString(pangoEscapeText(text[prev:r.Start]))
		b.WriteString(`<span background="#f5c518" foreground="#1b1b1b">`)
		b.WriteString(pangoEscapeText(text[r.Start:r.End]))
		b.WriteString(`</span>`)
		prev = r.End
	}
	b.WriteString(pangoEscapeText(text[prev:]))
	return b.String()
}
