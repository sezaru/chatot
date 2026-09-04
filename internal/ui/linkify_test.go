package ui

import "testing"

func TestMessageMarkupLinksMentionsAndHighlight(t *testing.T) {
	resolve := func(user string) string {
		if user == "5548999" {
			return "Nina"
		}
		return ""
	}
	tests := []struct {
		name, text, query, want string
	}{
		{"plain", "just <text> & more", "", "just &lt;text&gt; &amp; more"},
		{"scheme url keeps its query", "see https://a.b/c?x=1&y=2 now", "",
			`see <a href="https://a.b/c?x=1&amp;y=2">https://a.b/c?x=1&amp;y=2</a> now`},
		{"trailing punctuation stays outside", "go to www.site.org/p).", "",
			`go to <a href="https://www.site.org/p">www.site.org/p</a>).`},
		{"mention beside a link", "@5548999 https://x.y", "",
			`<span foreground="#147a63" weight="bold">@Nina</span> <a href="https://x.y">https://x.y</a>`},
		{"mention-like digits inside a url are the url", "https://t.me/@12345678", "",
			`<a href="https://t.me/@12345678">https://t.me/@12345678</a>`},
		{"highlight in plain and link", "read https://docs.x/read", "read",
			`<span background="#f5c518" foreground="#1b1b1b">read</span> <a href="https://docs.x/read">https://docs.x/<span background="#f5c518" foreground="#1b1b1b">read</span></a>`},
		{"bare domain is not a link", "file.txt e.g. this", "", "file.txt e.g. this"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messageMarkup(tt.text, resolve, false, mentionAccent, tt.query); got != tt.want {
				t.Errorf("messageMarkup(%q) =\n%s\nwant\n%s", tt.text, got, tt.want)
			}
		})
	}
	if got := messageMarkup("@5548999", resolve, true, mentionAccent, ""); got != `<span foreground="#ffffff" weight="bold">@Nina</span>` {
		t.Errorf("on green = %s", got)
	}
}
