package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestNotifierResolvesMentions(t *testing.T) {
	n := &Notifier{c: client.NewFake()}
	got := resolveMentionsPlain("@1112223333 look, @5550001111", n.mentionName)
	if want := "@Grace Hopper look, @5550001111"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
