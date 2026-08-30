package ui

import "testing"

func TestStickerRecentsAddDedupsAndMovesToFront(t *testing.T) {
	r := newStickerRecents(8)
	r.Add("a.webp")
	r.Add("b.webp")
	r.Add("a.webp")

	got := r.Items()
	want := []string{"a.webp", "b.webp"}
	if len(got) != len(want) {
		t.Fatalf("Items() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Items()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStickerRecentsCapsAtN(t *testing.T) {
	r := newStickerRecents(2)
	r.Add("a.webp")
	r.Add("b.webp")
	r.Add("c.webp")

	got := r.Items()
	want := []string{"c.webp", "b.webp"}
	if len(got) != len(want) {
		t.Fatalf("Items() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Items()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStickerRecentsItemsIsACopy(t *testing.T) {
	r := newStickerRecents(4)
	r.Add("a.webp")
	got := r.Items()
	got[0] = "mutated"
	if r.Items()[0] != "a.webp" {
		t.Fatal("Items() returned a slice aliasing internal state")
	}
}
