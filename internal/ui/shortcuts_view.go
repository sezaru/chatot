package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// shortcutRow is one name/keys line in the keyboard-shortcuts dialog.
type shortcutRow struct {
	Action string
	Keys   string
}

// appShortcuts lists the accelerators actually wired in the app, so the ⋮
// menu's "Keyboard shortcuts" entry never claims a binding that doesn't
// exist. Keep in sync with app.SetAccelsForAction calls (main.go) and the
// conversation search bar's key controller.
func appShortcuts() []shortcutRow {
	return []shortcutRow{
		{Action: "Preferences", Keys: "Ctrl+,"},
		{Action: "Next search result", Keys: "Enter"},
		{Action: "Previous search result", Keys: "Shift+Enter"},
		{Action: "Close search bar", Keys: "Escape"},
	}
}

// showShortcutsDialog presents a static modal listing the app's keyboard
// shortcuts, since the gotk4 binding has no gtk.ShortcutsWindow.
func showShortcutsDialog(parent *gtk.Window) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Keyboard shortcuts")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	for _, row := range appShortcuts() {
		line := gtk.NewBox(gtk.OrientationHorizontal, 8)

		action := gtk.NewLabel(row.Action)
		action.SetXAlign(0)
		action.SetHExpand(true)
		line.Append(action)

		keys := gtk.NewLabel(row.Keys)
		keys.AddCSSClass("dim-label")
		line.Append(keys)

		box.Append(line)
	}

	dialog.SetChild(box)
	dialog.Present()
}
