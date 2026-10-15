package client

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow/proto/waAICommonDeprecated"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/richresponse"
)

// A Meta AI answer arrives as a rich response: sections of Markdown, code,
// tables and LaTeX, as JSON in the unified form or as typed submessages in
// the older one. Bubbles show plain text, so the answer is flattened into
// text that reads well without Markdown rendering: no ** or # markers,
// bullets as •, links as their text and address, code and tables as plain
// lines. Images, reels and other cards are left out (their links expire).

// richResponseText is the plain text of a Meta AI answer, "" when it has
// none chatot can show.
func richResponseText(rr *waE2E.AIRichResponseMessage) string {
	if data := rr.GetUnifiedResponse().GetData(); len(data) > 0 {
		var resp richresponse.RichResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			log.Printf("chatot: Meta AI response: %v", err)
		} else {
			var parts []string
			for _, sec := range append(resp.Sections, resp.FooterSections...) {
				if sec.Model == nil {
					continue
				}
				for _, p := range sec.Model.GetPrimitives() {
					parts = append(parts, richPrimitiveText(p))
				}
			}
			if text := joinNonEmpty("\n\n", parts...); text != "" {
				return text
			}
		}
	}
	var parts []string
	for _, sub := range rr.GetSubmessages() {
		switch sub.GetMessageType() {
		case waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TEXT:
			parts = append(parts, plainMarkdown(sub.GetMessageText()))
		case waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_CODE:
			var code strings.Builder
			for _, b := range sub.GetCodeMetadata().GetCodeBlocks() {
				code.WriteString(b.GetCodeContent())
			}
			parts = append(parts, strings.TrimRight(code.String(), "\n"))
		case waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TABLE:
			t := sub.GetTableMetadata()
			var rows [][]string
			for _, r := range t.GetRows() {
				rows = append(rows, r.GetItems())
			}
			parts = append(parts, joinNonEmpty("\n", t.GetTitle(), tableText(rows)))
		}
	}
	return joinNonEmpty("\n\n", parts...)
}

// richPrimitiveText is one piece of a unified answer as plain text.
func richPrimitiveText(p richresponse.Primitive) string {
	switch p := p.(type) {
	case *richresponse.GenAIMarkdownTextUXPrimitive:
		return plainMarkdown(resolveEntities(p.Text, p.InlineEntities))
	case *richresponse.FOATextPrimitive:
		return plainMarkdown(resolveEntities(p.Text, nil))
	case *richresponse.GenAIMetadataTextPrimitive:
		return plainMarkdown(p.Text)
	case *richresponse.GenAICodeUXPrimitive:
		return strings.TrimRight(p.String(), "\n")
	case *richresponse.GenAILatexUXPrimitive:
		return p.GetLatexExpression()
	case *richresponse.GenATableUXPrimitive:
		rows := make([][]string, 0, len(p.Rows))
		for _, r := range p.Rows {
			if r == nil {
				continue
			}
			cells := r.Cells
			if len(cells) == 0 {
				for _, mc := range r.MarkdownCells {
					cells = append(cells, plainMarkdown(resolveEntities(mc.Text, mc.InlineEntities)))
				}
			}
			rows = append(rows, cells)
		}
		return tableText(rows)
	}
	// Thinking status, images, reels, posts, search cards, dividers and
	// spacers: nothing to read, or only a short-lived link.
	return ""
}

// tableText lays a table out one row per line, cells split by a bar.
func tableText(rows [][]string) string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		cells := make([]string, len(r))
		for i, c := range r {
			cells[i] = strings.TrimSpace(c)
		}
		if line := strings.Join(cells, " | "); strings.Trim(line, " |") != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

var (
	pairedEntity = regexp.MustCompile(`\{\{([^\s}/]+)}}([\s\S]*?)\{\{/[^\s}]+}}`)
	loneEntity   = regexp.MustCompile(`\{\{[^\s}]+}}`)
)

// resolveEntities replaces the {{key}} placeholders in an answer's text
// with what they stand for: a link's text and address, a deep link's text,
// a formula. A placeholder wrapped around text ({{key}}text{{/key}}) keeps
// its text; citations and anything unknown leave no trace.
func resolveEntities(text string, ents []richresponse.TextEntity) string {
	byKey := make(map[string]richresponse.TextEntityMetadata, len(ents))
	for _, e := range ents {
		byKey[e.Key] = e.Metadata
	}
	text = pairedEntity.ReplaceAllStringFunc(text, func(m string) string {
		sub := pairedEntity.FindStringSubmatch(m)
		if link, ok := byKey[sub[1]].(*richresponse.GenAIInlineLinkItem); ok && link.URL != "" && link.URL != sub[2] {
			return sub[2] + " (" + link.URL + ")"
		}
		return sub[2]
	})
	return loneEntity.ReplaceAllStringFunc(text, func(m string) string {
		switch e := byKey[m[2:len(m)-2]].(type) {
		case *richresponse.GenAIInlineLinkItem:
			if e.DisplayName == "" || e.DisplayName == e.URL {
				return e.URL
			}
			return e.DisplayName + " (" + e.URL + ")"
		case *richresponse.GenAIDeepLinkItem:
			return e.Text
		case *richresponse.GenAILatexItem:
			return e.LatexExpression
		}
		return ""
	})
}

var (
	mdHeading = regexp.MustCompile(`^\s{0,3}#{1,6}\s+`)
	mdBullet  = regexp.MustCompile(`^(\s*)[-*+]\s+`)
	mdRule    = regexp.MustCompile(`^\s{0,3}([-*_])(\s*[-*_]){2,}\s*$`)
	mdStrong  = regexp.MustCompile(`\*\*(\S(?:[^*]*\S)?)\*\*`)
	mdCode    = regexp.MustCompile("`([^`\n]+)`")
	mdLink    = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
)

// plainMarkdown strips the Markdown an answer is written in down to text:
// headings and bold lose their markers, bullets become •, inline code
// loses its backticks, a link becomes its text and address. Fenced code is
// kept as it is, minus the fences.
func plainMarkdown(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	out := lines[:0]
	inFence := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}
		if mdRule.MatchString(line) {
			out = append(out, "")
			continue
		}
		line = mdHeading.ReplaceAllString(line, "")
		line = mdBullet.ReplaceAllString(line, "$1• ")
		line = mdStrong.ReplaceAllString(line, "$1")
		line = mdCode.ReplaceAllString(line, "$1")
		line = mdLink.ReplaceAllStringFunc(line, func(m string) string {
			sub := mdLink.FindStringSubmatch(m)
			if sub[1] == sub[2] {
				return sub[2]
			}
			return sub[1] + " (" + sub[2] + ")"
		})
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
