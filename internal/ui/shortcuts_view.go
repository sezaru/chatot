package ui

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
