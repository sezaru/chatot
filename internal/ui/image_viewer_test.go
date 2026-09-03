package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestImageViewerSubtitle(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.Local)
	ts := time.Date(2026, 9, 3, 11, 13, 0, 0, time.Local).Unix()
	if got := imageViewerSubtitle(client.Message{FromMe: true, TS: ts}, now); got != "You · Today, 11:13" {
		t.Fatalf("own: %q", got)
	}
	if got := imageViewerSubtitle(client.Message{TS: ts - 86400}, now); got != "Yesterday, 11:13" {
		t.Fatalf("theirs: %q", got)
	}
}

func TestSuggestedImageName(t *testing.T) {
	ts := time.Date(2026, 9, 3, 11, 13, 5, 0, time.Local).Unix()
	if got := suggestedImageName(client.Message{TS: ts}, "/cache/abc.JPG"); got != "photo-2026-09-03-111305.jpg" {
		t.Fatalf("derived: %q", got)
	}
	if got := suggestedImageName(client.Message{TS: ts, Attachment: &client.Attachment{Filename: "cat.png"}}, "/cache/abc.png"); got != "cat.png" {
		t.Fatalf("named: %q", got)
	}
}
