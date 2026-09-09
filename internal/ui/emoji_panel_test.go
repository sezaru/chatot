package ui

import "testing"

// The band names the group being scrolled through, and stays away while
// that group's own heading is still on screen.
func TestPinnedEmojiSection(t *testing.T) {
	// Frequently used at 0, Smileys at 200, People at 900.
	offsets := []float64{0, 200, 900}
	cases := []struct {
		name string
		v    float64
		want int
	}{
		{"at rest on the first heading", 0, -1},
		{"a pixel into the first group", 1, 0},
		{"deep in the first group", 199, 0},
		{"exactly on the second heading", 200, -1},
		{"just past the second heading", 201, 1},
		{"deep in the second group", 899, 1},
		{"exactly on the third heading", 900, -1},
		{"past the last heading", 1200, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pinnedEmojiSection(offsets, c.v); got != c.want {
				t.Errorf("pinnedEmojiSection(%v) = %d, want %d", c.v, got, c.want)
			}
		})
	}
}

func TestPinnedEmojiSectionWithNoSections(t *testing.T) {
	if got := pinnedEmojiSection(nil, 0); got != -1 {
		t.Errorf("empty list pinned section %d, want none", got)
	}
	if got := pinnedEmojiSection([]float64{}, 40); got != -1 {
		t.Errorf("empty list pinned section %d, want none", got)
	}
}

// A search shows a single section; scrolling its hits should still name it.
func TestPinnedEmojiSectionSingleSection(t *testing.T) {
	offsets := []float64{0}
	if got := pinnedEmojiSection(offsets, 0); got != -1 {
		t.Errorf("at rest = %d, want none", got)
	}
	if got := pinnedEmojiSection(offsets, 120); got != 0 {
		t.Errorf("scrolled = %d, want the only section", got)
	}
}

// Half a pixel of scroll is the list sitting still, not a nudge into the
// group: sub-pixel offsets must not flicker the band on.
func TestPinnedEmojiSectionIgnoresSubPixelScroll(t *testing.T) {
	offsets := []float64{0, 200}
	if got := pinnedEmojiSection(offsets, 0.4); got != -1 {
		t.Errorf("sub-pixel scroll pinned section %d, want none", got)
	}
	if got := pinnedEmojiSection(offsets, 200.4); got != -1 {
		t.Errorf("sub-pixel scroll past a heading pinned %d, want none", got)
	}
}
