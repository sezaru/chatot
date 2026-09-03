package ui

import (
	"testing"
	"unicode/utf8"
)

func TestFindMatchesCaseInsensitiveNonOverlapping(t *testing.T) {
	got := findMatches("Hello hello HELLO world", "hello")
	want := []matchRange{{0, 5}, {6, 11}, {12, 17}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("range %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestFindMatchesEmptyQueryMatchesNothing(t *testing.T) {
	if got := findMatches("anything", ""); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestFindMatchesNoOccurrence(t *testing.T) {
	if got := findMatches("pizza night", "sushi"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// "İ" (U+0130, 2 bytes) folds to 1-byte "i", so offsets from a lower-cased
// copy would be misaligned; findMatches must return byte offsets into the
// original text on real rune boundaries.
func TestFindMatchesLengthChangingFold(t *testing.T) {
	text := "İstanbul" // İ is 2 bytes; "st" occupy bytes 2..4
	got := findMatches(text, "ist")
	if len(got) != 1 {
		t.Fatalf("got %v, want exactly one match", got)
	}
	if got[0].Start != 0 || got[0].End != 4 {
		t.Fatalf("got range %v, want {0, 4} (İst in original bytes)", got[0])
	}
	// The sliced region must be a valid rune boundary — İ + "st".
	seg := text[got[0].Start:got[0].End]
	if !utf8.ValidString(seg) || seg != "İst" {
		t.Errorf("got segment %q (valid=%v), want %q", seg, utf8.ValidString(seg), "İst")
	}
}

func TestHighlightMarkupLengthChangingFoldValidUTF8(t *testing.T) {
	got := highlightMarkup("İstanbul", "ist")
	if !utf8.ValidString(got) {
		t.Fatalf("markup is not valid UTF-8: %q", got)
	}
	want := `<span background="#f5c518" foreground="#1b1b1b">İst</span>anbul`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHighlightMarkupWrapsMatchesAndEscapesRest(t *testing.T) {
	got := highlightMarkup("A & B match", "match")
	want := `A &amp; B <span background="#f5c518" foreground="#1b1b1b">match</span>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHighlightMarkupNoMatchStillEscapes(t *testing.T) {
	got := highlightMarkup("<tag> & 'quote'", "nomatch")
	want := "&lt;tag&gt; &amp; &#39;quote&#39;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSearchHitStepWrapsForwardAndBackward(t *testing.T) {
	cases := []struct {
		cur, total int
		forward    bool
		want       int
	}{
		{-1, 3, true, 0},
		{-1, 3, false, 2},
		{0, 3, true, 1},
		{2, 3, true, 0},
		{0, 3, false, 2},
		{1, 3, false, 0},
		{0, 0, true, -1},
	}
	for _, c := range cases {
		got := searchHitStep(c.cur, c.total, c.forward)
		if got != c.want {
			t.Errorf("searchHitStep(%d, %d, %v) = %d, want %d", c.cur, c.total, c.forward, got, c.want)
		}
	}
}

func TestSearchHitCountText(t *testing.T) {
	if got := searchHitCountText(0, 0); got != "0/0" {
		t.Errorf("got %q, want %q", got, "0/0")
	}
	if got := searchHitCountText(2, 6); got != "3/6" {
		t.Errorf("got %q, want %q", got, "3/6")
	}
}
