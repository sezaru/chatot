package ui

import (
	"html"
	"regexp"
	"strings"
)

// mentionRE matches WhatsApp's wire form of an @mention: "@" followed by
// the mentioned account's numeric user part (a phone number or a LID).
var mentionRE = regexp.MustCompile(`@(\d{5,20})`)

// mentionAccent is the accent the mockup gives links inside an incoming
// bubble (linkFg); an outgoing bubble uses white via mentionMarkupOut. The
// dark theme lifts it to the sheet's mint (chatot_accent_text), since the
// brand green is unreadable on a dark bubble.
const (
	mentionAccent     = "#147a63"
	mentionAccentDark = "#46c39a"
)

// mentionAccentFor picks the mention colour for the current theme.
func mentionAccentFor() string {
	if isDark() {
		return mentionAccentDark
	}
	return mentionAccent
}

// mentionMarkupColor renders text as Pango markup with every resolvable
// @mention replaced by the person's name in bold accent (white on an
// outgoing bubble). resolve maps the numeric user part to a display name
// ("" leaves the mention as typed, still styled so it reads as a mention).
// Everything else is escaped verbatim. Message bubbles go through
// messageMarkup, which renders mentions the same way beside links.
func mentionMarkupColor(text string, resolve func(user string) string, onGreen bool, accent string) string {
	color := accent
	if onGreen {
		color = "#ffffff"
	}
	var b strings.Builder
	last := 0
	for _, m := range mentionRE.FindAllStringSubmatchIndex(text, -1) {
		b.WriteString(html.EscapeString(text[last:m[0]]))
		user := text[m[2]:m[3]]
		name := ""
		if resolve != nil {
			name = resolve(user)
		}
		if name == "" {
			name = user
		}
		b.WriteString(`<span foreground="` + color + `" weight="bold">@` + html.EscapeString(name) + `</span>`)
		last = m[1]
	}
	b.WriteString(html.EscapeString(text[last:]))
	return b.String()
}

// resolveMentionsPlain rewrites @mentions to "@Name" in plain text, for
// previews, quotes and notifications where no markup is possible.
func resolveMentionsPlain(text string, resolve func(user string) string) string {
	if resolve == nil || !strings.Contains(text, "@") {
		return text
	}
	return mentionRE.ReplaceAllStringFunc(text, func(m string) string {
		if name := resolve(m[1:]); name != "" {
			return "@" + name
		}
		return m
	})
}
