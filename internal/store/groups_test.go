package store

import "testing"

func TestUpsertGroupAndGroupMetaRoundTrip(t *testing.T) {
	s := newTestStore(t)

	must(t, s.UpsertGroup(GroupRow{JID: "g@g.us", Name: "Trip", Topic: "Plans for the trip"}))

	name, topic, err := s.GroupMeta("g@g.us")
	must(t, err)
	if name != "Trip" || topic != "Plans for the trip" {
		t.Fatalf("GroupMeta = (%q, %q), want (Trip, Plans for the trip)", name, topic)
	}

	// Empty Name/Topic on a later upsert leaves the existing values alone.
	must(t, s.UpsertGroup(GroupRow{JID: "g@g.us"}))
	name, topic, err = s.GroupMeta("g@g.us")
	must(t, err)
	if name != "Trip" || topic != "Plans for the trip" {
		t.Fatalf("GroupMeta after empty upsert = (%q, %q), want unchanged", name, topic)
	}
}

func TestSetGroupParticipantsReplacesAll(t *testing.T) {
	s := newTestStore(t)

	must(t, s.SetGroupParticipants("g@g.us", []GroupParticipant{
		{JID: "alice@s.whatsapp.net", IsAdmin: true},
		{JID: "bob@s.whatsapp.net"},
	}))

	parts, err := s.GroupParticipants("g@g.us")
	must(t, err)
	if len(parts) != 2 {
		t.Fatalf("GroupParticipants len = %d, want 2 (%v)", len(parts), parts)
	}
	byJID := map[string]GroupParticipant{}
	for _, p := range parts {
		byJID[p.JID] = p
	}
	if !byJID["alice@s.whatsapp.net"].IsAdmin {
		t.Fatalf("alice should be admin: %+v", byJID["alice@s.whatsapp.net"])
	}
	if byJID["bob@s.whatsapp.net"].IsAdmin {
		t.Fatalf("bob should not be admin: %+v", byJID["bob@s.whatsapp.net"])
	}

	// A second call replaces the set rather than appending.
	must(t, s.SetGroupParticipants("g@g.us", []GroupParticipant{
		{JID: "carol@s.whatsapp.net", IsSuperAdmin: true},
	}))
	parts, err = s.GroupParticipants("g@g.us")
	must(t, err)
	if len(parts) != 1 || parts[0].JID != "carol@s.whatsapp.net" || !parts[0].IsSuperAdmin {
		t.Fatalf("after replace parts = %+v", parts)
	}
}
