package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestTraySendLabel(t *testing.T) {
	// One file reads "Send"; the count only appears once it means something.
	if got := traySendLabel(1); got != "Send" {
		t.Errorf("traySendLabel(1) = %q", got)
	}
	if got := traySendLabel(2); got != "Send 2" {
		t.Errorf("traySendLabel(2) = %q", got)
	}
	if got := traySendLabel(0); got != "Send" {
		t.Errorf("traySendLabel(0) = %q", got)
	}
}

func TestTrayPreviewable(t *testing.T) {
	for _, path := range []string{"a.png", "b.JPG", "c.jpeg", "d.webp", "e.gif"} {
		if !trayPreviewable(path) {
			t.Errorf("trayPreviewable(%q) = false", path)
		}
	}
	for _, path := range []string{"a.pdf", "b.mp4", "c", "d.ogg"} {
		if trayPreviewable(path) {
			t.Errorf("trayPreviewable(%q) = true", path)
		}
	}
}

func TestTrayGlyph(t *testing.T) {
	tests := map[string]string{
		"clip.mp4":   "🎬",
		"note.ogg":   "🎵",
		"lease.pdf":  "📕",
		"bundle.zip": "🗜",
		"notes.txt":  "📄",
		"noext":      "📄",
	}
	for path, want := range tests {
		if got := trayGlyph(path); got != want {
			t.Errorf("trayGlyph(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestTrayMetaUnknownFile(t *testing.T) {
	// A path that doesn't exist has no size, so the meta line is the type
	// alone rather than "PDF · " with a dangling separator.
	if got, want := trayMeta("/nonexistent/lease.pdf"), "PDF"; got != want {
		t.Errorf("trayMeta = %q, want %q", got, want)
	}
	if got := trayMeta("/nonexistent/noext"); got != "" {
		t.Errorf("trayMeta(no ext, no file) = %q, want empty", got)
	}
}

func TestAttachSourcesMatchMockup(t *testing.T) {
	want := []string{"Photos", "Video", "Document", "Contact", "Location", "Poll", "Audio file"}
	got := attachSources()
	if len(got) != len(want) {
		t.Fatalf("attachSources() has %d rows, want %d", len(got), len(want))
	}
	for i, label := range want {
		if got[i].Label != label {
			t.Errorf("row %d = %q, want %q", i, got[i].Label, label)
		}
		if got[i].Icon == "" || got[i].Tint == "" {
			t.Errorf("row %q missing icon or tint", label)
		}
	}
}

func TestPickerPagesMatchMockup(t *testing.T) {
	want := []string{"Emoji", "GIF", "Stickers"}
	if len(pickerPages) != len(want) {
		t.Fatalf("pickerPages has %d tabs, want %d", len(pickerPages), len(want))
	}
	for i, label := range want {
		if pickerPages[i].Label != label {
			t.Errorf("tab %d = %q, want %q", i, pickerPages[i].Label, label)
		}
	}
}

func TestReplyAuthorName(t *testing.T) {
	if got := replyAuthorName(client.Message{FromMe: true}, "Ada"); got != "You" {
		t.Errorf("own message = %q, want You", got)
	}
	if got := replyAuthorName(client.Message{}, "Ada"); got != "Ada" {
		t.Errorf("their message = %q, want Ada", got)
	}
	// No chat name yet (a reply armed before SetChatName lands): the bar still
	// shows an author line rather than an empty accent row.
	if got := replyAuthorName(client.Message{}, ""); got != "Reply" {
		t.Errorf("nameless = %q, want Reply", got)
	}
}

func TestRecordingClock(t *testing.T) {
	// A just-started recording reads 0:00, not blank the way an unknown media
	// duration does.
	if got := recordingClock(0); got != "0:00" {
		t.Errorf("recordingClock(0) = %q", got)
	}
	if got := recordingClock(7_000_000_000); got != "0:07" {
		t.Errorf("recordingClock(7s) = %q", got)
	}
	if got := recordingClock(75_000_000_000); got != "1:15" {
		t.Errorf("recordingClock(75s) = %q", got)
	}
}

func TestTraySelectionFollowsRemoval(t *testing.T) {
	// Removing an item BEFORE the selection must shift the index down, or the
	// preview silently switches to a different queued file.
	tray := &AttachTray{
		items: []trayItem{
			{Path: "a.png"}, {Path: "b.png"}, {Path: "c.png"}, {Path: "d.png"},
		},
		selected: 3,
	}
	tray.removeAt(0)
	if got, want := tray.items[tray.selected].Path, "d.png"; got != want {
		t.Errorf("after removing an earlier item, selected = %q, want %q", got, want)
	}

	// Removing the selected (last) item clamps back into range.
	tray.removeAt(tray.selected)
	if tray.selected != len(tray.items)-1 {
		t.Errorf("selected = %d, want %d", tray.selected, len(tray.items)-1)
	}

	// Removing something after the selection leaves it alone.
	tray.selected = 0
	tray.removeAt(1)
	if tray.selected != 0 {
		t.Errorf("selected = %d, want 0", tray.selected)
	}

	// Out-of-range indices are ignored rather than panicking.
	before := len(tray.items)
	tray.removeAt(-1)
	tray.removeAt(99)
	if len(tray.items) != before {
		t.Errorf("out-of-range remove changed the queue")
	}
}
