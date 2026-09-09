package ui

import (
	"sort"
	"strings"
	"sync"
)

// emojiGroupSource is one group as the catalogue stores it: a key for the
// rail, the glyph that stands for the group, its heading, and the block of
// "<emoji> <name>:<extra words>" lines behind it.
type emojiGroupSource struct {
	Key, Icon, Label, Src string
}

// emojiEntry is a single emoji ready for the picker: the glyph, the name the
// tooltip shows, and the lower-case words a search matches against.
type emojiEntry struct {
	Char, Name, Keywords string
}

// emojiGroup is a parsed group.
type emojiGroup struct {
	Key, Icon, Label string
	Items            []emojiEntry
}

var (
	emojiOnce   sync.Once
	emojiParsed []emojiGroup
	emojiByChar map[string]emojiEntry
)

// emojiSet parses the catalogue once and hands back the groups in order.
func emojiSet() []emojiGroup {
	emojiOnce.Do(parseEmojiCatalogue)
	return emojiParsed
}

// emojiNamed is the catalogue entry for glyph, and whether the catalogue
// has one at all (a recent picked up from an older build might not be).
func emojiNamed(glyph string) (emojiEntry, bool) {
	emojiOnce.Do(parseEmojiCatalogue)
	e, ok := emojiByChar[glyph]
	return e, ok
}

func parseEmojiCatalogue() {
	emojiByChar = make(map[string]emojiEntry, 1100)
	emojiParsed = make([]emojiGroup, 0, len(emojiCatalogue))
	for _, src := range emojiCatalogue {
		g := emojiGroup{Key: src.Key, Icon: src.Icon, Label: src.Label}
		for _, line := range strings.Split(strings.TrimSpace(src.Src), "\n") {
			line = strings.TrimSpace(line)
			sp := strings.IndexByte(line, ' ')
			if sp <= 0 {
				continue
			}
			glyph, rest := line[:sp], line[sp+1:]
			name := rest
			if cut := strings.IndexByte(rest, ':'); cut >= 0 {
				name = rest[:cut]
			}
			e := emojiEntry{
				Char:     glyph,
				Name:     name,
				Keywords: strings.ToLower(strings.ReplaceAll(rest, ":", " ")),
			}
			g.Items = append(g.Items, e)
			if _, seen := emojiByChar[glyph]; !seen {
				emojiByChar[glyph] = e
			}
		}
		emojiParsed = append(emojiParsed, g)
	}
}

// searchEmoji is the picker's search: everything whose name or keywords hold
// the query, with the entries whose *name* starts with it first, so typing
// "thanks" puts 🙏 at the top instead of somewhere down the list. An empty
// query matches nothing — the caller shows the groups instead.
func searchEmoji(query string) []emojiEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var starts, rest []emojiEntry
	for _, g := range emojiSet() {
		for _, e := range g.Items {
			switch {
			case strings.HasPrefix(e.Name, q):
				starts = append(starts, e)
			case strings.Contains(e.Keywords, q):
				rest = append(rest, e)
			}
		}
	}
	return append(starts, rest...)
}

// emojiRecents is the frequently-used row both pickers share, most recent
// first. It is seeded so the row is never empty on a fresh install, and
// capped so it stays one or two rows.
const emojiRecentsMax = 24

var defaultEmojiRecents = []string{
	"👍", "❤️", "😂", "🙏", "🔥", "🎉", "😍", "😢",
	"👏", "🤝", "☕", "📌", "✅", "🥐", "🐦", "🏔️",
}

// recentEmojis is the live frequently-used list both pickers read and both
// pickers feed. It starts from the seed so the row is never empty, and
// SetRecentEmojis replaces it with what the settings file remembered.
var recentEmojis = append([]string(nil), defaultEmojiRecents...)

// SaveRecentEmojis persists the list after a pick; main.go points it at the
// settings file. Nil keeps the list to this session.
var SaveRecentEmojis func([]string)

// SetRecentEmojis restores the remembered list at startup. An empty or
// unusable list leaves the seed in place.
func SetRecentEmojis(list []string) {
	if len(recentEmojiEntries(list)) == 0 {
		return
	}
	recentEmojis = append([]string(nil), list...)
}

// recentEmojiList is the current frequently-used order.
func recentEmojiList() []string { return recentEmojis }

// rememberRecentEmoji records a pick from either picker and persists it.
func rememberRecentEmoji(glyph string) {
	recentEmojis = rememberEmoji(recentEmojis, glyph)
	if SaveRecentEmojis != nil {
		SaveRecentEmojis(recentEmojis)
	}
}

// rememberEmoji moves glyph to the front of recents and returns the new
// list, dropping the tail past the cap. Pure, so the caller decides what to
// do with it (keep it in memory, write it to the settings file).
func rememberEmoji(recents []string, glyph string) []string {
	out := make([]string, 0, len(recents)+1)
	out = append(out, glyph)
	for _, r := range recents {
		if r != glyph {
			out = append(out, r)
		}
	}
	if len(out) > emojiRecentsMax {
		out = out[:emojiRecentsMax]
	}
	return out
}

// recentEmojiEntries resolves recents to catalogue entries, dropping any the
// catalogue no longer carries.
func recentEmojiEntries(recents []string) []emojiEntry {
	out := make([]emojiEntry, 0, len(recents))
	for _, glyph := range recents {
		if e, ok := emojiNamed(glyph); ok {
			out = append(out, e)
		}
	}
	return out
}

// emojiGroupKeys is the rail's order: frequently used, then the catalogue.
func emojiGroupKeys() []string {
	keys := []string{emojiRecentKey}
	for _, g := range emojiSet() {
		keys = append(keys, g.Key)
	}
	return keys
}

// emojiRecentKey names the recents group, which has no catalogue block of
// its own.
const emojiRecentKey = "recent"

// emojiCount is the catalogue's size, for the tests that guard against a
// generated file arriving half-empty.
func emojiCount() int {
	n := 0
	for _, g := range emojiSet() {
		n += len(g.Items)
	}
	return n
}

// duplicateEmoji lists any glyph the catalogue carries in more than one
// group — a picker must not show the same emoji twice.
func duplicateEmoji() []string {
	seen := map[string]int{}
	for _, g := range emojiSet() {
		for _, e := range g.Items {
			seen[e.Char]++
		}
	}
	var dupes []string
	for glyph, n := range seen {
		if n > 1 {
			dupes = append(dupes, glyph)
		}
	}
	sort.Strings(dupes)
	return dupes
}
