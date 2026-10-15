package ui

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

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

// A send refused for want of admin rights locks the strip at once, in that
// chat only, without waiting on the next read of the group.
func TestRefusedSendLocksTheComposer(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	c := NewComposer(client.NewFake())
	c.SetChat("g@g.us")
	if c.sendRefused() {
		t.Fatal("a group nobody has read yet is locked")
	}
	c.refuseSends("other@g.us")
	if c.sendRefused() {
		t.Error("a refusal in another chat locked this one")
	}
	c.refuseSends("g@g.us")
	if !c.sendRefused() {
		t.Error("the strip still takes messages after the group refused one")
	}
	c.SetChat("other@g.us")
	if !c.sendRefused() {
		t.Error("the other chat's refusal was forgotten when it opened")
	}
}

// refusingGroup is a backend whose group is admins-only by now, though the
// first read of it, slow to come back, still says otherwise.
type refusingGroup struct {
	*client.Fake
	firstRead chan struct{} // closed to let the first read return
	reads     atomic.Int32
}

func (r *refusingGroup) GroupInfo(ctx context.Context, jid string) (*client.GroupInfo, error) {
	if r.reads.Add(1) == 1 {
		<-r.firstRead
		return &client.GroupInfo{JID: jid}, nil
	}
	return &client.GroupInfo{JID: jid, Announce: true}, nil
}

func (r *refusingGroup) SendText(ctx context.Context, jid, text string, replyTo *client.MsgRef) (string, error) {
	return "", fmt.Errorf("chatot/client: send text: %w", client.ErrSendNotAllowed)
}

// A send refused while an older read of the group is still out: the strip
// locks, the older read's "open" does not unlock it, and the read that
// follows the refusal settles it.
func TestRefusedSendOutlivesAnOlderRead(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	b := &refusingGroup{Fake: client.NewFake(), firstRead: make(chan struct{})}
	c := NewComposer(b)
	c.SetChat("g@g.us")
	c.dispatch(client.Message{ChatJID: "g@g.us", FromMe: true, Text: "hi"})
	pump := func(what string, done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !done(); {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			glib.MainContextDefault().Iteration(false)
			time.Sleep(time.Millisecond)
		}
	}
	pump("the refusal", c.sendRefused)
	close(b.firstRead)
	unlocked := false
	pump("the read after the refusal", func() bool {
		unlocked = unlocked || !c.sendRefused()
		return b.reads.Load() == 2 && !c.blockFetching
	})
	if unlocked || !c.sendRefused() {
		t.Error("an older read unlocked the strip after the group refused a send")
	}
}
