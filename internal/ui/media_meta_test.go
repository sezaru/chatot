package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestDocKindLabel(t *testing.T) {
	tests := []struct{ mime, want string }{
		{"application/pdf", "PDF"},
		{"image/svg+xml", "SVG"},
		{"", "FILE"},
		{"noSlash", "FILE"},
	}
	for _, tt := range tests {
		if got := docKindLabel(tt.mime); got != tt.want {
			t.Errorf("docKindLabel(%q) = %q, want %q", tt.mime, got, tt.want)
		}
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1258291, "1.2 MB"},
	}
	for _, tt := range tests {
		if got := formatFileSize(tt.n); got != tt.want {
			t.Errorf("formatFileSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestLinkSubtitle(t *testing.T) {
	l := client.LinkItem{Host: "stay.example.com", TS: time.Date(2026, time.August, 14, 12, 0, 0, 0, time.Local).Unix()}
	got := linkSubtitle(l)
	if got != "stay.example.com · Aug 14, 2026" {
		t.Errorf("linkSubtitle = %q", got)
	}
}

func TestMediaTileLabelFor(t *testing.T) {
	now := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	// Inside the last week the mockup labels a tile with its weekday…
	yesterday := now.Add(-24 * time.Hour).Unix()
	if got, want := mediaTileLabelFor(yesterday, now), "Tue"; got != want {
		t.Errorf("recent = %q, want %q", got, want)
	}
	// …and falls back to a short date beyond it.
	old := now.Add(-30 * 24 * time.Hour).Unix()
	if got, want := mediaTileLabelFor(old, now), "3 Aug"; got != want {
		t.Errorf("old = %q, want %q", got, want)
	}
	// An item with no timestamp gets no caption rather than a 1970 date.
	if got := mediaTileLabelFor(0, now); got != "" {
		t.Errorf("zero ts = %q, want empty", got)
	}
}

func TestStarredCountLabel(t *testing.T) {
	if got, want := starredCountLabel(1), "1 starred"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
	if got, want := starredCountLabel(0), "0 starred"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
}

func TestMediaPanePagesMatchMockup(t *testing.T) {
	want := []string{"Media", "Links", "Docs"}
	if len(mediaPanePages) != len(want) {
		t.Fatalf("mediaPanePages has %d tabs, want %d", len(mediaPanePages), len(want))
	}
	for i, label := range want {
		if mediaPanePages[i].Label != label {
			t.Errorf("tab %d = %q, want %q", i, mediaPanePages[i].Label, label)
		}
	}
}
