package ui

import (
	"reflect"
	"testing"

	"chatot/internal/client"
)

// labelsOf is menuItemLabels with separators kept as a visible marker, so a
// test can assert section boundaries as well as item order.
func labelsOf(items []menuItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Separator {
			out = append(out, "---")
			continue
		}
		out = append(out, it.Label)
	}
	return out
}

func iconsOf(items []menuItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Separator {
			continue
		}
		out = append(out, it.Icon)
	}
	return out
}

func TestPlusMenuItems(t *testing.T) {
	got := labelsOf(plusMenuItems(plusMenuActions{}))
	want := []string{"New chat", "New group", "New community"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("plusMenuItems() = %v, want %v", got, want)
	}
	if gotIcons := iconsOf(plusMenuItems(plusMenuActions{})); !reflect.DeepEqual(gotIcons, []string{"💬", "👥", "🏘"}) {
		t.Errorf("plusMenuItems() icons = %v", gotIcons)
	}
}

func TestAppMenuItems(t *testing.T) {
	items := appMenuItems(appMenuActions{})
	got := labelsOf(items)
	want := []string{
		"Archived", "Starred messages", "Blocked contacts", "---",
		"Linked devices", "Preferences", "About chatot", "---",
		"Unlink this device", "Quit",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appMenuItems() = %v, want %v", got, want)
	}

	byLabel := map[string]menuItem{}
	for _, it := range items {
		byLabel[it.Label] = it
	}
	if byLabel["Preferences"].Accel != "Ctrl+," {
		t.Errorf("Preferences accel = %q, want Ctrl+,", byLabel["Preferences"].Accel)
	}
	if byLabel["Quit"].Accel != "Ctrl+Q" {
		t.Errorf("Quit accel = %q, want Ctrl+Q", byLabel["Quit"].Accel)
	}
	if !byLabel["Unlink this device"].Destructive {
		t.Error("Unlink this device should be destructive")
	}
	if byLabel["Quit"].Destructive {
		t.Error("Quit should not be destructive")
	}
}

func TestChatMenuItems(t *testing.T) {
	t.Run("one-to-one chat in its default state", func(t *testing.T) {
		got := labelsOf(chatMenuItems(client.Chat{}, false, chatMenuActions{}))
		want := []string{
			"Contact info", "Search in chat", "Media, links and docs", "---",
			"Mute notifications…", "Pin chat", "Disappearing messages…", "Archive chat", "---",
			"Export chat…", "Clear chat…", "Block contact…",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("chatMenuItems() = %v, want %v", got, want)
		}
	})

	t.Run("group chats say Group info", func(t *testing.T) {
		got := labelsOf(chatMenuItems(client.Chat{IsGroup: true}, false, chatMenuActions{}))
		if got[0] != "Group info" {
			t.Errorf("first item = %q, want Group info", got[0])
		}
	})

	t.Run("state flips the toggling labels", func(t *testing.T) {
		chat := client.Chat{Muted: true, Pinned: true, Archived: true}
		got := labelsOf(chatMenuItems(chat, false, chatMenuActions{}))
		for _, want := range []string{"Unmute notifications", "Unpin chat", "Unarchive chat"} {
			found := false
			for _, g := range got {
				if g == want {
					found = true
				}
			}
			if !found {
				t.Errorf("missing %q in %v", want, got)
			}
		}
	})

	t.Run("clear and block are destructive, archive is not", func(t *testing.T) {
		for _, it := range chatMenuItems(client.Chat{}, false, chatMenuActions{}) {
			switch it.Label {
			case "Clear chat…", "Block contact…":
				if !it.Destructive {
					t.Errorf("%q should be destructive", it.Label)
				}
			case "Archive chat", "Export chat…":
				if it.Destructive {
					t.Errorf("%q should not be destructive", it.Label)
				}
			}
		}
	})

	t.Run("search carries the Ctrl+F accelerator", func(t *testing.T) {
		for _, it := range chatMenuItems(client.Chat{}, false, chatMenuActions{}) {
			if it.Label == "Search in chat" && it.Accel != "Ctrl+F" {
				t.Errorf("Search in chat accel = %q, want Ctrl+F", it.Accel)
			}
		}
	})
}

func TestMessageMenuItems(t *testing.T) {
	t.Run("incoming message", func(t *testing.T) {
		got := labelsOf(messageMenuItems(client.Message{}, messageMenuActions{}))
		want := []string{
			"Reply", "Forward", "Star message", "Copy text", "Pin in chat", "---",
			"Message info", "Delete message",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("messageMenuItems() = %v, want %v", got, want)
		}
	})

	t.Run("starred messages offer to unstar", func(t *testing.T) {
		got := labelsOf(messageMenuItems(client.Message{Starred: true}, messageMenuActions{}))
		if got[2] != "Unstar message" {
			t.Errorf("third item = %q, want Unstar message", got[2])
		}
	})

	t.Run("own messages add Edit after Copy text", func(t *testing.T) {
		got := labelsOf(messageMenuItems(client.Message{FromMe: true}, messageMenuActions{}))
		want := []string{
			"Reply", "Forward", "Star message", "Copy text", "Edit message", "Pin in chat", "---",
			"Message info", "Delete message",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("messageMenuItems(own) = %v, want %v", got, want)
		}
	})

	t.Run("delete is the only destructive row", func(t *testing.T) {
		for _, it := range messageMenuItems(client.Message{FromMe: true}, messageMenuActions{}) {
			if it.Destructive != (it.Label == "Delete message") {
				t.Errorf("%q destructive = %v", it.Label, it.Destructive)
			}
		}
	})
}

func TestChatRowMenuItems(t *testing.T) {
	labels := []chatLabelState{
		{ID: "l1", Name: "Work", Applied: false},
		{ID: "l2", Name: "Family", Applied: true},
	}
	got := labelsOf(chatRowMenuItems(client.Chat{}, labels, chatRowMenuActions{}))
	want := []string{
		"Pin chat", "Mute", "Mark as unread", "Lists",
		"Work", "Family", "New list…", "---",
		"Archive chat", "Delete chat",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chatRowMenuItems() = %v, want %v", got, want)
	}

	items := chatRowMenuItems(client.Chat{}, labels, chatRowMenuActions{})
	byLabel := map[string]menuItem{}
	for _, it := range items {
		byLabel[it.Label] = it
	}
	if !byLabel["Lists"].Dim {
		t.Error("the Lists caption should be dimmed")
	}
	if byLabel["Work"].Icon != "☐" {
		t.Errorf("unapplied label icon = %q, want ☐", byLabel["Work"].Icon)
	}
	if byLabel["Family"].Icon != "☑" {
		t.Errorf("applied label icon = %q, want ☑", byLabel["Family"].Icon)
	}
	if !byLabel["Delete chat"].Destructive {
		t.Error("Delete chat should be destructive")
	}
}

func TestBlockChatMenuLabel(t *testing.T) {
	if got := blockChatMenuLabel(false); got != "Block contact…" {
		t.Errorf("blockChatMenuLabel(false) = %q", got)
	}
	if got := blockChatMenuLabel(true); got != "Unblock contact" {
		t.Errorf("blockChatMenuLabel(true) = %q", got)
	}
}
