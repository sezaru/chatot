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

func TestSearchHitVM(t *testing.T) {
	now := time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)

	t.Run("basic fields pass through", func(t *testing.T) {
		vm := searchHitVM(client.SearchHit{
			ChatJID:  "j1",
			ChatName: "Ada Lovelace",
			Snippet:  "let's grab [pizza] tonight",
			TS:       now.Add(-time.Hour).Unix(),
		}, now)

		if vm.ChatJID != "j1" {
			t.Errorf("ChatJID = %q, want j1", vm.ChatJID)
		}
		if vm.ChatName != "Ada Lovelace" {
			t.Errorf("ChatName = %q, want passthrough", vm.ChatName)
		}
		if vm.Snippet != "let's grab [pizza] tonight" {
			t.Errorf("Snippet = %q, want passthrough", vm.Snippet)
		}
		if vm.Initial != "A" {
			t.Errorf("Initial = %q, want A", vm.Initial)
		}
		if vm.TimeText != "14:00" {
			t.Errorf("TimeText = %q, want 14:00", vm.TimeText)
		}
	})

	t.Run("empty chat name falls back to JID", func(t *testing.T) {
		vm := searchHitVM(client.SearchHit{ChatJID: "1234567890@s.whatsapp.net"}, now)
		if vm.ChatName != "1234567890@s.whatsapp.net" {
			t.Errorf("ChatName = %q, want the JID fallback", vm.ChatName)
		}
		if vm.Initial == "" {
			t.Error("Initial should not be empty even with no name")
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

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"spaces stripped", "+1 555 123 4567", "+15551234567", true},
		{"dashes and parens stripped", "+1 (555)-123-4567", "+15551234567", true},
		{"missing plus", "15551234567", "", false},
		{"too short", "+1555", "", false},
		{"too long", "+1555123456789012", "", false},
		{"letters rejected", "+1555abc4567", "", false},
		{"empty", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizePhone(tc.input)
			if ok != tc.ok || got != tc.want {
				t.Errorf("normalizePhone(%q) = (%q, %v), want (%q, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}
}
