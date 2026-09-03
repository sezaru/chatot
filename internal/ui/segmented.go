package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// segmentedPage is one tab of a segmented pill switcher: the stack page it
// selects and the label shown on it.
type segmentedPage struct{ ID, Label string }

// newSegmentedSwitcher builds the mockup's segmented pill — equal-width
// buttons in a tinted track, the active one a raised white chip. A
// GtkStackSwitcher can't be styled into this shape, so the toggles drive the
// stack directly.
//
// Shared by the composer's Emoji|GIF|Stickers picker and the Media|Links|Docs
// page, which the design draws identically.
func newSegmentedSwitcher(stack *gtk.Stack, pages []segmentedPage, expand bool) gtk.Widgetter {
	track := gtk.NewBox(gtk.OrientationHorizontal, 4)
	track.AddCSSClass("chatot-segmented")

	var buttons []*gtk.ToggleButton
	sync := func() {
		current := stack.VisibleChildName()
		for i, b := range buttons {
			b.SetActive(pages[i].ID == current)
		}
	}
	for _, page := range pages {
		id := page.ID
		btn := gtk.NewToggleButton()
		btn.SetLabel(page.Label)
		btn.AddCSSClass("chatot-segmented-tab")
		// The picker's tabs split the card's full width; the media page's hug
		// their labels beside a title.
		btn.SetHExpand(expand)
		btn.ConnectClicked(func() {
			stack.SetVisibleChildName(id)
			sync()
		})
		buttons = append(buttons, btn)
		track.Append(btn)
	}
	// Keep the pill in step with any other route to a page (a shot hook, the
	// initial state), not just with clicks on the pill itself.
	stack.NotifyProperty("visible-child-name", sync)
	sync()
	return track
}
