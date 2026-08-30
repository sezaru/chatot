package ui

import (
	"context"
	"log"
	"strconv"
	"strings"

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
	Active bool
}

// chipCountText appends " <n>" to base when n > 0, matching the mockup's
// "Unread 19" / "Groups 7" style (no trailing count when there's nothing to
// show, e.g. plain "All").
func chipCountText(base string, n int) string {
	if n <= 0 {
		return base
	}
	return base + " " + strconv.Itoa(n)
}

// buildChips derives the fixed chip row plus, when a custom label is
// active, that label's chip appended inline so it stays visible and
// clearable without opening the overflow popover.
func buildChips(counts chatCounts, filter chatFilter, labelCounts map[string]int, labels []client.Label) []chipSpec {
	chips := []chipSpec{
		{Key: "all", Text: "All", Active: filter.Kind == filterAll},
		{Key: "unread", Text: chipCountText("Unread", counts.Unread), Active: filter.Kind == filterUnread},
		{Key: "favorites", Text: "Favorites", Active: filter.Kind == filterFavorites},
		{Key: "groups", Text: chipCountText("Groups", counts.Groups), Active: filter.Kind == filterGroups},
	}
	if filter.Kind == filterLabel {
		for _, l := range labels {
			if l.ID != filter.LabelID {
				continue
			}
			chips = append(chips, chipSpec{
				Key:    "label:" + l.ID,
				Text:   chipCountText(labelDisplayName(l), labelCounts[l.ID]),
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
	if cl.showStarred {
		cl.showStarred = false
		cl.starredT.SetActive(false)
	}
	if cl.showStatus {
		cl.showStatus = false
		cl.statusT.SetActive(false)
		cl.postStatusBar.SetVisible(false)
	}
	if cl.showChannels {
		cl.showChannels = false
		cl.channelT.SetActive(false)
		cl.followBar.SetVisible(false)
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

	for child := cl.chipRow.FirstChild(); child != nil; child = cl.chipRow.FirstChild() {
		cl.chipRow.Remove(child)
	}

	for _, chip := range buildChips(counts, cl.filter, labelCounts, labels) {
		cl.chipRow.Append(cl.buildChipButton(chip))
	}

	overflow := gtk.NewButtonWithLabel("…")
	overflow.AddCSSClass("flat")
	overflow.AddCSSClass("chatot-chip")
	overflow.SetTooltipText("More labels")
	overflow.ConnectClicked(func() {
		cl.showLabelOverflowPopover(overflow, labels, labelCounts)
	})
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

// buildChipButton renders one pill chip; clicking it selects the filter it
// represents.
func (cl *ChatList) buildChipButton(chip chipSpec) *gtk.Button {
	btn := gtk.NewButtonWithLabel(chip.Text)
	btn.AddCSSClass("chatot-chip")
	if chip.Active {
		// A class rule on the embedded sheet does not reliably repaint a
		// button's background here; a per-widget provider does (same trick
		// as the label dots).
		css := gtk.NewCSSProvider()
		css.LoadFromString("button { background-image: none; background-color: #1b8c72; color: #ffffff; } button:hover { background-color: #0f6350; }")
		btn.StyleContext().AddProvider(css, uint(gtk.STYLE_PROVIDER_PRIORITY_USER))
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
	pop := gtk.NewPopover()
	box := gtk.NewBox(gtk.OrientationVertical, 2)

	shown := 0
	for _, l := range labels {
		if cl.filter.Kind == filterLabel && cl.filter.LabelID == l.ID {
			continue
		}
		shown++
		row := gtk.NewBox(gtk.OrientationHorizontal, 6)

		dot := gtk.NewLabel("")
		dot.AddCSSClass("chatot-label-dot")
		dot.SetSizeRequest(10, 10)
		css := gtk.NewCSSProvider()
		css.LoadFromString("label { background-color: " + labelColorHex(l.Color) + "; border-radius: 999px; }")
		dot.StyleContext().AddProvider(css, uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
		row.Append(dot)

		btn := gtk.NewButtonWithLabel(chipCountText(labelDisplayName(l), labelCounts[l.ID]))
		btn.AddCSSClass("flat")
		btn.SetHExpand(true)
		labelID := l.ID
		btn.ConnectClicked(func() {
			pop.Popdown()
			cl.setFilter(chatFilter{Kind: filterLabel, LabelID: labelID})
		})
		row.Append(btn)

		box.Append(row)
	}
	if shown == 0 {
		empty := gtk.NewLabel("No more labels")
		empty.AddCSSClass("chatot-placeholder")
		box.Append(empty)
	}

	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	box.Append(sep)

	manageBtn := gtk.NewButtonWithLabel("Manage labels…")
	manageBtn.AddCSSClass("flat")
	manageBtn.ConnectClicked(func() {
		pop.Popdown()
		showCreateLabelDialog(cl.window, cl.c, func(labelID string) {
			cl.setFilter(chatFilter{Kind: filterLabel, LabelID: labelID})
		})
	})
	box.Append(manageBtn)

	pop.SetChild(box)
	pop.SetParent(anchor)
	pop.ConnectClosed(func() { pop.Unparent() })
	pop.Popup()
}

// showLabelsSubmenu pops a checklist of every label reflecting whether chat
// carries it (toggling calls SetChatLabeled), anchored to parent — the
// context menu's "Labels ▸" entry.
func showLabelsSubmenu(parent *gtk.Button, c client.Client, chat client.Chat) {
	pop := gtk.NewPopover()
	box := gtk.NewBox(gtk.OrientationVertical, 2)

	labels, err := c.Labels()
	if err != nil {
		labels = nil
	}
	onChat, _ := c.LabelsForChat(chat.JID)
	active := make(map[string]bool, len(onChat))
	for _, id := range onChat {
		active[id] = true
	}

	if len(labels) == 0 {
		empty := gtk.NewLabel("No labels yet")
		empty.AddCSSClass("chatot-placeholder")
		box.Append(empty)
	}
	for _, l := range labels {
		check := gtk.NewCheckButtonWithLabel(labelDisplayName(l))
		check.SetActive(active[l.ID])
		labelID := l.ID
		check.ConnectToggled(func() {
			labeled := check.Active()
			go func() {
				if err := c.SetChatLabeled(context.Background(), labelID, chat.JID, labeled); err != nil {
					log.Printf("chatot: set chat label failed: %v", err)
				}
			}()
		})
		box.Append(check)
	}

	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	box.Append(sep)

	newBtn := gtk.NewButtonWithLabel("+ New label…")
	newBtn.AddCSSClass("flat")
	newBtn.ConnectClicked(func() {
		pop.Popdown()
		showCreateLabelDialog(nil, c, func(labelID string) {
			go func() {
				if err := c.SetChatLabeled(context.Background(), labelID, chat.JID, true); err != nil {
					log.Printf("chatot: set chat label failed: %v", err)
				}
			}()
		})
	})
	box.Append(newBtn)

	pop.SetChild(box)
	pop.SetParent(parent)
	pop.ConnectClosed(func() { pop.Unparent() })
	pop.Popup()
}

// showCreateLabelDialog opens a small modal to create a label by name,
// calling onCreated with its new id on success. Doubles as the sidebar's
// "Manage labels…" fallback, since there's no full label CRUD UI yet.
func showCreateLabelDialog(parent *gtk.Window, c client.Client, onCreated func(labelID string)) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("New label")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Label name")
	box.Append(entry)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	box.Append(status)

	createBtn := gtk.NewButtonWithLabel("Create")
	createBtn.AddCSSClass("suggested-action")
	box.Append(createBtn)

	create := func() {
		name := strings.TrimSpace(entry.Text())
		if name == "" {
			status.SetText("Enter a label name")
			return
		}
		createBtn.SetSensitive(false)
		status.SetText("Creating…")
		go func() {
			id, err := c.CreateLabel(context.Background(), name, 0)
			glib.IdleAdd(func() {
				createBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't create label, try again")
					return
				}
				dialog.Close()
				if onCreated != nil {
					onCreated(id)
				}
			})
		}()
	}
	createBtn.ConnectClicked(create)
	entry.ConnectActivate(create)

	dialog.SetChild(box)
	dialog.SetDefaultWidget(createBtn)
	dialog.Present()
}
