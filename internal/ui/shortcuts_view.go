package ui

// shortcutRow is one name/keys line in the Preferences Shortcuts page.
type shortcutRow struct {
	Group  string
	Action string
	Keys   string
}

// shortcutGroups is the display order of the Shortcuts page's cards.
var shortcutGroups = []string{"Window", "Conversation"}

// appShortcuts lists the accelerators actually wired in the app, so the
// Shortcuts page never claims a binding that doesn't exist. Keep in sync
// with app.SetAccelsForAction calls (main.go) and the conversation search
// bar's key controller.
func appShortcuts() []shortcutRow {
	return []shortcutRow{
		{Group: "Window", Action: "Search chats", Keys: "Ctrl+K"},
		{Group: "Window", Action: "Preferences", Keys: "Ctrl+,"},
		{Group: "Window", Action: "Close window", Keys: "Ctrl+W"},
		{Group: "Window", Action: "Quit", Keys: "Ctrl+Q"},
		{Group: "Conversation", Action: "Search in chat", Keys: "Ctrl+F"},
		{Group: "Conversation", Action: "Next search result", Keys: "Enter"},
		{Group: "Conversation", Action: "Previous search result", Keys: "Shift+Enter"},
		{Group: "Conversation", Action: "Close search bar", Keys: "Esc"},
	}
}

// shortcutsByGroup buckets appShortcuts in shortcutGroups order.
func shortcutsByGroup() [][]shortcutRow {
	out := make([][]shortcutRow, len(shortcutGroups))
	for _, sc := range appShortcuts() {
		for i, g := range shortcutGroups {
			if sc.Group == g {
				out[i] = append(out[i], sc)
			}
		}
	}
	return out
}
