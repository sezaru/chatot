package ui

import (
	"strings"
	"testing"
)

func TestTabBadgeText(t *testing.T) {
	cases := map[int]string{0: "", -1: "", 1: "1", 99: "99", 100: "99+", 1200: "99+"}
	for n, want := range cases {
		if got := tabBadgeText(n); got != want {
			t.Errorf("tabBadgeText(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestSidebarTabsMatchMockup(t *testing.T) {
	want := []string{"Chats", "Status", "Channels", "Communities"}
	if len(sidebarTabs) != len(want) {
		t.Fatalf("sidebarTabs has %d tabs, want %d", len(sidebarTabs), len(want))
	}
	for i, d := range sidebarTabs {
		if d.Label != want[i] {
			t.Errorf("tab %d = %q, want %q", i, d.Label, want[i])
		}
		if d.D1 == "" {
			t.Errorf("tab %q has no icon path", d.ID)
		}
	}
}

func TestTabIconSVG(t *testing.T) {
	svg := string(tabIconSVG(sidebarTabs[1], "#147a63", 2))
	for _, frag := range []string{`stroke="#147a63"`, `stroke-width="2"`, `stroke-dasharray="14.6 3.1"`, `viewBox="0 0 24 24"`} {
		if !strings.Contains(svg, frag) {
			t.Errorf("status icon SVG lacks %s:\n%s", frag, svg)
		}
	}
	// A tab without a dash pattern must not emit an empty attribute.
	if strings.Contains(string(tabIconSVG(sidebarTabs[0], "#000", 1.6)), "stroke-dasharray") {
		t.Errorf("chats icon SVG has a dash pattern")
	}
}
