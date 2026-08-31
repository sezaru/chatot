package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestParticipantBadge(t *testing.T) {
	owner := "owner@s.whatsapp.net"
	cases := []struct {
		name string
		p    client.GroupParticipant
		want string
	}{
		{"owner", client.GroupParticipant{JID: owner}, "Owner"},
		{"super admin", client.GroupParticipant{JID: "a@s.whatsapp.net", IsSuperAdmin: true}, "Admin"},
		{"admin", client.GroupParticipant{JID: "a@s.whatsapp.net", IsAdmin: true}, "Admin"},
		{"member", client.GroupParticipant{JID: "a@s.whatsapp.net"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := participantBadge(tc.p, owner); got != tc.want {
				t.Errorf("participantBadge = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParticipantBadge_NoOwnerSet(t *testing.T) {
	if got := participantBadge(client.GroupParticipant{JID: "a@s.whatsapp.net"}, ""); got != "" {
		t.Errorf("participantBadge = %q, want empty", got)
	}
}

func TestOrderParticipants(t *testing.T) {
	owner := "owner@s.whatsapp.net"
	parts := []client.GroupParticipant{
		{JID: "zzz@s.whatsapp.net"},
		{JID: "admin@s.whatsapp.net", IsAdmin: true},
		{JID: owner},
		{JID: "aaa@s.whatsapp.net"},
	}
	got := orderParticipants(parts, owner)
	want := []string{owner, "admin@s.whatsapp.net", "aaa@s.whatsapp.net", "zzz@s.whatsapp.net"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, jid := range want {
		if got[i].JID != jid {
			t.Errorf("got[%d] = %q, want %q", i, got[i].JID, jid)
		}
	}
}

func TestOrderParticipants_DoesNotMutateInput(t *testing.T) {
	parts := []client.GroupParticipant{{JID: "b@s.whatsapp.net"}, {JID: "a@s.whatsapp.net"}}
	orderParticipants(parts, "")
	if parts[0].JID != "b@s.whatsapp.net" {
		t.Errorf("input slice was mutated: %v", parts)
	}
}

func TestIsSelfAdmin(t *testing.T) {
	info := client.GroupInfo{
		OwnerJID: "owner@s.whatsapp.net",
		Participants: []client.GroupParticipant{
			{JID: "owner@s.whatsapp.net", IsAdmin: true, IsSuperAdmin: true},
			{JID: "admin@s.whatsapp.net", IsAdmin: true},
			{JID: "member@s.whatsapp.net"},
		},
	}
	cases := []struct {
		own  string
		want bool
	}{
		{"owner@s.whatsapp.net", true},
		{"admin@s.whatsapp.net", true},
		{"member@s.whatsapp.net", false},
		{"stranger@s.whatsapp.net", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isSelfAdmin(info, tc.own); got != tc.want {
			t.Errorf("isSelfAdmin(%q) = %v, want %v", tc.own, got, tc.want)
		}
	}
}

func TestPromoteDemoteLabel(t *testing.T) {
	if l, a := promoteDemoteLabel(client.GroupParticipant{}); l != "Promote" || a != "promote" {
		t.Errorf("member = (%q,%q), want (Promote,promote)", l, a)
	}
	if l, a := promoteDemoteLabel(client.GroupParticipant{IsAdmin: true}); l != "Demote" || a != "demote" {
		t.Errorf("admin = (%q,%q), want (Demote,demote)", l, a)
	}
}

func TestParseParticipantList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"+1 555 123 4567", []string{"15551234567@s.whatsapp.net"}},
		{"15551234567, 16662223333", []string{"15551234567@s.whatsapp.net", "16662223333@s.whatsapp.net"}},
		{"someone@s.whatsapp.net", []string{"someone@s.whatsapp.net"}},
		{" , ,", nil},
		{"", nil},
	}
	for _, tc := range cases {
		got := parseParticipantList(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("parseParticipantList(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseParticipantList(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestDisappearingSecondsForIndex(t *testing.T) {
	cases := []struct {
		idx  int
		want int64
	}{
		{0, 0},
		{1, 24 * 60 * 60},
		{2, 7 * 24 * 60 * 60},
		{3, 90 * 24 * 60 * 60},
		{-1, 0},
		{4, 0},
	}
	for _, tc := range cases {
		if got := disappearingSecondsForIndex(tc.idx); got != tc.want {
			t.Errorf("disappearingSecondsForIndex(%d) = %d, want %d", tc.idx, got, tc.want)
		}
	}
}

func TestParticipantSelection(t *testing.T) {
	sel := newParticipantSelection()
	if sel.Count() != 0 {
		t.Fatalf("new selection Count = %d, want 0", sel.Count())
	}

	sel.Add("a@s.whatsapp.net", "Alex")
	sel.Add("b@s.whatsapp.net", "Mom")
	sel.Add("a@s.whatsapp.net", "Alex Rivera") // duplicate add is a no-op
	if got := sel.Count(); got != 2 {
		t.Errorf("Count after adds = %d, want 2", got)
	}
	if !sel.Contains("a@s.whatsapp.net") {
		t.Error("Contains(a) = false, want true")
	}
	if sel.Contains("z@s.whatsapp.net") {
		t.Error("Contains(z) = true, want false")
	}

	wantJIDs := []string{"a@s.whatsapp.net", "b@s.whatsapp.net"}
	if got := sel.JIDs(); len(got) != len(wantJIDs) || got[0] != wantJIDs[0] || got[1] != wantJIDs[1] {
		t.Errorf("JIDs() = %v, want %v", got, wantJIDs)
	}

	chips := sel.Chips()
	if len(chips) != 2 || chips[0].Name != "Alex" || chips[1].Name != "Mom" {
		t.Errorf("Chips() = %v, want Alex then Mom (first add wins name)", chips)
	}

	sel.Add("c@s.whatsapp.net", "Priya")
	sel.Remove("b@s.whatsapp.net")
	if sel.Contains("b@s.whatsapp.net") {
		t.Error("Contains(b) after Remove = true, want false")
	}
	if got := sel.Count(); got != 2 {
		t.Errorf("Count after remove = %d, want 2", got)
	}
	if got := sel.JIDs(); len(got) != 2 || got[0] != "a@s.whatsapp.net" || got[1] != "c@s.whatsapp.net" {
		t.Errorf("JIDs() after remove = %v, want [a c]", got)
	}

	sel.Remove("nonexistent@s.whatsapp.net") // no-op, must not panic or corrupt state
	if got := sel.Count(); got != 2 {
		t.Errorf("Count after removing missing jid = %d, want 2", got)
	}
}
