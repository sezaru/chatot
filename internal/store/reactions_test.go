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

// An unread reaction is listed until it is read, by our own read or the
// phone's receipt, or until it is taken back; a changed reaction replaces
// the earlier one, whose id no longer needs a receipt.
func TestUnseenReactions(t *testing.T) {
	s := newTestStore(t)
	const (
		chat  = "1234567890@s.whatsapp.net"
		other = "999@g.us"
		alice = "111@s.whatsapp.net"
		bob   = "222@s.whatsapp.net"
	)
	add := func(chat, msg, reactor, id string) {
		t.Helper()
		if err := s.AddUnseenReaction(UnseenReaction{ChatJID: chat, MsgID: msg, ReactorJID: reactor, ReactionID: id}); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(chat string) map[string]string {
		t.Helper()
		rows, err := s.UnseenReactions(chat)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, r := range rows {
			out[r.ReactionID] = r.ReactorJID
		}
		return out
	}

	add(chat, "M1", alice, "R1")
	add(chat, "M1", alice, "R2") // alice changed the reaction
	add(chat, "M2", bob, "R3")
	add(other, "M9", alice, "R9")
	if got := ids(chat); len(got) != 2 || got["R2"] != alice || got["R3"] != bob {
		t.Fatalf("UnseenReactions = %v, want R2 by alice and R3 by bob", got)
	}

	if err := s.DropUnseenReactions(chat, []string{"R2", "M1"}); err != nil {
		t.Fatal(err)
	}
	if got := ids(chat); len(got) != 1 || got["R3"] != bob {
		t.Fatalf("after dropping R2 = %v, want R3 only", got)
	}

	if err := s.ClearUnseenReaction(chat, "M2", bob); err != nil {
		t.Fatal(err)
	}
	if got := ids(chat); len(got) != 0 {
		t.Fatalf("after bob took the reaction back = %v, want none", got)
	}
	if got := ids(other); len(got) != 1 {
		t.Fatalf("another chat's unread reaction went too: %v", got)
	}
}
