package ui

import "strings"

// A long message body is folded in the bubble behind a "Read more", the way
// WhatsApp folds one: the cut lands on a word boundary so the preview never
// breaks mid-word, and a wall of pasted lines is capped by line count even
// when it is short on characters. The numbers are the mockup's: about six
// lines in a bubble at its widest.
const (
	longTextMaxChars = 420
	longTextMaxLines = 8
	longTextCut      = 370
)

// isLongText reports whether text is folded in the bubble.
func isLongText(text string) bool {
	if text == "" {
		return false
	}
	return len([]rune(text)) > longTextMaxChars ||
		strings.Count(text, "\n") >= longTextMaxLines
}

// clipText is the folded preview of text: the first lines or characters,
// trimmed back to a word boundary and closed with an ellipsis.
func clipText(text string) string {
	if lines := strings.Split(text, "\n"); len(lines) > longTextMaxLines {
		return strings.TrimRight(strings.Join(lines[:longTextMaxLines], "\n"), " \t\n") + "…"
	}
	runes := []rune(text)
	if len(runes) <= longTextCut {
		return text
	}
	cut := string(runes[:longTextCut])
	// Only honour a space that is far enough in; a cut at the very start of
	// the window would throw away most of the preview.
	if sp := strings.LastIndexAny(cut, " \n\t"); sp > longTextCut*6/10 {
		cut = cut[:sp]
	}
	return strings.TrimRight(cut, " \t\n,.;:!?-—") + "…"
}

// readMoreLabel is the fold control's wording, which flips once the body is
// unfolded so the same button closes it again.
func readMoreLabel(open bool) string {
	if open {
		return "Show less"
	}
	return "Read more"
}

// textExpanded reports whether msgID's long body is unfolded.
func (cv *ConversationView) textExpanded(msgID string) bool {
	return cv.expandedText[msgID]
}

// setTextExpanded remembers that msgID's body is unfolded, so a rebuilt row
// (an edit, a reaction, a refill) shows it the same way.
func (cv *ConversationView) setTextExpanded(msgID string, open bool) {
	if cv.expandedText == nil {
		cv.expandedText = map[string]bool{}
	}
	if open {
		cv.expandedText[msgID] = true
		return
	}
	delete(cv.expandedText, msgID)
}
