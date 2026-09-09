package ui

import (
	"strconv"

	"github.com/diamondburned/gotk4/pkg/graphene"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The emoji panel's geometry, from the mockup: a 34px cell for the
// composer's nine columns, a slightly tighter one for the reaction card's
// eight, and a category rail under both.
const (
	emojiCellSize   = 34
	emojiRailButton = 28
	// emojiPinSlack is the half-pixel of tolerance the pinned heading gives
	// the scroll position, so a list sitting exactly on a heading counts as
	// at rest rather than one pixel into it.
	emojiPinSlack = 0.5
)

// emojiPanelConfig is what differs between the two pickers that share this
// panel: how wide the grid is, how tall the scroller stands, and what a
// pick does.
type emojiPanelConfig struct {
	Columns int
	// Height is the scrolling area's fixed height. The flow boxes report a
	// natural height for every row they hold and the scroller honours that
	// first, so without a hard request the card grows past the design.
	Height int
	// Width, when > 0, is a hard width request on the grid, keeping the
	// card at the mockup's size before any emoji are measured.
	Width int
	// OnPick receives the glyph. The panel has already recorded it in
	// recents by then.
	OnPick func(glyph string)
}

// emojiSection is one group in the list: the widget holding its heading and
// grid, and the heading's text, which the pinned band reuses.
type emojiSection struct {
	widget gtk.Widgetter
	label  string
}

// newEmojiPanel builds the picker both the composer and the reaction card
// show: a search field, the catalogue in its groups behind one scroller,
// and a rail that jumps to a group. Search replaces the groups with a
// single ranked list while there is a query.
func newEmojiPanel(cfg emojiPanelConfig) gtk.Widgetter {
	col := gtk.NewBox(gtk.OrientationVertical, 7)
	col.AddCSSClass("chatot-emoji-panel")

	search := gtk.NewSearchEntry()
	search.AddCSSClass("chatot-emoji-search")
	search.SetPlaceholderText("Search emoji")
	search.SetHExpand(true)
	col.Append(search)

	// list holds one section per group; sections keeps their order so the
	// rail can scroll to one by index.
	list := gtk.NewBox(gtk.OrientationVertical, 0)
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetChild(list)
	scroller.SetSizeRequest(-1, cfg.Height)
	scroller.SetPropagateNaturalHeight(false)

	// The heading of the group being scrolled through stays at the top of
	// the list, as the mockup's `position: sticky` does it. GTK has no
	// sticky, so the band is an overlay: it names the group whose own
	// heading has scrolled out of sight, and hides while that heading is
	// still on screen. It cannot be clicked, so the emoji underneath it
	// stay reachable.
	overlay := gtk.NewOverlay()
	overlay.SetChild(scroller)
	pinned := gtk.NewLabel("")
	pinned.AddCSSClass("chatot-emoji-heading")
	pinned.AddCSSClass("chatot-emoji-heading-pinned")
	pinned.SetXAlign(0)
	pinned.SetHAlign(gtk.AlignFill)
	pinned.SetVAlign(gtk.AlignStart)
	pinned.SetCanTarget(false)
	pinned.SetVisible(false)
	overlay.AddOverlay(pinned)
	col.Append(overlay)

	var sections []emojiSection
	adj := scroller.VAdjustment()
	// syncPinned asks GTK where each section sits rather than adding up row
	// heights, so it stays right however the flow boxes wrapped.
	syncPinned := func() {
		offsets := make([]float64, 0, len(sections))
		for _, s := range sections {
			p, ok := gtk.BaseWidget(s.widget).ComputePoint(list, graphene.NewPointAlloc().Init(0, 0))
			if !ok {
				pinned.SetVisible(false)
				return
			}
			offsets = append(offsets, float64(p.Y()))
		}
		idx := pinnedEmojiSection(offsets, adj.Value())
		if idx < 0 {
			pinned.SetVisible(false)
			return
		}
		pinned.SetLabel(sections[idx].label)
		pinned.SetVisible(true)
	}
	adj.ConnectValueChanged(syncPinned)
	// The adjustment's "changed" covers a rebuilt or re-measured list, where
	// the offsets move under a scroll position that did not.
	adj.ConnectChanged(syncPinned)

	pick := func(glyph string) {
		rememberRecentEmoji(glyph)
		if cfg.OnPick != nil {
			cfg.OnPick(glyph)
		}
	}
	// fill rebuilds the list for the current query: the ranked hits while
	// there is one, otherwise recents followed by every group.
	fill := func(query string) {
		for {
			child := list.FirstChild()
			if child == nil {
				break
			}
			list.Remove(child)
		}
		sections = sections[:0]
		pinned.SetVisible(false)
		if hits := searchEmoji(query); query != "" {
			if len(hits) == 0 {
				list.Append(newEmojiEmptyState(query, cfg.Height))
				return
			}
			s := newEmojiSection(searchHeading(len(hits)), hits, cfg, pick)
			sections = append(sections, s)
			list.Append(s.widget)
			return
		}
		if recents := recentEmojiEntries(recentEmojiList()); len(recents) > 0 {
			s := newEmojiSection("Frequently used", recents, cfg, pick)
			sections = append(sections, s)
			list.Append(s.widget)
		}
		for _, g := range emojiSet() {
			s := newEmojiSection(g.Label, g.Items, cfg, pick)
			sections = append(sections, s)
			list.Append(s.widget)
		}
	}
	fill("")

	search.ConnectSearchChanged(func() { fill(search.Text()) })

	rail := gtk.NewBox(gtk.OrientationHorizontal, 1)
	rail.AddCSSClass("chatot-emoji-rail")
	var railButtons []*gtk.Button
	setActive := func(idx int) {
		for i, b := range railButtons {
			if i == idx {
				b.AddCSSClass("chatot-emoji-rail-on")
			} else {
				b.RemoveCSSClass("chatot-emoji-rail-on")
			}
		}
	}
	railEntries := append([]emojiGroup{{Key: emojiRecentKey, Icon: "🕘", Label: "Frequently used"}}, emojiSet()...)
	for i, g := range railEntries {
		idx, group := i, g
		btn := gtk.NewButtonWithLabel(group.Icon)
		btn.AddCSSClass("flat")
		btn.AddCSSClass("chatot-emoji-rail-btn")
		btn.SetTooltipText(group.Label)
		btn.SetHExpand(true)
		btn.SetSizeRequest(-1, emojiRailButton)
		btn.SetFocusOnClick(false)
		btn.ConnectClicked(func() {
			// Jumping to a group clears the search, so the rail always acts
			// on the full catalogue rather than on a filtered list.
			if search.Text() != "" {
				search.SetText("")
			}
			setActive(idx)
			scrollToEmojiSection(scroller, list, sections, idx)
		})
		railButtons = append(railButtons, btn)
		rail.Append(btn)
	}
	setActive(0)
	col.Append(rail)
	return col
}

// pinnedEmojiSection is the section whose heading the band shows for scroll
// offset v, given each section's top offset, or -1 for none. A list resting
// exactly on a heading shows no band: that heading is on screen already, and
// drawing it twice is what a sticky heading must not do.
func pinnedEmojiSection(offsets []float64, v float64) int {
	idx := -1
	for i, y := range offsets {
		if y > v+emojiPinSlack {
			break
		}
		idx = i
	}
	if idx < 0 || v <= offsets[idx]+emojiPinSlack {
		return -1
	}
	return idx
}

// searchHeading is the heading over the hits, which doubles as the count.
func searchHeading(n int) string {
	if n == 1 {
		return "1 result"
	}
	return strconv.Itoa(n) + " results"
}

// newEmojiSection is one heading with its grid of emoji.
func newEmojiSection(label string, items []emojiEntry, cfg emojiPanelConfig, pick func(string)) emojiSection {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	head := gtk.NewLabel(label)
	head.AddCSSClass("chatot-emoji-heading")
	head.SetXAlign(0)
	box.Append(head)

	grid := gtk.NewFlowBox()
	grid.SetSelectionMode(gtk.SelectionNone)
	grid.SetMinChildrenPerLine(uint(cfg.Columns))
	grid.SetMaxChildrenPerLine(uint(cfg.Columns))
	grid.SetRowSpacing(1)
	grid.SetColumnSpacing(1)
	grid.SetHomogeneous(true)
	grid.SetActivateOnSingleClick(true)
	if cfg.Width > 0 {
		grid.SetSizeRequest(cfg.Width, -1)
	}
	for _, e := range items {
		entry := e
		btn := gtk.NewButtonWithLabel(entry.Char)
		btn.AddCSSClass("flat")
		btn.AddCSSClass("chatot-picker-emoji")
		btn.SetTooltipText(entry.Name)
		btn.SetSizeRequest(-1, emojiCellSize)
		btn.ConnectClicked(func() { pick(entry.Char) })
		grid.Insert(btn, -1)
	}
	box.Append(grid)
	return emojiSection{widget: box, label: label}
}

// newEmojiEmptyState is what a search with no hits shows, with a nudge
// towards the words that do work.
func newEmojiEmptyState(query string, height int) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 7)
	box.SetVAlign(gtk.AlignCenter)
	box.SetSizeRequest(-1, height)
	glyph := gtk.NewLabel("🔍")
	glyph.AddCSSClass("chatot-emoji-empty-glyph")
	box.Append(glyph)
	l := gtk.NewLabel("Nothing matches “" + query + "”.\nTry a word like “laugh”, “thanks” or “coffee”.")
	l.AddCSSClass("chatot-emoji-empty")
	l.SetJustify(gtk.JustifyCenter)
	l.SetWrap(true)
	box.Append(l)
	return box
}

// scrollToEmojiSection puts section idx at the top of the scroller. The
// section's offset is asked of GTK rather than accumulated by hand, so it
// stays right whatever the rows measured.
func scrollToEmojiSection(scroller *gtk.ScrolledWindow, list *gtk.Box, sections []emojiSection, idx int) {
	if idx < 0 || idx >= len(sections) {
		return
	}
	target := gtk.BaseWidget(sections[idx].widget)
	point, ok := target.ComputePoint(list, graphene.NewPointAlloc().Init(0, 0))
	if !ok {
		return
	}
	adj := scroller.VAdjustment()
	y := float64(point.Y())
	if max := adj.Upper() - adj.PageSize(); y > max {
		y = max
	}
	if y < 0 {
		y = 0
	}
	adj.SetValue(y)
}
