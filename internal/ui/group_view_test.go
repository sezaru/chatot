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
