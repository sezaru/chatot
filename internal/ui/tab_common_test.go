package ui

import "testing"

func TestCompactCount(t *testing.T) {
	cases := map[int]string{
		0: "0", 986: "986", 4218: "4,218", 9860: "9,860", 27100: "27.1K", 38400: "38.4K",
		96000: "96K", 182000: "182K", 244000: "244K", 612000: "612K", 1204: "1,204", 2500000: "2.5M",
	}
	for n, want := range cases {
		if got := compactCount(n); got != want {
			t.Errorf("compactCount(%d) = %q, want %q", n, got, want)
		}
	}
	if got := followersText(4218); got != "4,218 followers" {
		t.Errorf("followersText = %q", got)
	}
	if got := viewsText(1); got != "1 view" {
		t.Errorf("viewsText(1) = %q", got)
	}
}

func TestTabEmptyCopy(t *testing.T) {
	for _, tab := range []string{"status", "channels", "communities"} {
		glyph, title, text := tabEmptyCopy(tab)
		if glyph == "" || title == "" || text == "" {
			t.Errorf("tabEmptyCopy(%q) incomplete", tab)
		}
	}
}

func TestPluralCount(t *testing.T) {
	if got := pluralCount(1, "update", "updates"); got != "1 update" {
		t.Errorf("got %q", got)
	}
	if got := pluralCount(3, "update", "updates"); got != "3 updates" {
		t.Errorf("got %q", got)
	}
}

func TestIsOwnJID(t *testing.T) {
	if !isOwnJID("1234567890:12@s.whatsapp.net", "1234567890@s.whatsapp.net") {
		t.Errorf("device suffix should not matter")
	}
	if isOwnJID("", "1234567890@s.whatsapp.net") || isOwnJID("x@s.whatsapp.net", "") {
		t.Errorf("empty jids are never own")
	}
}
