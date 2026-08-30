package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestNewsletterRowVM(t *testing.T) {
	vm := newsletterRowVM(client.Newsletter{Name: "Chatot News", Description: "  Release notes  ", Muted: true})
	if vm.Name != "Chatot News" {
		t.Errorf("Name = %q", vm.Name)
	}
	if vm.Snippet != "Release notes" {
		t.Errorf("Snippet = %q, want trimmed", vm.Snippet)
	}
	if !vm.Muted {
		t.Errorf("Muted = false, want true")
	}
	if vm.Initial != "C" {
		t.Errorf("Initial = %q, want C", vm.Initial)
	}

	empty := newsletterRowVM(client.Newsletter{})
	if empty.Name != "Unknown channel" {
		t.Errorf("empty name fallback = %q", empty.Name)
	}
	if empty.Initial != "U" {
		t.Errorf("empty initial = %q", empty.Initial)
	}
}

func TestNewsletterPostVM(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-2 * time.Hour).Unix()
	m := client.NewsletterMessage{
		Text:      "Hello world",
		TS:        ts,
		Views:     42,
		Reactions: map[string]int{"👍": 8, "❤️": 3, "🎉": 3, "🔥": 1},
	}
	vm := newsletterPostVM(m, now)
	if vm.Text != "Hello world" {
		t.Errorf("Text = %q", vm.Text)
	}
	if vm.Views != "42 views" {
		t.Errorf("Views = %q", vm.Views)
	}
	if vm.TimeText != "10:00" {
		t.Errorf("TimeText = %q, want 10:00", vm.TimeText)
	}
	// Top three by count desc, emoji asc for ties (❤️ vs 🎉 both 3).
	if vm.Reactions != "👍 8   ❤️ 3   🎉 3" {
		t.Errorf("Reactions = %q", vm.Reactions)
	}
}

func TestNewsletterPostVMEmpty(t *testing.T) {
	now := time.Now()
	vm := newsletterPostVM(client.NewsletterMessage{Text: "   ", Views: 0}, now)
	if vm.Text != "(no text)" {
		t.Errorf("empty text = %q, want placeholder", vm.Text)
	}
	if vm.Reactions != "" {
		t.Errorf("empty reactions = %q", vm.Reactions)
	}
}

func TestMuteActionLabel(t *testing.T) {
	if muteActionLabel(true) != "Unmute" {
		t.Errorf("muted label wrong")
	}
	if muteActionLabel(false) != "Mute" {
		t.Errorf("unmuted label wrong")
	}
}
