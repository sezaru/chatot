package ui

import (
	"strings"
	"testing"
)

func TestIsLongText(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"empty", "", false},
		{"short", "Two croissant trays for Friday", false},
		{"at the character limit", strings.Repeat("a", longTextMaxChars), false},
		{"past the character limit", strings.Repeat("a", longTextMaxChars+1), true},
		{"a wall of short lines", strings.Repeat("x\n", longTextMaxLines+1), true},
		{"few lines, few characters", "one\ntwo\nthree", false},
		// Runes, not bytes: a message of accented characters is not long
		// just because it is long in UTF-8.
		{"accents count once", strings.Repeat("á", longTextMaxChars-1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLongText(tc.text); got != tc.want {
				t.Errorf("isLongText = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClipTextEndsOnAWordBoundary(t *testing.T) {
	body := strings.Repeat("landlord keys tomorrow ", 40)
	got := clipText(body)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("clip does not end with an ellipsis: %q", got[len(got)-20:])
	}
	trimmed := strings.TrimSuffix(got, "…")
	if strings.HasSuffix(trimmed, " ") {
		t.Errorf("clip keeps trailing whitespace: %q", got)
	}
	// The cut must land where a word ends, so the preview never shows half
	// a word.
	if !strings.HasSuffix(trimmed, "landlord") && !strings.HasSuffix(trimmed, "keys") &&
		!strings.HasSuffix(trimmed, "tomorrow") {
		t.Errorf("clip cut mid-word: %q", trimmed[len(trimmed)-20:])
	}
	if n := len([]rune(trimmed)); n > longTextCut {
		t.Errorf("clip kept %d runes, over the %d cut", n, longTextCut)
	}
}

func TestClipTextCutsAWallOfLinesByLineCount(t *testing.T) {
	body := strings.Repeat("x\n", longTextMaxLines+6)
	got := clipText(body)
	if lines := strings.Count(got, "\n") + 1; lines > longTextMaxLines {
		t.Errorf("clip kept %d lines, over the %d cap", lines, longTextMaxLines)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("clip does not end with an ellipsis: %q", got)
	}
}

func TestClipTextLeavesAShortBodyAlone(t *testing.T) {
	body := "Picked them up this morning"
	if got := clipText(body); got != body {
		t.Errorf("clipText(%q) = %q, want it unchanged", body, got)
	}
}

func TestReadMoreLabelFlips(t *testing.T) {
	if got := readMoreLabel(false); got != "Read more" {
		t.Errorf("folded label = %q", got)
	}
	if got := readMoreLabel(true); got != "Show less" {
		t.Errorf("unfolded label = %q", got)
	}
}

func TestSetTextExpandedRemembersAndForgets(t *testing.T) {
	cv := &ConversationView{}
	if cv.textExpanded("m1") {
		t.Fatal("a body starts folded")
	}
	cv.setTextExpanded("m1", true)
	if !cv.textExpanded("m1") {
		t.Error("unfolded body not remembered")
	}
	cv.setTextExpanded("m1", false)
	if cv.textExpanded("m1") {
		t.Error("refolded body still remembered")
	}
	if len(cv.expandedText) != 0 {
		t.Errorf("refolding leaks an entry: %v", cv.expandedText)
	}
}
