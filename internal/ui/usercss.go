package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// userSheetReset turns a stylesheet into its own antidote: the same
// selectors, @media blocks and nesting, with every declaration's value
// replaced by "unset" and the ;-terminated at-rules (@define-color,
// @import) dropped. Added one priority above the user sheet, it makes each
// property the theme touched fall back to libadwaita's or GTK's value on
// exactly the nodes the theme matched, without the app having to know what
// the theme set.
func userSheetReset(css string) string {
	css = stripCSSComments(css)
	var out strings.Builder
	var prelude strings.Builder
	i := 0
	for i < len(css) {
		c := css[i]
		switch c {
		case '"', '\'':
			end := cssStringEnd(css, i)
			prelude.WriteString(css[i:end])
			i = end
		case ';':
			// A ;-terminated at-rule (@define-color, @import): dropped.
			prelude.Reset()
			i++
		case '}':
			out.WriteString("}\n")
			prelude.Reset()
			i++
		case '{':
			p := strings.TrimSpace(prelude.String())
			prelude.Reset()
			out.WriteString(p)
			out.WriteString(" {")
			i++
			if cssGroupingRule(p) {
				out.WriteString("\n")
				continue
			}
			end := cssBlockEnd(css, i)
			for _, decl := range splitCSSDecls(css[i:end]) {
				name := strings.TrimSpace(decl[:strings.IndexByte(decl, ':')])
				if name != "" {
					out.WriteString(" " + name + ": unset;")
				}
			}
			out.WriteString(" }\n")
			i = end + 1
		default:
			prelude.WriteByte(c)
			i++
		}
	}
	return out.String()
}

// cssGroupingRule reports whether a block prelude opens a group of rules
// (@media, @keyframes, @supports) rather than a declaration block.
func cssGroupingRule(prelude string) bool {
	for _, p := range []string{"@media", "@keyframes", "@supports", "@container"} {
		if strings.HasPrefix(prelude, p) {
			return true
		}
	}
	return false
}

// cssStringEnd returns the index just past the string literal starting at
// css[i] (a quote), honouring backslash escapes.
func cssStringEnd(css string, i int) int {
	q := css[i]
	for j := i + 1; j < len(css); j++ {
		switch css[j] {
		case '\\':
			j++
		case q:
			return j + 1
		}
	}
	return len(css)
}

// cssBlockEnd returns the index of the '}' closing the declaration block
// whose body starts at css[i], skipping strings and nested parentheses.
func cssBlockEnd(css string, i int) int {
	depth := 0
	for j := i; j < len(css); j++ {
		switch css[j] {
		case '"', '\'':
			j = cssStringEnd(css, j) - 1
		case '(':
			depth++
		case ')':
			depth--
		case '}':
			if depth <= 0 {
				return j
			}
		}
	}
	return len(css)
}

// splitCSSDecls splits a declaration block body on the ';' separators that
// sit outside strings and parentheses, keeping only "name: value" items.
func splitCSSDecls(body string) []string {
	var decls []string
	start, depth := 0, 0
	flush := func(end int) {
		if d := body[start:end]; strings.IndexByte(d, ':') > 0 {
			decls = append(decls, d)
		}
		start = end + 1
	}
	for j := 0; j < len(body); j++ {
		switch body[j] {
		case '"', '\'':
			j = cssStringEnd(body, j) - 1
		case '(':
			depth++
		case ')':
			depth--
		case ';':
			if depth <= 0 {
				flush(j)
			}
		}
	}
	if start < len(body) {
		flush(len(body))
	}
	return decls
}

// stripCSSComments removes /* … */ comments outside string literals.
func stripCSSComments(css string) string {
	var out strings.Builder
	for i := 0; i < len(css); {
		switch {
		case css[i] == '"' || css[i] == '\'':
			end := cssStringEnd(css, i)
			out.WriteString(css[i:end])
			i = end
		case strings.HasPrefix(css[i:], "/*"):
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				return out.String()
			}
			i += end + 4
		default:
			out.WriteByte(css[i])
			i++
		}
	}
	return out.String()
}

var cssImportRE = regexp.MustCompile(`@import\s+(?:url\()?\s*["']?([^"')\s;]+)["']?\s*\)?\s*;`)

// loadUserSheet reads the stylesheet at path with its @import chain
// inlined (relative paths and file:// URIs, up to depth levels), the way
// GTK would see it.
func loadUserSheet(path string, depth int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	css := stripCSSComments(string(data))
	if depth <= 0 {
		return cssImportRE.ReplaceAllString(css, "")
	}
	return cssImportRE.ReplaceAllStringFunc(css, func(m string) string {
		target := cssImportRE.FindStringSubmatch(m)[1]
		target = strings.TrimPrefix(target, "file://")
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		return loadUserSheet(target, depth-1)
	})
}
