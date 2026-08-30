package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func tsAt(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 12, 0, 0, 0, time.Local).Unix()
}

func TestGroupMediaByMonth(t *testing.T) {
	// 2026-08-16, 2026-08-02, 2026-07-20, newest first (as ChatMedia returns).
	items := []client.MediaItem{
		{MsgID: "a", TS: tsAt(2026, time.August, 16)},
		{MsgID: "b", TS: tsAt(2026, time.August, 2)},
		{MsgID: "c", TS: tsAt(2026, time.July, 20)},
	}
	groups := groupMediaByMonth(items)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2: %+v", len(groups), groups)
	}
	if groups[0].Header != "AUGUST 2026" || len(groups[0].Items) != 2 {
		t.Errorf("group[0] = %+v, want AUGUST 2026 with 2 items", groups[0])
	}
	if groups[1].Header != "JULY 2026" || len(groups[1].Items) != 1 {
		t.Errorf("group[1] = %+v, want JULY 2026 with 1 item", groups[1])
	}
}

func TestGroupMediaByMonthEmpty(t *testing.T) {
	if got := groupMediaByMonth(nil); got != nil {
		t.Errorf("groupMediaByMonth(nil) = %+v, want nil", got)
	}
}

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
	l := client.LinkItem{Host: "stay.example.com", TS: tsAt(2026, time.August, 14)}
	got := linkSubtitle(l)
	if got != "stay.example.com · Aug 14, 2026" {
		t.Errorf("linkSubtitle = %q", got)
	}
}
