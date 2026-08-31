package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestJoinRequestBannerText(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, ""},
		{-1, ""},
		{1, "1 person requested to join"},
		{2, "2 people requested to join"},
		{5, "5 people requested to join"},
	}
	for _, tc := range cases {
		if got := joinRequestBannerText(tc.n); got != tc.want {
			t.Errorf("joinRequestBannerText(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestSortJoinRequests(t *testing.T) {
	older := time.Unix(100, 0)
	newer := time.Unix(200, 0)
	reqs := []client.JoinRequest{
		{JID: "b@s.whatsapp.net", RequestedAt: newer},
		{JID: "a@s.whatsapp.net", RequestedAt: older},
		{JID: "z@s.whatsapp.net", RequestedAt: older},
	}
	got := sortJoinRequests(reqs)
	want := []string{"a@s.whatsapp.net", "z@s.whatsapp.net", "b@s.whatsapp.net"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, jid := range want {
		if got[i].JID != jid {
			t.Errorf("got[%d] = %q, want %q", i, got[i].JID, jid)
		}
	}
}

func TestSortJoinRequests_DoesNotMutateInput(t *testing.T) {
	reqs := []client.JoinRequest{
		{JID: "b@s.whatsapp.net", RequestedAt: time.Unix(2, 0)},
		{JID: "a@s.whatsapp.net", RequestedAt: time.Unix(1, 0)},
	}
	sortJoinRequests(reqs)
	if reqs[0].JID != "b@s.whatsapp.net" {
		t.Errorf("input slice was mutated: %v", reqs)
	}
}
