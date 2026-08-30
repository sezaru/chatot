package ui

// isEmojiOnly reports whether text is nothing but 1-3 emoji (whitespace
// aside) — the mockup's "emoji-only message" case, rendered large with no
// bubble. Sequences joined by ZWJ (families), flag pairs (two regional
// indicators) and skin-tone modifiers count as a single emoji.
func isEmojiOnly(text string) bool {
	clusters, ok := emojiClusters(text)
	return ok && len(clusters) >= 1 && len(clusters) <= 3
}

// emojiClusters splits text into emoji clusters. ok is false if text is
// empty or contains any non-emoji, non-whitespace rune.
func emojiClusters(text string) (clusters []string, ok bool) {
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if isEmojiSpace(r) {
			i++
			continue
		}
		if !isEmojiRune(r) {
			return nil, false
		}
		j := i + 1
	absorb:
		for j < len(runes) {
			nr := runes[j]
			switch {
			case nr == 0xFE0F || nr == 0xFE0E: // variation selectors
				j++
			case nr == 0x200D && j+1 < len(runes) && isEmojiRune(runes[j+1]): // ZWJ joiner
				j += 2
			case isSkinToneModifier(nr):
				j++
			case isRegionalIndicator(r) && isRegionalIndicator(nr) && j == i+1: // flag pair: absorb exactly the second indicator, no more
				j++
			default:
				break absorb
			}
		}
		clusters = append(clusters, string(runes[i:j]))
		i = j
	}
	if len(clusters) == 0 {
		return nil, false
	}
	return clusters, true
}

func isEmojiSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isSkinToneModifier(r rune) bool {
	return r >= 0x1F3FB && r <= 0x1F3FF
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

// isEmojiRune covers the common emoji blocks: emoticons, misc
// symbols/pictographs, transport, supplemental symbols and pictographs,
// misc symbols and dingbats, regional indicators (flags), and misc
// symbols/arrows containing stars/hearts.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x1F1E6 && r <= 0x1F1FF:
		return true
	case r >= 0x2B00 && r <= 0x2BFF:
		return true
	default:
		return false
	}
}
