package ui

import (
	"context"
	"reflect"
	"testing"

	"chatot/internal/client"
)

func TestMentionQuery(t *testing.T) {
	cases := []struct {
		text   string
		cursor int
		start  int
		frag   string
		ok     bool
	}{
		{"@Mar", 4, 0, "Mar", true},
		{"hi @Ma", 6, 3, "Ma", true},
		{"hi @", 4, 3, "", true},
		{"hi @Mar x", 9, 0, "", false},
		{"mail me@x.org", 13, 0, "", false},
		{"plain", 5, 0, "", false},
		{"@Már", 4, 0, "Már", true},
		{"@Mar", 9, 0, "", false},
	}
	for _, tc := range cases {
		start, frag, ok := mentionQuery(tc.text, tc.cursor)
		if start != tc.start || frag != tc.frag || ok != tc.ok {
			t.Errorf("mentionQuery(%q, %d) = (%d, %q, %v), want (%d, %q, %v)", tc.text, tc.cursor, start, frag, ok, tc.start, tc.frag, tc.ok)
		}
	}
}

func TestFilterMentionsRanksPrefixFirst(t *testing.T) {
	people := []mentionCandidate{
		{Name: "Ken Thompson", User: "1998887777"},
		{Name: "Marco Santos Souza", User: "257157073207386"},
		{Name: "Marcelo Saviski", User: "554899010873"},
		{Name: "You", User: "1234567890"},
	}
	got := filterMentions(people, "mar", 6)
	want := []string{"Marco Santos Souza", "Marcelo Saviski"}
	names := func(cs []mentionCandidate) []string {
		out := []string{}
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return out
	}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("filterMentions(mar) = %v, want %v", names(got), want)
	}
	if got := names(filterMentions(people, "thomp", 6)); !reflect.DeepEqual(got, []string{"Ken Thompson"}) {
		t.Errorf("word prefix = %v", got)
	}
	if got := names(filterMentions(people, "1998", 6)); !reflect.DeepEqual(got, []string{"Ken Thompson"}) {
		t.Errorf("number prefix = %v", got)
	}
	if got := filterMentions(people, "", 2); len(got) != 2 {
		t.Errorf("empty fragment lists everyone up to max, got %d", len(got))
	}
	if got := filterMentions(people, "zzz", 6); len(got) != 0 {
		t.Errorf("no match should be empty, got %v", got)
	}
}

func TestApplyAndWireMentions(t *testing.T) {
	text, cursor := applyMention("hi @Mar, ok?", 3, 7, "Marco Santos Souza")
	if text != "hi @Marco Santos Souza , ok?" || cursor != 23 {
		t.Errorf("applyMention = %q, %d", text, cursor)
	}
	names := map[string]string{"Ken": "111", "Ken Thompson": "1998887777"}
	got := wireMentions("@Ken Thompson and @Ken go", names)
	if got != "@1998887777 and @111 go" {
		t.Errorf("wireMentions = %q", got)
	}
	if got := wireMentions("no mentions", names); got != "no mentions" {
		t.Errorf("wireMentions untouched = %q", got)
	}
}

func TestMentionPeopleOwnChatListsYouOnce(t *testing.T) {
	f := client.NewFake()
	people := mentionPeople(context.Background(), f, f.OwnJID(), "")
	if len(people) != 1 || people[0].Name != "You" {
		t.Fatalf("own chat = %+v, want just You", people)
	}
}
