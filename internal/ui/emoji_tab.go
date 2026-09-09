package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// pickerEmojiCols / pickerEmojiHeight are the mockup's emoji grid: nine
// columns, capped at 212px so the card stays the design's height and the
// catalogue scrolls inside it.
const (
	pickerEmojiCols   = 9
	pickerEmojiCell   = 32
	pickerEmojiHeight = 212
)

// newEmojiTab builds the picker's Emoji page: the shared panel over the
// whole catalogue — search, groups, the category rail — inserting the
// picked glyph at the entry's cursor and closing the popover.
func newEmojiTab(c *Composer, popover *gtk.Popover) gtk.Widgetter {
	return newEmojiPanel(emojiPanelConfig{
		Columns: pickerEmojiCols,
		Height:  pickerEmojiHeight,
		OnPick: func(glyph string) {
			c.entry.InsertAtCursor(glyph)
			popover.Popdown()
			// Return focus to the entry so typing continues where the glyph
			// landed instead of at the popover's former grab.
			c.entry.GrabFocus()
		},
	})
}
