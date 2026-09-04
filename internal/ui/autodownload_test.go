package ui

import (
	"testing"
	"time"
)

func TestAutoDownloadWants(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-time.Hour).Unix()
	old := now.Add(-30 * 24 * time.Hour).Unix()
	cases := []struct {
		mode, kind string
		ts         int64
		want       bool
	}{
		{"always", "document", fresh, true},
		{"always", "video", old, false},
		{"photos", "image", fresh, true},
		{"photos", "sticker", fresh, true},
		{"photos", "audio", fresh, true},
		{"photos", "video", fresh, false},
		{"photos", "document", fresh, false},
		{"never", "image", fresh, false},
		{"photos", "image", 0, false},
	}
	for _, c := range cases {
		if got := autoDownloadWants(c.mode, c.kind, c.ts, now); got != c.want {
			t.Errorf("autoDownloadWants(%q, %q, age %v) = %v, want %v", c.mode, c.kind, c.ts, got, c.want)
		}
	}
}
