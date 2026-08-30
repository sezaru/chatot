package store

import (
	"bytes"
	"testing"
)

func hash(b string) []byte { return []byte(b) } // opaque test bytes; store never interprets them

func TestSetPollVotesAndTally(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "poll1", TS: 1, Kind: "poll", Payload: `{"name":"Q","options":["x","y"]}`}))

	must(t, s.SetPollVotes("a@s.whatsapp.net", "poll1", "v1@s.whatsapp.net", [][]byte{hash("x")}))
	must(t, s.SetPollVotes("a@s.whatsapp.net", "poll1", "v2@s.whatsapp.net", [][]byte{hash("x"), hash("y")}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	votes := msgs[0].PollVotes
	if len(votes) != 3 {
		t.Fatalf("got %d vote rows, want 3", len(votes))
	}
	var xCount int
	for _, v := range votes {
		if bytes.Equal(v.OptionHash, hash("x")) {
			xCount++
		}
	}
	if xCount != 2 {
		t.Fatalf("got %d votes for x, want 2", xCount)
	}
}

func TestSetPollVotesReplacesPriorVote(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "poll1", TS: 1, Kind: "poll", Payload: `{}`}))

	must(t, s.SetPollVotes("a@s.whatsapp.net", "poll1", "v1@s.whatsapp.net", [][]byte{hash("x")}))
	must(t, s.SetPollVotes("a@s.whatsapp.net", "poll1", "v1@s.whatsapp.net", [][]byte{hash("y")}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	votes := msgs[0].PollVotes
	if len(votes) != 1 {
		t.Fatalf("got %d vote rows, want 1 (re-vote replaces)", len(votes))
	}
	if !bytes.Equal(votes[0].OptionHash, hash("y")) {
		t.Fatalf("expected the replacement vote for y, got % x", votes[0].OptionHash)
	}
}

func TestSetPollVotesEmptyClears(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "poll1", TS: 1, Kind: "poll", Payload: `{}`}))

	must(t, s.SetPollVotes("a@s.whatsapp.net", "poll1", "v1@s.whatsapp.net", [][]byte{hash("x")}))
	must(t, s.SetPollVotes("a@s.whatsapp.net", "poll1", "v1@s.whatsapp.net", nil))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs[0].PollVotes) != 0 {
		t.Fatalf("expected votes cleared, got %d", len(msgs[0].PollVotes))
	}
}

func TestNonPollMessageHasNoVotesAttached(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hi", TS: 1}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if msgs[0].PollVotes != nil {
		t.Fatalf("plain message should have no poll votes, got %+v", msgs[0].PollVotes)
	}
}
