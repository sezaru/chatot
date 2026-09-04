package ui

import (
	"html"
	"regexp"
	"sort"
	"strings"
)

// urlRE matches a web address as people type them: a scheme, or "www."
// with none. A bare domain is not a link; "e.g." and "file.txt" would be.
var urlRE = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>"']+`)

// urlTrailing are the characters a sentence hangs on a URL that are not
// part of it: "see https://x.y/z." links https://x.y/z.
const urlTrailing = ".,;:!?)]}'\""

// textSpan is one run of a bubble's text with its own rendering: a link,
// a mention, or plain text between them.
type textSpan struct {
	start, end int
	kind       spanKind
}

type spanKind int

const (
	spanPlain spanKind = iota
	spanLink
	spanMention
)

// messageSpans splits text into links, mentions and the plain runs between
// them, in order. A mention inside a URL is part of the URL.
func messageSpans(text string) []textSpan {
	var spans []textSpan
	for _, m := range urlRE.FindAllStringIndex(text, -1) {
		end := m[1]
		for end > m[0] && strings.IndexByte(urlTrailing, text[end-1]) >= 0 {
			end--
		}
		spans = append(spans, textSpan{m[0], end, spanLink})
	}
	for _, m := range mentionRE.FindAllStringIndex(text, -1) {
		inLink := false
		for _, s := range spans {
			if m[0] < s.end && m[1] > s.start {
				inLink = true
				break
			}
		}
		if !inLink {
			spans = append(spans, textSpan{m[0], m[1], spanMention})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var out []textSpan
	last := 0
	for _, s := range spans {
		if s.start > last {
			out = append(out, textSpan{last, s.start, spanPlain})
		}
		out = append(out, s)
		last = s.end
	}
	if last < len(text) {
		out = append(out, textSpan{last, len(text), spanPlain})
	}
	return out
}

// linkHref is the address a matched URL opens: as written with a scheme,
// else https:// in front of a "www." address.
func linkHref(url string) string {
	if strings.HasPrefix(strings.ToLower(url), "www.") {
		return "https://" + url
	}
	return url
}

// messageMarkup renders a bubble's text as Pango markup: web addresses as
// links the label opens on click, @mentions as bold accent names (white on
// an outgoing bubble), and every occurrence of query highlighted as the
// in-chat search does. Everything else is escaped verbatim, so the result
// is safe for gtk.Label.SetMarkup.
func messageMarkup(text string, resolve func(user string) string, onGreen bool, accent, query string) string {
	color := accent
	if onGreen {
		color = "#ffffff"
	}
	var b strings.Builder
	for _, s := range messageSpans(text) {
		run := text[s.start:s.end]
		switch s.kind {
		case spanLink:
			b.WriteString(`<a href="` + html.EscapeString(linkHref(run)) + `">`)
			b.WriteString(highlightMarkup(run, query))
			b.WriteString(`</a>`)
		case spanMention:
			user := run[1:]
			name := ""
			if resolve != nil {
				name = resolve(user)
			}
			if name == "" {
				name = user
			}
			b.WriteString(`<span foreground="` + color + `" weight="bold">@` + html.EscapeString(name) + `</span>`)
		default:
			b.WriteString(highlightMarkup(run, query))
		}
	}
	return b.String()
}
