package client

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestParseInviteCode(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"full https url", "https://chat.whatsapp.com/ABC123xyz", "ABC123xyz"},
		{"http url", "http://chat.whatsapp.com/ABC123xyz", "ABC123xyz"},
		{"scheme-less", "chat.whatsapp.com/ABC123xyz", "ABC123xyz"},
		{"bare code", "ABC123xyz", "ABC123xyz"},
		{"trailing slash", "chat.whatsapp.com/ABC123xyz/", "ABC123xyz"},
		{"whitespace", "  ABC123xyz  ", "ABC123xyz"},
		{"url with whitespace", "  https://chat.whatsapp.com/ABC123xyz/  ", "ABC123xyz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseInviteCode(tc.in); got != tc.want {
				t.Errorf("parseInviteCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMapParticipantAction(t *testing.T) {
	cases := []struct {
		in     string
		want   whatsmeow.ParticipantChange
		wantOK bool
	}{
		{"add", whatsmeow.ParticipantChangeAdd, true},
		{"remove", whatsmeow.ParticipantChangeRemove, true},
		{"promote", whatsmeow.ParticipantChangePromote, true},
		{"demote", whatsmeow.ParticipantChangeDemote, true},
		{"kick", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := mapParticipantAction(tc.in)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("mapParticipantAction(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestValidGroupName(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Weekend Trip", true},
		{"", false},
		{"   ", false},
		{"exactly twenty-five chars", true}, // 25 chars
		{"this name is twenty six ch", false},
	}
	for _, tc := range cases {
		if got := validGroupName(tc.in); got != tc.want {
			t.Errorf("validGroupName(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFakeCreateGroup(t *testing.T) {
	f := NewFake()
	jid, err := f.CreateGroup(context.Background(), "Trip", []string{"5551@s.whatsapp.net"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	chats, _ := f.Chats(0)
	found := false
	for _, c := range chats {
		if c.JID == jid && c.IsGroup && c.Name == "Trip" {
			found = true
		}
	}
	if !found {
		t.Fatalf("created group %s not in chat list", jid)
	}
	info, err := f.GroupInfo(context.Background(), jid)
	if err != nil {
		t.Fatalf("GroupInfo: %v", err)
	}
	if len(info.Participants) != 2 {
		t.Errorf("participants = %d, want 2 (self + one)", len(info.Participants))
	}
}

func TestFakeCreateGroup_InvalidName(t *testing.T) {
	f := NewFake()
	if _, err := f.CreateGroup(context.Background(), "", nil); err == nil {
		t.Error("expected error for empty group name")
	}
}

func TestFakeLeaveGroup(t *testing.T) {
	f := NewFake()
	jid, _ := f.CreateGroup(context.Background(), "Trip", nil)
	if err := f.LeaveGroup(context.Background(), jid); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}
	chats, _ := f.Chats(0)
	for _, c := range chats {
		if c.JID == jid {
			t.Fatalf("group %s still in chat list after leaving", jid)
		}
	}
}

func TestFakeUpdateGroupParticipants(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	const g = "grp@g.us"
	member := "9990@s.whatsapp.net"

	if err := f.UpdateGroupParticipants(ctx, g, []string{member}, "add"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !hasParticipant(t, f, g, member, false) {
		t.Fatalf("member %s not added", member)
	}

	if err := f.UpdateGroupParticipants(ctx, g, []string{member}, "promote"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !hasParticipant(t, f, g, member, true) {
		t.Fatalf("member %s not promoted to admin", member)
	}

	if err := f.UpdateGroupParticipants(ctx, g, []string{member}, "demote"); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if !hasParticipant(t, f, g, member, false) {
		t.Fatalf("member %s not demoted", member)
	}

	if err := f.UpdateGroupParticipants(ctx, g, []string{member}, "remove"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	info, _ := f.GroupInfo(ctx, g)
	for _, p := range info.Participants {
		if p.JID == member {
			t.Fatalf("member %s not removed", member)
		}
	}
}

func TestFakeUpdateGroupParticipants_BadAction(t *testing.T) {
	f := NewFake()
	if err := f.UpdateGroupParticipants(context.Background(), "grp@g.us", []string{"x@s.whatsapp.net"}, "kick"); err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestFakeGroupJoinRequests_Seeded(t *testing.T) {
	reqs, err := NewFake().GroupJoinRequests(context.Background(), "weekendtrip@g.us")
	if err != nil {
		t.Fatalf("GroupJoinRequests: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("len = %d, want 1", len(reqs))
	}
}

func TestFakeGroupJoinRequests_NoneForUnknownGroup(t *testing.T) {
	reqs, err := NewFake().GroupJoinRequests(context.Background(), "other@g.us")
	if err != nil {
		t.Fatalf("GroupJoinRequests: %v", err)
	}
	if len(reqs) != 0 {
		t.Fatalf("len = %d, want 0", len(reqs))
	}
}

func TestFakeResolveGroupJoinRequest_Approve(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	const g = "weekendtrip@g.us"
	reqs, _ := f.GroupJoinRequests(ctx, g)
	requester := reqs[0].JID

	if err := f.ResolveGroupJoinRequest(ctx, g, requester, true); err != nil {
		t.Fatalf("ResolveGroupJoinRequest: %v", err)
	}
	if reqs, _ := f.GroupJoinRequests(ctx, g); len(reqs) != 0 {
		t.Fatalf("request still pending after approve: %v", reqs)
	}
	if !hasParticipant(t, f, g, requester, false) {
		t.Fatalf("approved requester %s not added to group", requester)
	}
}

func TestFakeResolveGroupJoinRequest_Reject(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	const g = "weekendtrip@g.us"
	reqs, _ := f.GroupJoinRequests(ctx, g)
	requester := reqs[0].JID

	if err := f.ResolveGroupJoinRequest(ctx, g, requester, false); err != nil {
		t.Fatalf("ResolveGroupJoinRequest: %v", err)
	}
	if reqs, _ := f.GroupJoinRequests(ctx, g); len(reqs) != 0 {
		t.Fatalf("request still pending after reject: %v", reqs)
	}
	info, _ := f.GroupInfo(ctx, g)
	for _, p := range info.Participants {
		if p.JID == requester {
			t.Fatalf("rejected requester %s was added to group", requester)
		}
	}
}

func TestFakeResolveGroupJoinRequest_Unknown(t *testing.T) {
	f := NewFake()
	err := f.ResolveGroupJoinRequest(context.Background(), "weekendtrip@g.us", "nobody@s.whatsapp.net", true)
	if err == nil {
		t.Error("expected error for unknown join request")
	}
}

func hasParticipant(t *testing.T, f *Fake, jid, member string, wantAdmin bool) bool {
	t.Helper()
	info, err := f.GroupInfo(context.Background(), jid)
	if err != nil {
		t.Fatalf("GroupInfo: %v", err)
	}
	for _, p := range info.Participants {
		if p.JID == member {
			return p.IsAdmin == wantAdmin
		}
	}
	return false
}
