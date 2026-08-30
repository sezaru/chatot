package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestPollVMNilPoll(t *testing.T) {
	v := pollVM(client.Message{})
	if v.IsPoll {
		t.Fatalf("nil poll should yield IsPoll=false, got %+v", v)
	}
}

func TestPollVMUnnamedFallsBackToPoll(t *testing.T) {
	v := pollVM(client.Message{Poll: &client.Poll{
		Options: []client.PollOption{{Name: "a"}, {Name: "b"}}, SelectableCount: 1,
	}})
	if v.Question != "Poll" {
		t.Fatalf("Question = %q, want Poll", v.Question)
	}
}

func TestPollVMCountsAndPercent(t *testing.T) {
	v := pollVM(client.Message{Poll: &client.Poll{
		Name:            "Lunch?",
		SelectableCount: 1,
		Options: []client.PollOption{
			{Name: "Pizza", Count: 3, Voted: true},
			{Name: "Sushi", Count: 1},
		},
	}})
	if !v.IsPoll {
		t.Fatal("expected IsPoll")
	}
	if v.Total != 4 {
		t.Fatalf("Total = %d, want 4", v.Total)
	}
	if !v.HasVoted {
		t.Fatal("expected HasVoted true (Pizza voted)")
	}
	if v.MultiVote {
		t.Fatal("SelectableCount 1 should not be multi-vote")
	}
	if v.SelectHint != "Select one" {
		t.Fatalf("SelectHint = %q, want Select one", v.SelectHint)
	}
	if v.Options[0].Percent != 75 {
		t.Fatalf("Pizza percent = %d, want 75", v.Options[0].Percent)
	}
	if v.Options[1].Percent != 25 {
		t.Fatalf("Sushi percent = %d, want 25", v.Options[1].Percent)
	}
	if !v.Options[0].Voted {
		t.Fatal("expected Pizza option Voted")
	}
}

func TestPollVMZeroVotesNoPercent(t *testing.T) {
	v := pollVM(client.Message{Poll: &client.Poll{
		Name: "Q", Options: []client.PollOption{{Name: "a"}, {Name: "b"}}, SelectableCount: 2,
	}})
	if v.Total != 0 {
		t.Fatalf("Total = %d, want 0", v.Total)
	}
	if v.Options[0].Percent != 0 {
		t.Fatalf("percent with no votes should be 0, got %d", v.Options[0].Percent)
	}
	if !v.MultiVote {
		t.Fatal("SelectableCount 2 should be multi-vote")
	}
	if v.SelectHint != "Select up to 2" {
		t.Fatalf("SelectHint = %q, want Select up to 2", v.SelectHint)
	}
}
