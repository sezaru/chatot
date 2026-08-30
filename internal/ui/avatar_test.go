package ui

import "testing"

func TestAvatarCacheGetSetInvalidate(t *testing.T) {
	c := newAvatarCache()

	if _, resolved := c.get("a@s.whatsapp.net"); resolved {
		t.Fatal("get on empty cache reported resolved")
	}

	c.set("a@s.whatsapp.net", "/tmp/a.jpg")
	path, resolved := c.get("a@s.whatsapp.net")
	if !resolved || path != "/tmp/a.jpg" {
		t.Fatalf("get after set = (%q, %v), want (/tmp/a.jpg, true)", path, resolved)
	}

	// A "known empty" entry is still resolved (don't re-fetch), just with "".
	c.set("b@s.whatsapp.net", "")
	path, resolved = c.get("b@s.whatsapp.net")
	if !resolved || path != "" {
		t.Fatalf("get after set empty = (%q, %v), want (\"\", true)", path, resolved)
	}

	c.invalidate("a@s.whatsapp.net")
	if _, resolved := c.get("a@s.whatsapp.net"); resolved {
		t.Fatal("get after invalidate still reported resolved")
	}
}

func TestInitialFor(t *testing.T) {
	cases := []struct{ name, want string }{
		{"alice", "A"},
		{"Bob", "B"},
		{"", "?"},
		{"éclair", "É"},
	}
	for _, c := range cases {
		if got := initialFor(c.name); got != c.want {
			t.Errorf("initialFor(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
