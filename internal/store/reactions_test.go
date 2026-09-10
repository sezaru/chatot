package store

import "testing"

// ReactionBy is what a reaction shown before it is sent uses to remember what
// to put back if the send fails, so it has to report "no reaction" and a
// cleared reaction the same way: as the empty string, without an error.
func TestReactionBy(t *testing.T) {
	s := newTestStore(t)
	const (
		chat = "1234567890@s.whatsapp.net"
		msg  = "MSG1"
		me   = "5551112222@s.whatsapp.net"
	)

	got, err := s.ReactionBy(chat, msg, me)
	if err != nil || got != "" {
		t.Fatalf("ReactionBy on a message with no reaction = (%q, %v), want (\"\", nil)", got, err)
	}

	if err := s.UpsertReaction(ReactionRow{ChatJID: chat, MsgID: msg, ReactorJID: me, Emoji: "👍", TS: 1}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.ReactionBy(chat, msg, me); err != nil || got != "👍" {
		t.Fatalf("ReactionBy after react = (%q, %v), want (👍, nil)", got, err)
	}

	// Someone else's reaction on the same message is not ours.
	if err := s.UpsertReaction(ReactionRow{ChatJID: chat, MsgID: msg, ReactorJID: "999@s.whatsapp.net", Emoji: "😂", TS: 2}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.ReactionBy(chat, msg, me); err != nil || got != "👍" {
		t.Fatalf("ReactionBy with another reactor present = (%q, %v), want (👍, nil)", got, err)
	}

	if err := s.UpsertReaction(ReactionRow{ChatJID: chat, MsgID: msg, ReactorJID: me, Emoji: "", TS: 3}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.ReactionBy(chat, msg, me); err != nil || got != "" {
		t.Fatalf("ReactionBy after clearing = (%q, %v), want (\"\", nil)", got, err)
	}
}
