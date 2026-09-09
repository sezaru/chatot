package ui

import (
	"strings"
	"testing"
)

// The catalogue is generated from the mockup; a half-written or truncated
// generation must not pass silently.
func TestEmojiCatalogueIsWhole(t *testing.T) {
	groups := emojiSet()
	if len(groups) != 9 {
		t.Fatalf("groups = %d, want the 9 Unicode groups", len(groups))
	}
	if n := emojiCount(); n < 1000 {
		t.Errorf("catalogue holds %d emoji, want the full set (~1045)", n)
	}
	for _, g := range groups {
		if g.Key == "" || g.Icon == "" || g.Label == "" {
			t.Errorf("group %+v is missing its key, icon or label", g)
		}
		if len(g.Items) < 20 {
			t.Errorf("group %q holds only %d emoji", g.Label, len(g.Items))
		}
		for _, e := range g.Items {
			if e.Char == "" || e.Name == "" {
				t.Errorf("group %q has an entry with no glyph or name: %+v", g.Label, e)
			}
			if strings.ContainsAny(e.Char, " \t") {
				t.Errorf("glyph %q in %q holds whitespace, so the line split wrongly", e.Char, g.Label)
			}
			if e.Keywords != strings.ToLower(e.Keywords) {
				t.Errorf("keywords for %q are not lower-cased: %q", e.Char, e.Keywords)
			}
		}
	}
}

func TestEmojiCatalogueHasNoDuplicates(t *testing.T) {
	if dupes := duplicateEmoji(); len(dupes) > 0 {
		t.Errorf("the same emoji appears in more than one group: %v", dupes)
	}
}

func TestSearchEmojiRanksNameMatchesFirst(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"thanks", "🙏"},
		{"coffee", "☕"},
		{"deal", "🤝"},
		{"rofl", "🤣"},
		{"portugal", "🇵🇹"},
		{"pizza", "🍕"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			hits := searchEmoji(c.query)
			if len(hits) == 0 {
				t.Fatalf("%q found nothing", c.query)
			}
			for _, h := range hits {
				if h.Char == c.want {
					return
				}
			}
			t.Errorf("%q did not find %s (got %s…)", c.query, c.want, hits[0].Char)
		})
	}
}

func TestSearchEmojiPutsAPrefixMatchAtTheTop(t *testing.T) {
	hits := searchEmoji("grinning")
	if len(hits) == 0 {
		t.Fatal("no hits for a name every smiley shares")
	}
	if !strings.HasPrefix(hits[0].Name, "grinning") {
		t.Errorf("top hit %q (%s) is not a name match", hits[0].Name, hits[0].Char)
	}
}

func TestSearchEmojiEmptyQueryMatchesNothing(t *testing.T) {
	if hits := searchEmoji("   "); hits != nil {
		t.Errorf("blank query returned %d hits", len(hits))
	}
}

func TestRememberEmojiMovesToTheFrontWithoutDuplicating(t *testing.T) {
	recents := []string{"👍", "❤️", "😂"}
	got := rememberEmoji(recents, "😂")
	if got[0] != "😂" {
		t.Errorf("picked emoji is not first: %v", got)
	}
	if len(got) != len(recents) {
		t.Errorf("recents grew on a repeat pick: %v", got)
	}
	got = rememberEmoji(got, "🎉")
	if got[0] != "🎉" || len(got) != 4 {
		t.Errorf("new emoji not prepended: %v", got)
	}
}

func TestRememberEmojiKeepsTheListShort(t *testing.T) {
	var recents []string
	for _, g := range emojiSet()[0].Items {
		recents = rememberEmoji(recents, g.Char)
	}
	if len(recents) != emojiRecentsMax {
		t.Errorf("recents = %d entries, want the %d cap", len(recents), emojiRecentsMax)
	}
}

func TestRecentEmojiEntriesDropsUnknownGlyphs(t *testing.T) {
	got := recentEmojiEntries([]string{"👍", "not-an-emoji", "🎉"})
	if len(got) != 2 {
		t.Fatalf("resolved %d entries, want 2: %+v", len(got), got)
	}
	if got[0].Char != "👍" || got[1].Char != "🎉" {
		t.Errorf("order lost: %+v", got)
	}
}

func TestDefaultRecentsAreAllInTheCatalogue(t *testing.T) {
	if got := recentEmojiEntries(defaultEmojiRecents); len(got) != len(defaultEmojiRecents) {
		t.Errorf("seeded recents resolve to %d of %d entries", len(got), len(defaultEmojiRecents))
	}
}

func TestEmojiGroupKeysStartWithRecents(t *testing.T) {
	keys := emojiGroupKeys()
	if len(keys) != len(emojiSet())+1 {
		t.Fatalf("rail has %d entries for %d groups", len(keys), len(emojiSet()))
	}
	if keys[0] != emojiRecentKey {
		t.Errorf("rail starts with %q, want the recents", keys[0])
	}
}
