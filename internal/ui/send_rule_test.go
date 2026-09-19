package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestSendBlockLine(t *testing.T) {
	const own = "me:17@s.whatsapp.net" // OwnJID carries this device's suffix
	member := client.GroupInfo{
		JID:      "g@g.us",
		OwnerJID: "boss@s.whatsapp.net",
		Participants: []client.GroupParticipant{
			{JID: "boss@s.whatsapp.net", IsSuperAdmin: true},
			{JID: "me@s.whatsapp.net"},
		},
	}
	admin := member
	admin.Participants = []client.GroupParticipant{
		{JID: "boss@s.whatsapp.net", IsSuperAdmin: true},
		{JID: "me@s.whatsapp.net", IsAdmin: true},
	}
	owner := member
	owner.OwnerJID = "me@s.whatsapp.net"

	announce := func(g client.GroupInfo) client.GroupInfo { g.Announce = true; return g }

	cases := []struct {
		name  string
		info  *client.GroupInfo
		block bool
	}{
		{"an open group takes our messages", &member, false},
		{"an announcement group does not", ptr(announce(member)), true},
		{"unless we are an admin of it", ptr(announce(admin)), false},
		{"or its owner", ptr(announce(owner)), false},
		{"a group nobody could read stays open", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line := sendBlockLine(c.info, own)
			if (line != "") != c.block {
				t.Errorf("sendBlockLine = %q, blocked = %v, want blocked = %v", line, line != "", c.block)
			}
		})
	}
}

// Without a JID, there is nobody to be an admin: the rule must not read a
// logged-out account as an owner match on two empty strings.
func TestSendBlockLine_NoOwnJIDBlocksAnAnnouncementGroup(t *testing.T) {
	info := client.GroupInfo{JID: "g@g.us", Announce: true}
	if sendBlockLine(&info, "") == "" {
		t.Error("an announcement group must not open up just because OwnJID is unknown")
	}
}

func ptr(g client.GroupInfo) *client.GroupInfo { return &g }
