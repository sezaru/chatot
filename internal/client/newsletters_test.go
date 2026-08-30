package client

import (
	"context"
	"testing"
)

func TestParseChannelInvite(t *testing.T) {
	cases := map[string]string{
		"https://whatsapp.com/channel/AbCdEf123":     "AbCdEf123",
		"http://whatsapp.com/channel/AbCdEf123":      "AbCdEf123",
		"https://www.whatsapp.com/channel/AbCdEf123": "AbCdEf123",
		"whatsapp.com/channel/AbCdEf123":             "AbCdEf123",
		"whatsapp.com/channel/AbCdEf123/":            "AbCdEf123",
		"  AbCdEf123  ":                              "AbCdEf123",
		"AbCdEf123":                                  "AbCdEf123",
		"":                                           "",
	}
	for in, want := range cases {
		if got := parseChannelInvite(in); got != want {
			t.Errorf("parseChannelInvite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFakeNewsletters(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	ns, err := f.Newsletters(ctx)
	if err != nil {
		t.Fatalf("Newsletters: %v", err)
	}
	if len(ns) != 2 {
		t.Fatalf("want 2 seeded channels, got %d", len(ns))
	}
	// Sorted by name: "Chatot News" before "Weather Alerts".
	if ns[0].Name != "Chatot News" || ns[1].Name != "Weather Alerts" {
		t.Errorf("unexpected channel order: %q, %q", ns[0].Name, ns[1].Name)
	}
	if !ns[1].Muted {
		t.Errorf("expected Weather Alerts to be muted")
	}

	msgs, err := f.NewsletterMessages(ctx, "111111@newsletter", 20)
	if err != nil {
		t.Fatalf("NewsletterMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 posts, got %d", len(msgs))
	}
	// Newest first.
	if msgs[0].TS < msgs[1].TS {
		t.Errorf("posts not newest-first")
	}
}

func TestFakeNewsletterMutate(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	if err := f.NewsletterSetMuted(ctx, "111111@newsletter", true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}
	ns, _ := f.Newsletters(ctx)
	for _, n := range ns {
		if n.ID == "111111@newsletter" && !n.Muted {
			t.Errorf("channel not muted after SetMuted")
		}
	}

	if err := f.UnfollowNewsletter(ctx, "111111@newsletter"); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	ns, _ = f.Newsletters(ctx)
	for _, n := range ns {
		if n.ID == "111111@newsletter" {
			t.Errorf("channel still present after Unfollow")
		}
	}

	jid, err := f.FollowNewsletterByLink(ctx, "https://whatsapp.com/channel/NewKey42")
	if err != nil {
		t.Fatalf("FollowByLink: %v", err)
	}
	if jid != "NewKey42@newsletter" {
		t.Errorf("unexpected followed jid %q", jid)
	}
	ns, _ = f.Newsletters(ctx)
	found := false
	for _, n := range ns {
		if n.ID == jid {
			found = true
		}
	}
	if !found {
		t.Errorf("followed channel not in list")
	}
}

func TestFakeNewsletterReact(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	if err := f.NewsletterReact(ctx, "111111@newsletter", "n2", 2, "🔥"); err != nil {
		t.Fatalf("React: %v", err)
	}
	msgs, _ := f.NewsletterMessages(ctx, "111111@newsletter", 20)
	for _, m := range msgs {
		if m.ID == "n2" && m.Reactions["🔥"] != 1 {
			t.Errorf("reaction not recorded, got %v", m.Reactions)
		}
	}
}
