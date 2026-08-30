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

func TestChatActionLabels(t *testing.T) {
	cases := []struct {
		name string
		chat client.Chat
		want chatActionLabelsView
	}{
		{
			"unpinned/unmuted/unarchived/read",
			client.Chat{},
			chatActionLabelsView{Pin: "Pin", Mute: "Mute", Archive: "Archive", Unread: "Mark as unread"},
		},
		{
			"pinned/muted/archived/unread",
			client.Chat{Pinned: true, Muted: true, Archived: true, UnreadCount: 3},
			chatActionLabelsView{Pin: "Unpin", Mute: "Unmute", Archive: "Unarchive", Unread: "Mark as read"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chatActionLabels(tc.chat)
			if got != tc.want {
				t.Errorf("chatActionLabels(%+v) = %+v, want %+v", tc.chat, got, tc.want)
			}
		})
	}
}

func TestBlockActionLabel(t *testing.T) {
	if got := blockActionLabel(false); got != "Block" {
		t.Errorf("blockActionLabel(false) = %q, want %q", got, "Block")
	}
	if got := blockActionLabel(true); got != "Unblock" {
		t.Errorf("blockActionLabel(true) = %q, want %q", got, "Unblock")
	}
}

func TestPrivacySettingsRows(t *testing.T) {
	got := privacySettingsRows(map[string]string{"Status": "contacts", "Last Seen": "all", "Online": "everyone"})
	want := []privacySettingRow{
		{Name: "Last Seen", Value: "all"},
		{Name: "Online", Value: "everyone"},
		{Name: "Status", Value: "contacts"},
	}
	if len(got) != len(want) {
		t.Fatalf("privacySettingsRows() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("privacySettingsRows()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestShowChatInList(t *testing.T) {
	cases := []struct {
		name         string
		archived     bool
		showArchived bool
		want         bool
	}{
		{"normal chat, normal view", false, false, true},
		{"normal chat, archived view", false, true, false},
		{"archived chat, normal view", true, false, false},
		{"archived chat, archived view", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := showChatInList(client.Chat{Archived: tc.archived}, tc.showArchived)
			if got != tc.want {
				t.Errorf("showChatInList(archived=%v, showArchived=%v) = %v, want %v", tc.archived, tc.showArchived, got, tc.want)
			}
		})
	}
}

func TestNewChatContacts(t *testing.T) {
	chats := []client.Chat{
		{JID: "g1", Name: "Book Club", IsGroup: true},
		{JID: "c1", Name: "zack"},
		{JID: "c2", Name: "Ada"},
		{JID: "g2", Name: "AAA Group", IsGroup: true},
		{JID: "c3", Name: "bob"},
	}

	got := newChatContacts(chats)

	if len(got) != 3 {
		t.Fatalf("newChatContacts() returned %d contacts, want 3 (groups excluded)", len(got))
	}
	for _, c := range got {
		if c.IsGroup {
			t.Errorf("newChatContacts() included group chat %q", c.Name)
		}
	}

	wantOrder := []string{"Ada", "bob", "zack"}
	for i, want := range wantOrder {
		if got[i].Name != want {
			t.Errorf("newChatContacts()[%d].Name = %q, want %q (case-insensitive sort)", i, got[i].Name, want)
		}
	}
}

func TestComposingPreviewText(t *testing.T) {
	if got := composingPreviewText("recording"); got != "recording audio…" {
		t.Errorf("composingPreviewText(recording) = %q, want recording audio…", got)
	}
	if got := composingPreviewText("typing"); got != "typing…" {
		t.Errorf("composingPreviewText(typing) = %q, want typing…", got)
	}
	if got := composingPreviewText(""); got != "typing…" {
		t.Errorf("composingPreviewText(\"\") = %q, want typing… fallback", got)
	}
}

func TestStarredSnippet(t *testing.T) {
	cases := []struct {
		name string
		msg  client.Message
		want string
	}{
		{"text", client.Message{Text: "hello there"}, "hello there"},
		{"media falls back to media chip", client.Message{Attachment: &client.Attachment{Kind: "image", Caption: "trip"}}, "[image] trip"},
		{"location", client.Message{Location: &client.Location{Name: "Home"}}, "📍 Location"},
		{"contact", client.Message{Contact: &client.Contact{DisplayName: "Ada"}}, "👤 Contact"},
		{"poll", client.Message{Poll: &client.Poll{Name: "Lunch?"}}, "📊 Lunch?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := starredSnippet(tc.msg); got != tc.want {
				t.Errorf("starredSnippet(%+v) = %q, want %q", tc.msg, got, tc.want)
			}
		})
	}
}

func TestStatusRowVM(t *testing.T) {
	now := time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)

	t.Run("text status", func(t *testing.T) {
		vm := statusRowVM(client.Message{
			FromJID: "1234567890@s.whatsapp.net",
			Text:    "Off to the mountains!",
			TS:      now.Add(-2 * time.Hour).Unix(),
		}, "Ada Lovelace", now)
		if vm.PosterName != "Ada Lovelace" {
			t.Errorf("PosterName = %q, want Ada Lovelace", vm.PosterName)
		}
		if vm.Snippet != "Off to the mountains!" {
			t.Errorf("Snippet = %q, want the text", vm.Snippet)
		}
		if vm.Initial != "A" {
			t.Errorf("Initial = %q, want A", vm.Initial)
		}
		if vm.TimeText != "13:00" {
			t.Errorf("TimeText = %q, want 13:00", vm.TimeText)
		}
	})

	t.Run("image status shows photo placeholder", func(t *testing.T) {
		vm := statusRowVM(client.Message{
			FromJID:    "1234567890@s.whatsapp.net",
			Attachment: &client.Attachment{Kind: "image"},
			TS:         now.Unix(),
		}, "Grace", now)
		if vm.Snippet != "📷 Photo" {
			t.Errorf("Snippet = %q, want 📷 Photo", vm.Snippet)
		}
	})

	t.Run("video status shows video placeholder", func(t *testing.T) {
		vm := statusRowVM(client.Message{
			Attachment: &client.Attachment{Kind: "video"},
		}, "Grace", now)
		if vm.Snippet != "🎥 Video" {
			t.Errorf("Snippet = %q, want 🎥 Video", vm.Snippet)
		}
	})

	t.Run("empty poster name falls back to jid", func(t *testing.T) {
		vm := statusRowVM(client.Message{FromJID: "raw@jid"}, "", now)
		if vm.PosterName != "raw@jid" {
			t.Errorf("PosterName = %q, want raw@jid fallback", vm.PosterName)
		}
	})
}

func TestPosterName(t *testing.T) {
	names := map[string]string{"1234567890@s.whatsapp.net": "Ada"}
	if got := posterName("1234567890@s.whatsapp.net", names); got != "Ada" {
		t.Errorf("known chat: got %q, want Ada", got)
	}
	if got := posterName("5551234567@s.whatsapp.net", names); got != "+5551234567" {
		t.Errorf("unknown numeric: got %q, want +5551234567", got)
	}
	if got := posterName("weird@server", names); got != "weird@server" {
		t.Errorf("non-numeric: got %q, want raw jid", got)
	}
}
