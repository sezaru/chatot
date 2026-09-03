package ui

import "testing"

func TestMentionMarkupResolvesAndEscapes(t *testing.T) {
	resolve := func(user string) string {
		if user == "257157073207386" {
			return "Marco <Souza>"
		}
		return ""
	}
	got := mentionMarkup("@257157073207386 vai sair um remake & @123456 viu?", resolve, false)
	want := `<span foreground="#147a63" weight="bold">@Marco &lt;Souza&gt;</span> vai sair um remake &amp; <span foreground="#147a63" weight="bold">@123456</span> viu?`
	if got != want {
		t.Errorf("mentionMarkup =\n%s\nwant\n%s", got, want)
	}
	if got := mentionMarkup("plain a@b.c", nil, true); got != "plain a@b.c" {
		t.Errorf("plain text = %q", got)
	}
}

func TestResolveMentionsPlain(t *testing.T) {
	resolve := func(user string) string {
		if user == "5548999" {
			return "Nina"
		}
		return ""
	}
	if got := resolveMentionsPlain("hey @5548999 and @1", resolve); got != "hey @Nina and @1" {
		t.Errorf("got %q", got)
	}
	if !hasMention("@5548999 hi") || hasMention("mail me a@b") {
		t.Error("hasMention mismatch")
	}
}
