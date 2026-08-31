package ui

import "testing"

func TestTrayTooltip(t *testing.T) {
	cases := []struct {
		unread int
		want   string
	}{
		{-1, "chatot"},
		{0, "chatot"},
		{1, "chatot — 1 unread"},
		{42, "chatot — 42 unread"},
	}
	for _, tc := range cases {
		if got := trayTooltip(tc.unread); got != tc.want {
			t.Errorf("trayTooltip(%d) = %q, want %q", tc.unread, got, tc.want)
		}
	}
}
