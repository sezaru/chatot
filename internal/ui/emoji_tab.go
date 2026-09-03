package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// pickerEmojiCols / pickerEmojiCell are the mockup's emoji grid: nine columns
// of 32px cells carrying 18px glyphs, capped at 210px so the card stays the
// design's height and the rest scrolls.
const (
	pickerEmojiCols   = 9
	pickerEmojiCell   = 32
	pickerEmojiHeight = 210
)

// pickerEmojis is the picker's palette, grouped the way the mockup orders it
// (smileys, gestures, hearts, objects). It opens with the design's exact set
// so the first two rows read identically, then continues with the everyday
// glyphs a real composer needs — the native GtkEmojiChooser this replaces
// offered the full Unicode set, and shipping only 46 would be a regression.
var pickerEmojis = []string{
	"😀", "😃", "😄", "😁", "😆", "😅", "😂", "🙂", "🙃",
	"😉", "😊", "😇", "🥰", "😍", "🤩", "😘", "😗", "😚",
	"🤗", "🤔", "🤨", "😐", "😴", "🥳", "😎", "🤓", "🧐",
	"😕", "🙁", "😢", "😭", "😤", "😡", "🥺", "😳", "🤯",
	"😬", "🙄", "😏", "😌", "😔", "🤧", "🤒", "🤕", "🥱",
	"👍", "👎", "👏", "🙏", "💪", "👌", "✌️", "🤝", "👋",
	"🔥", "✨", "🎉", "💯", "⭐", "🌟", "💥", "❗", "❓",
	"❤️", "🧡", "💛", "💚", "💙", "💜", "🖤", "💔", "💕",
	"🐦", "🎵", "🎶", "☕", "🍕", "🎂", "🌧", "☀️", "🌙",
	"✅", "❌", "⏰", "📅", "📍", "📷", "🎁", "💡", "🚗",
}

// newEmojiTab builds the picker's Emoji page: a scrolling nine-column grid
// that inserts the picked glyph at the entry's cursor and closes the popover,
// matching the mockup's behaviour.
func newEmojiTab(c *Composer, popover *gtk.Popover) gtk.Widgetter {
	grid := gtk.NewFlowBox()
	grid.SetSelectionMode(gtk.SelectionNone)
	grid.SetMinChildrenPerLine(pickerEmojiCols)
	grid.SetMaxChildrenPerLine(pickerEmojiCols)
	grid.SetRowSpacing(2)
	grid.SetColumnSpacing(2)
	grid.SetHomogeneous(true)
	grid.SetActivateOnSingleClick(true)

	for _, glyph := range pickerEmojis {
		emoji := glyph
		btn := gtk.NewButtonWithLabel(emoji)
		btn.AddCSSClass("flat")
		btn.AddCSSClass("chatot-picker-emoji")
		btn.SetSizeRequest(-1, pickerEmojiCell)
		btn.ConnectClicked(func() {
			c.entry.InsertAtCursor(emoji)
			popover.Popdown()
			// Return focus to the entry so typing continues where the glyph
			// landed instead of at the popover's former grab.
			c.entry.GrabFocus()
		})
		grid.Insert(btn, -1)
	}

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetChild(grid)
	// A hard height request, not just MaxContentHeight: the flow box reports a
	// natural height for every row it holds, and the scroller honours that
	// first, so the card grew past the design's 210px window.
	scroller.SetSizeRequest(-1, pickerEmojiHeight)
	scroller.SetPropagateNaturalHeight(false)
	return scroller
}
