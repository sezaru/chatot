package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestChatRowVM(t *testing.T) {
	now := time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)

	t.Run("basic fields pass through", func(t *testing.T) {
		vm := chatRowVM(client.Chat{
			JID:     "j1",
			Name:    "ada lovelace",
			Preview: "See you tomorrow!",
		}, now)

		if vm.JID != "j1" {
			t.Errorf("JID = %q, want j1", vm.JID)
		}
		if vm.Name != "ada lovelace" {
			t.Errorf("Name = %q, want passthrough", vm.Name)
		}
		if vm.Preview != "See you tomorrow!" {
			t.Errorf("Preview = %q, want passthrough", vm.Preview)
		}
		if vm.Initial != "A" {
			t.Errorf("Initial = %q, want A", vm.Initial)
		}
	})

	t.Run("empty name falls back", func(t *testing.T) {
		vm := chatRowVM(client.Chat{JID: "j2"}, now)
		if vm.Name == "" {
			t.Error("Name should not be empty for a chat with no name")
		}
		if vm.Initial == "" {
			t.Error("Initial should not be empty even with no name")
		}
	})

	t.Run("unread badge shows only when UnreadCount > 0", func(t *testing.T) {
		read := chatRowVM(client.Chat{Name: "X", UnreadCount: 0}, now)
		if read.ShowUnread {
			t.Error("ShowUnread should be false for UnreadCount 0")
		}
		if read.UnreadText != "" {
			t.Errorf("UnreadText = %q, want empty", read.UnreadText)
		}

		unread := chatRowVM(client.Chat{Name: "X", UnreadCount: 3}, now)
		if !unread.ShowUnread {
			t.Error("ShowUnread should be true for UnreadCount 3")
		}
		if unread.UnreadText != "3" {
			t.Errorf("UnreadText = %q, want 3", unread.UnreadText)
		}
	})

	t.Run("unread count caps display at 99+", func(t *testing.T) {
		vm := chatRowVM(client.Chat{Name: "X", UnreadCount: 150}, now)
		if vm.UnreadText != "99+" {
			t.Errorf("UnreadText = %q, want 99+", vm.UnreadText)
		}
	})
}

func TestFormatChatTime(t *testing.T) {
	now := time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC) // a Sunday

	cases := []struct {
		name string
		ts   int64
		want string
	}{
		{"zero ts", 0, ""},
		{"earlier today", now.Add(-2 * time.Hour).Unix(), "13:00"},
		{"three days ago", now.AddDate(0, 0, -3).Unix(), now.AddDate(0, 0, -3).Format("Monday")},
		{"eight days ago", now.AddDate(0, 0, -8).Unix(), now.AddDate(0, 0, -8).Format("02/01")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatChatTime(tc.ts, now)
			if got != tc.want {
				t.Errorf("formatChatTime(%d) = %q, want %q", tc.ts, got, tc.want)
			}
		})
	}
}
