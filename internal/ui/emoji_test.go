package ui

import "testing"

func TestIsEmojiOnly(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"👍", true},
		{"👍😂", true},
		{"👍😂😢", true},
		{"👍😂😢😮", false}, // 4 emoji: too many
		{"", false},
		{"hi", false},
		{"👍 nice", false},
		{"  👍  ", true},     // surrounding whitespace is fine
		{"❤️", true},        // heart + variation selector counts as one cluster
		{"🇺🇸", true},        // flag: two regional indicators, one cluster
		{"🇺🇸🇬🇧", true},      // two flags, two clusters
		{"🇺🇸🇬🇧🇯🇵🇩🇪", false}, // 4 flags: too many, must not collapse into one cluster
		{"👨‍👩‍👧", true},     // ZWJ family sequence, one cluster
		{"👍🏽", true},        // skin-tone modifier stays in the same cluster
	}
	for _, tc := range cases {
		if got := isEmojiOnly(tc.text); got != tc.want {
			t.Errorf("isEmojiOnly(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
