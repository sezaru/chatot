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
