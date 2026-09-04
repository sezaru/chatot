package ui

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// chatFilterKind selects which sidebar chip is active.
type chatFilterKind int

const (
	filterAll chatFilterKind = iota
	filterUnread
	filterFavorites
	filterGroups
	filterLabel
)

// chatFilter is the sidebar's active chip: one of the fixed kinds, or
// filterLabel with LabelID set to a custom label's id.
type chatFilter struct {
	Kind    chatFilterKind
	LabelID string
}

// chatMatchesFilter reports whether c should show under filter. onLabels is
// c's current label ids; only filterLabel consults it, so callers can pass
// nil for the fixed filters and skip the LabelsForChat round-trip.
func chatMatchesFilter(c client.Chat, onLabels []string, filter chatFilter) bool {
	switch filter.Kind {
	case filterUnread:
		return c.UnreadCount > 0
	case filterFavorites:
		return c.Pinned
	case filterGroups:
		return c.IsGroup
	case filterLabel:
		for _, id := range onLabels {
			if id == filter.LabelID {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// chatVisible is chatMatchesFilter plus the client round-trip filterLabel
// needs to learn c's labels.
func chatVisible(c client.Client, chat client.Chat, filter chatFilter) bool {
	if filter.Kind != filterLabel {
		return chatMatchesFilter(chat, nil, filter)
	}
	ids, err := c.LabelsForChat(chat.JID)
	if err != nil {
		return false
	}
	return chatMatchesFilter(chat, ids, filter)
}

// chatCounts holds the tallies driving the fixed chips' badge numbers.
type chatCounts struct {
	Unread int
	Groups int
}

// computeChatCounts tallies unread and group chats. chats should already be
// scoped to the current archived-state, matching what the fixed chips count
// against regardless of which chip is active.
func computeChatCounts(chats []client.Chat) chatCounts {
	var cc chatCounts
	for _, c := range chats {
		if c.UnreadCount > 0 {
			cc.Unread++
		}
		if c.IsGroup {
			cc.Groups++
		}
	}
	return cc
}

// computeLabelCounts tallies, for each label id, how many chats carry it.
// chatLabels maps a chat JID to its label ids.
func computeLabelCounts(chatLabels map[string][]string) map[string]int {
	counts := make(map[string]int)
	for _, ids := range chatLabels {
		for _, id := range ids {
			counts[id]++
		}
	}
	return counts
}

// chipSpec is the pure, pre-rendered state of one filter chip.
type chipSpec struct {
	Key    string // "all", "unread", "favorites", "groups", or "label:<id>"
	Text   string
	Count  int    // rendered after Text as the mockup's accent-bold numeral; 0 = none
	Dot    string // label chips: leading rounded-square dot in this hex color; "" = none
	Active bool
}

// buildChips derives the fixed chip row plus, when a custom label is
// active, that label's chip appended inline so it stays visible and
// clearable without opening the overflow popover.
func buildChips(counts chatCounts, filter chatFilter, labelCounts map[string]int, labels []client.Label) []chipSpec {
	chips := []chipSpec{
		{Key: "all", Text: "All", Active: filter.Kind == filterAll},
		{Key: "unread", Text: "Unread", Count: counts.Unread, Active: filter.Kind == filterUnread},
		{Key: "favorites", Text: "Favorites", Active: filter.Kind == filterFavorites},
		{Key: "groups", Text: "Groups", Count: counts.Groups, Active: filter.Kind == filterGroups},
	}
	if filter.Kind == filterLabel {
		for _, l := range labels {
			if l.ID != filter.LabelID {
				continue
			}
			chips = append(chips, chipSpec{
				Key:    "label:" + l.ID,
				Text:   labelDisplayName(l),
				Count:  labelCounts[l.ID],
				Dot:    labelColorHex(l.Color),
				Active: true,
			})
			break
		}
	}
	return chips
}

// labelDisplayName falls back to a synthetic name for a label with no name
// set (mirrors the old dropdown's behavior).
func labelDisplayName(l client.Label) string {
	if l.Name != "" {
		return l.Name
	}
	return "Label " + l.ID
}

// labelSwatchColors maps whatsmeow's small label color-index palette to a
// display hex; the index wraps for anything out of range so an unexpected
// value still renders a dot rather than erroring.
var labelSwatchColors = []string{
	"#e34c4c", "#e3924c", "#e3c94c", "#a8c94c", "#4ce36e",
	"#4cc9e3", "#4c92e3", "#7a4ce3", "#c94ce3", "#e34c9c",
	"#8c8c8c", "#5a7ab5", "#b58a4a", "#9c5b8a", "#c26b5c",
	"#7a8b5a", "#e8a34a", "#427677", "#6a6a8c", "#1b8c72",
}

// labelColorHex resolves a label's Color index to a display hex string.
func labelColorHex(color int) string {
	if len(labelSwatchColors) == 0 {
		return "#8c8c8c"
	}
	idx := color % len(labelSwatchColors)
	if idx < 0 {
		idx += len(labelSwatchColors)
	}
	return labelSwatchColors[idx]
}

// filterForChipKey parses a chipSpec.Key/overflow-row key back into the
// chatFilter it selects.
func filterForChipKey(key string) chatFilter {
	switch key {
	case "unread":
		return chatFilter{Kind: filterUnread}
	case "favorites":
		return chatFilter{Kind: filterFavorites}
	case "groups":
		return chatFilter{Kind: filterGroups}
	default:
		if id, ok := strings.CutPrefix(key, "label:"); ok {
			return chatFilter{Kind: filterLabel, LabelID: id}
		}
		return chatFilter{}
	}
}

// setFilter applies f, exiting search/starred/status/channels mode the same
// way picking one of those exits the others, then refreshes.
func (cl *ChatList) setFilter(f chatFilter) {
	cl.filter = f
	if cl.query != "" {
		cl.query = ""
		cl.search.SetText("")
	}
	if cl.tab != "chats" {
		cl.selectTab("chats")
	}
	cl.refresh()
}

// updateChipRow rebuilds the chip row from the current chats/labels/filter.
// Must run on the GTK main loop; called from refresh so the chips (unread
// count, active state, inline label) always track live data.
func (cl *ChatList) updateChipRow() {
	chats, err := cl.c.Chats(0)
	if err != nil {
		chats = nil
	}
	scoped := make([]client.Chat, 0, len(chats))
	for _, c := range chats {
		if showChatInList(c, cl.showArchived) {
			scoped = append(scoped, c)
		}
	}
	counts := computeChatCounts(scoped)

	labels, err := cl.c.Labels()
	if err != nil {
		labels = nil
	}
	if cl.filter.Kind == filterLabel && !labelExists(labels, cl.filter.LabelID) {
		cl.filter = chatFilter{}
	}

	chatLabels := make(map[string][]string, len(scoped))
	for _, c := range scoped {
		if ids, err := cl.c.LabelsForChat(c.JID); err == nil {
			chatLabels[c.JID] = ids
		}
	}
	labelCounts := computeLabelCounts(chatLabels)

	// The strip is rebuilt on every refresh; emptying it resets its scroll,
	// which is put back once the new chips have their size.
	scrollX := cl.chipScroller.HAdjustment().Value()
	for child := cl.chipRow.FirstChild(); child != nil; child = cl.chipRow.FirstChild() {
		cl.chipRow.Remove(child)
	}

	for _, chip := range buildChips(counts, cl.filter, labelCounts, labels) {
		cl.chipRow.Append(cl.buildChipButton(chip))
	}
	if scrollX > 0 {
		glib.IdleAdd(func() { cl.chipScroller.HAdjustment().SetValue(scrollX) })
	}

	overflow := gtk.NewButtonWithLabel("…")
	overflow.AddCSSClass("flat")
	overflow.AddCSSClass("chatot-chip")
	overflow.SetTooltipText("More labels")
	overflow.SetFocusOnClick(false)
	cl.overflowBtn = overflow
	overflow.ConnectClicked(func() {
		cl.showLabelOverflowPopover(overflow, labels, labelCounts)
	})
	// The … chip scrolls with the others: the row is one strip.
	cl.chipRow.Append(overflow)
}

func labelExists(labels []client.Label, id string) bool {
	for _, l := range labels {
		if l.ID == id {
			return true
		}
	}
	return false
}

// buildChipButton renders one pill chip — optional label dot, text, and the
// mockup's accent-bold trailing count; clicking it selects the filter it
// represents.
func (cl *ChatList) buildChipButton(chip chipSpec) *gtk.Button {
	btn := gtk.NewButton()
	btn.AddCSSClass("chatot-chip")
	btn.SetFocusOnClick(false)

	content := gtk.NewBox(gtk.OrientationHorizontal, 5)
	if chip.Dot != "" {
		dot := gtk.NewLabel("")
		dot.SetSizeRequest(8, 8)
		dot.SetVAlign(gtk.AlignCenter)
		css := gtk.NewCSSProvider()
		css.LoadFromString("label { background-color: " + chip.Dot + "; border-radius: 3px; }")
		dot.StyleContext().AddProvider(css, widgetPriority(uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)))
		content.Append(dot)
	}
	content.Append(gtk.NewLabel(chip.Text))
	if chip.Count > 0 {
		count := gtk.NewLabel(strconv.Itoa(chip.Count))
		count.AddCSSClass("chatot-chip-count")
		content.Append(count)
	}
	btn.SetChild(content)

	if chip.Active {
		// A class rule on the embedded sheet does not reliably repaint a
		// button's background here; a per-widget provider does (same trick
		// as the label dots). The count label's white-on-green variant rides
		// on the class below (providers don't reach child widgets).
		btn.AddCSSClass("chatot-chip-active")
		css := gtk.NewCSSProvider()
		css.LoadFromString("button { background-image: none; background-color: #1b8c72; color: #ffffff; font-weight: bold; } button:hover { background-color: #0f6350; }")
		btn.StyleContext().AddProvider(css, widgetPriority(uint(gtk.STYLE_PROVIDER_PRIORITY_USER)))
	}
	key := chip.Key
	btn.ConnectClicked(func() {
		cl.setFilter(filterForChipKey(key))
	})
	return btn
}

// showLabelOverflowPopover pops a menu of every custom label not already
// shown inline as the active chip (colored dot + name + chat count), plus a
// "Manage labels…" row.
func (cl *ChatList) showLabelOverflowPopover(anchor *gtk.Button, labels []client.Label, labelCounts map[string]int) {
	rows := make([]labelMenuRow, 0, len(labels))
	for _, l := range labels {
		// The list already showing as the active chip is not offered again.
		if cl.filter.Kind == filterLabel && cl.filter.LabelID == l.ID {
			continue
		}
		rows = append(rows, labelMenuRow{
			ID:    l.ID,
			Name:  labelDisplayName(l),
			Color: labelColorHex(l.Color),
			Count: labelCounts[l.ID],
		})
	}

	pop := newMenuPopover(labelMenuItems(rows,
		func(id string) { cl.setFilter(chatFilter{Kind: filterLabel, LabelID: id}) },
		func() {
			showCreateLabelDialog(cl.window, cl.c, func(labelID string) {
				cl.setFilter(chatFilter{Kind: filterLabel, LabelID: labelID})
			})
		},
	))
	pop.AddCSSClass("chatot-menu-lists")
	// Parented to the list, not the button: the chip row is rebuilt on
	// every refresh, and the popover must outlive the button it opened
	// from. It points at where that button was.
	if b, ok := anchor.ComputeBounds(cl); ok {
		rect := gdk.NewRectangle(int(b.X()), int(b.Y()), int(b.Width()), int(b.Height()))
		pop.SetPointingTo(&rect)
	}
	pop.SetParent(cl)
	cl.overflowPop = pop
	pop.ConnectClosed(func() {
		if cl.overflowPop == pop {
			cl.overflowPop = nil
		}
		pop.Unparent()
	})
	pop.Popup()
}

// showCreateLabelDialog opens a small modal to create a label by name,
// calling onCreated with its new id on success. Doubles as the sidebar's
// "Manage labels…" fallback, since there's no full label CRUD UI yet.
func showCreateLabelDialog(parent *gtk.Window, c client.Client, onCreated func(labelID string)) {
	showManageListsDialog(parent, c, onCreated)
}

// listChatCount renders a list's chat count the way the mockup's row does.
func listChatCount(n int) string {
	if n == 1 {
		return "1 chat"
	}
	return fmt.Sprintf("%d chats", n)
}

// showManageListsDialog is the mockup's "Manage lists" card: one row per list
// (colour dot, name, chat count, 🗑) over a colour palette, a name field and
// an Add button.
//
// This replaces a bare name-entry modal that could only create a list — the
// design's dialog is where lists are also deleted and coloured, and the API
// for both already existed.
func showManageListsDialog(parent *gtk.Window, c client.Client, onCreated func(labelID string)) {
	dialog := newCardDialog()
	dialog.SetTitle("Manage lists")
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(380, -1)

	body := dialogBody(12)

	listCard := newSettingsCard()
	body.Append(listCard)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	status.AddCSSClass("chatot-newchat-status")

	var reload func()
	reload = func() {
		removeAllChildren(listCard.Box)
		listCard.rows = 0

		labels, err := c.Labels()
		if err != nil {
			log.Printf("chatot: load labels failed: %v", err)
		}
		counts := computeLabelCounts(chatLabelsMap(c))
		if len(labels) == 0 {
			empty := gtk.NewLabel("No lists yet")
			empty.AddCSSClass("chatot-card-row")
			empty.AddCSSClass("chatot-card-sub")
			listCard.Add(empty)
			return
		}
		for _, l := range labels {
			id := l.ID
			row := gtk.NewBox(gtk.OrientationHorizontal, 11)
			row.AddCSSClass("chatot-card-row")
			row.Append(newMenuDot(labelColorHex(l.Color)))

			name := gtk.NewLabel(labelDisplayName(l))
			name.SetXAlign(0)
			name.SetHExpand(true)
			name.AddCSSClass("chatot-card-label")
			row.Append(name)

			count := gtk.NewLabel(listChatCount(counts[l.ID]))
			count.AddCSSClass("chatot-card-value")
			count.SetVAlign(gtk.AlignCenter)
			row.Append(count)

			del := gtk.NewButtonWithLabel("🗑")
			del.AddCSSClass("flat")
			del.AddCSSClass("chatot-list-delete")
			del.SetTooltipText("Delete list")
			del.SetVAlign(gtk.AlignCenter)
			del.ConnectClicked(func() {
				del.SetSensitive(false)
				go func() {
					err := c.DeleteLabel(context.Background(), id)
					glib.IdleAdd(func() {
						if err != nil {
							del.SetSensitive(true)
							status.SetText("Couldn't delete that list, try again")
							return
						}
						reload()
					})
				}()
			})
			row.Append(del)
			listCard.Add(row)
		}
	}
	reload()

	// Palette + name + Add, per the mockup's footer.
	swatches := gtk.NewBox(gtk.OrientationHorizontal, 6)
	swatches.AddCSSClass("chatot-swatch-row")
	picked := 0
	var buttons []*gtk.ToggleButton
	for i := range labelSwatchColors {
		idx := i
		btn := gtk.NewToggleButton()
		btn.AddCSSClass("chatot-swatch")
		btn.SetSizeRequest(20, 20)
		css := gtk.NewCSSProvider()
		css.LoadFromString("button { background-color: " + labelSwatchColors[idx] + "; }")
		btn.StyleContext().AddProvider(css, widgetPriority(uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)))
		btn.ConnectClicked(func() {
			picked = idx
			for j, b := range buttons {
				b.SetActive(j == idx)
			}
		})
		buttons = append(buttons, btn)
		swatches.Append(btn)
	}
	if len(buttons) > 0 {
		buttons[0].SetActive(true)
	}
	body.Append(swatches)

	addRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("New list name")
	entry.SetHExpand(true)
	addRow.Append(entry)
	addBtn := gtk.NewButtonWithLabel("Add")
	addBtn.AddCSSClass("chatot-primary-btn")
	addRow.Append(addBtn)
	body.Append(addRow)

	body.Append(status)

	hint := gtk.NewLabel("Lists are private to this device and are never sent to WhatsApp.")
	hint.SetXAlign(0)
	hint.SetWrap(true)
	hint.AddCSSClass("chatot-card-sub")
	body.Append(hint)

	create := func() {
		name := strings.TrimSpace(entry.Text())
		if name == "" {
			status.SetText("Enter a list name")
			return
		}
		addBtn.SetSensitive(false)
		status.SetText("Creating…")
		go func() {
			id, err := c.CreateLabel(context.Background(), name, picked)
			glib.IdleAdd(func() {
				addBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't create that list, try again")
					return
				}
				status.SetText("")
				entry.SetText("")
				reload()
				if onCreated != nil {
					onCreated(id)
				}
			})
		}()
	}
	addBtn.ConnectClicked(create)
	entry.ConnectActivate(create)

	dialog.SetChild(body)
	dialog.SetDefaultWidget(addBtn)
	dialog.Present()
}

// chatLabelsMap reads every chat's labels once, for the per-list chat counts.
func chatLabelsMap(c client.Client) map[string][]string {
	out := map[string][]string{}
	chats, err := c.Chats(0)
	if err != nil {
		return out
	}
	for _, chat := range chats {
		if ids, err := c.LabelsForChat(chat.JID); err == nil {
			out[chat.JID] = ids
		}
	}
	return out
}
