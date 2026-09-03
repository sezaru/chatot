package ui

import (
	"reflect"
	"testing"

	"chatot/internal/client"
)

func sampleCommunity() client.Community {
	return client.Community{
		JID: "c@g.us", Name: "Bloco B — Residents", MemberCount: 128, IsAdmin: true,
		Groups: []client.CommunityGroup{
			{JID: "ann@g.us", Name: "Announcements", Announcement: true, Joined: true, MemberCount: 128, UnreadCount: 3},
			{JID: "geral@g.us", Name: "Bloco B — Geral", Joined: true, Preview: "Priya: the lift is fixed", UnreadCount: 12},
			{JID: "pool@g.us", Name: "Piscina", MemberCount: 34},
		},
	}
}

func TestCommunityRowVM(t *testing.T) {
	vm := communityRowVM(sampleCommunity())
	if vm.Sub != "2 groups · 128 members" {
		t.Errorf("Sub = %q", vm.Sub)
	}
	if vm.Unread != "15" {
		t.Errorf("Unread = %q, want 15", vm.Unread)
	}
	if vm.Initial != "B" {
		t.Errorf("Initial = %q", vm.Initial)
	}
}

func TestCommunityPaneSub(t *testing.T) {
	c := sampleCommunity()
	if got := communityPaneSub(c); got != "128 members · 2 groups · you are an admin" {
		t.Errorf("communityPaneSub = %q", got)
	}
	c.IsAdmin, c.Muted = false, true
	if got := communityPaneSub(c); got != "128 members · 2 groups · muted" {
		t.Errorf("communityPaneSub(muted) = %q", got)
	}
}

func TestCommunitySections(t *testing.T) {
	secs := communitySections(sampleCommunity())
	var captions []string
	for _, s := range secs {
		captions = append(captions, s.Caption)
	}
	want := []string{"Announcement group", "Groups you're in", "Other groups in this community"}
	if !reflect.DeepEqual(captions, want) {
		t.Errorf("captions = %v, want %v", captions, want)
	}
	if !secs[2].CanJoin || secs[0].CanJoin {
		t.Errorf("only the other-groups block offers Join")
	}
	// An empty block disappears rather than showing a bare caption.
	c := sampleCommunity()
	c.Groups = c.Groups[:1]
	if got := len(communitySections(c)); got != 1 {
		t.Errorf("sections with only an announcement group = %d, want 1", got)
	}
}

func TestCommunityGroupSub(t *testing.T) {
	c := sampleCommunity()
	if got := communityGroupSub(c.Groups[0]); got != "Only admins can post · 128 members" {
		t.Errorf("announcement sub = %q", got)
	}
	if got := communityGroupSub(c.Groups[1]); got != "Priya: the lift is fixed" {
		t.Errorf("joined sub = %q", got)
	}
	if got := communityGroupSub(c.Groups[2]); got != "34 members" {
		t.Errorf("not-joined sub = %q", got)
	}
	states := []string{communityGroupState(c.Groups[0]), communityGroupState(c.Groups[1]), communityGroupState(c.Groups[2])}
	if !reflect.DeepEqual(states, []string{"announcements", "joined", "not joined"}) {
		t.Errorf("states = %v", states)
	}
}

func TestLinkableGroups(t *testing.T) {
	chats := []client.Chat{
		{JID: "trip@g.us", Name: "Weekend Trip", IsGroup: true},
		{JID: "geral@g.us", Name: "Bloco B — Geral", IsGroup: true},
		{JID: "old@g.us", Name: "Archived", IsGroup: true, Archived: true},
		{JID: "ada@s.whatsapp.net", Name: "Ada"},
		{JID: "c@g.us", Name: "Bloco B — Residents", IsGroup: true},
	}
	got := linkableGroups(chats, []client.Community{sampleCommunity()})
	if len(got) != 1 || got[0].JID != "trip@g.us" {
		t.Errorf("linkableGroups = %+v", got)
	}
	if linkPickLabel(0) != "Pick one or more groups" || linkPickLabel(1) != "1 group selected" || linkPickLabel(2) != "2 groups selected" {
		t.Errorf("linkPickLabel wording")
	}
}

func TestJoinLinkHint(t *testing.T) {
	if text, ok := joinLinkHint(""); text != "" || ok {
		t.Errorf("empty input should have no hint")
	}
	if text, ok := joinLinkHint("https://chat.whatsapp.com/AbCdEf123"); !ok || text != "Link looks valid" {
		t.Errorf("valid link: %q %v", text, ok)
	}
	if _, ok := joinLinkHint("hello"); ok {
		t.Errorf("garbage should not validate")
	}
}

func TestCommunityCreatedText(t *testing.T) {
	names := map[string]string{"g@s.whatsapp.net": "Grace"}
	c := client.Community{Created: 1710201600, CreatorJID: "g@s.whatsapp.net"} // 12 Mar 2024 UTC
	got := communityCreatedText(c, names, "me@s.whatsapp.net")
	if got == "" || got[:8] != "Created " || got[len(got)-9:] != " by Grace" {
		t.Errorf("communityCreatedText = %q", got)
	}
	if got := communityCreatedText(client.Community{}, names, ""); got != "" {
		t.Errorf("unknown creation time should be blank, got %q", got)
	}
}
