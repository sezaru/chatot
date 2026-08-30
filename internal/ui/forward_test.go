package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestForwardSelectionLabel(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0 chats selected"},
		{1, "1 chat selected"},
		{2, "2 chats selected"},
		{5, "5 chats selected"},
	}
	for _, tc := range cases {
		if got := forwardSelectionLabel(tc.n); got != tc.want {
			t.Errorf("forwardSelectionLabel(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestFilterForwardChats(t *testing.T) {
	chats := []client.Chat{
		{JID: "1", Name: "Ada Lovelace"},
		{JID: "2", Name: "Grace Hopper"},
		{JID: "3", Name: "Alan Turing"},
	}

	t.Run("empty query matches everything in order", func(t *testing.T) {
		got := filterForwardChats(chats, "")
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		if got[0].JID != "1" || got[2].JID != "3" {
			t.Errorf("order not preserved: %+v", got)
		}
	})

	t.Run("case-insensitive substring match", func(t *testing.T) {
		got := filterForwardChats(chats, "ada")
		if len(got) != 1 || got[0].JID != "1" {
			t.Errorf("got %+v, want only Ada", got)
		}
	})

	t.Run("matches multiple", func(t *testing.T) {
		got := filterForwardChats(chats, "a")
		if len(got) != 3 {
			t.Errorf("got %d matches, want 3 (all names contain 'a')", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := filterForwardChats(chats, "zzz")
		if len(got) != 0 {
			t.Errorf("got %+v, want none", got)
		}
	})
}
