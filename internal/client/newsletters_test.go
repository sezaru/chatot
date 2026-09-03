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
	if len(ns) != 3 {
		t.Fatalf("want 3 seeded channels, got %d", len(ns))
	}
	// Sorted by name: the book club, then "Chatot News", then "Weather Alerts".
	if ns[0].Name != "Ada's Book Club" || ns[1].Name != "Chatot News" || ns[2].Name != "Weather Alerts" {
		t.Errorf("unexpected channel order: %q, %q, %q", ns[0].Name, ns[1].Name, ns[2].Name)
	}
	if !ns[2].Muted {
		t.Errorf("expected Weather Alerts to be muted")
	}

	msgs, err := f.NewsletterMessages(ctx, "111111@newsletter", 20)
	if err != nil {
		t.Fatalf("NewsletterMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("want 3 posts, got %d", len(msgs))
	}
	// Newest first, and the photo post carries its attachment.
	if msgs[0].TS < msgs[1].TS {
		t.Errorf("posts not newest-first")
	}
	if msgs[0].ID != "n2b" || msgs[0].Attachment == nil || msgs[0].Attachment.Kind != "image" {
		t.Errorf("photo post = %+v, want n2b with an image attachment", msgs[0])
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
	count := func(emoji string) int {
		msgs, _ := f.NewsletterMessages(ctx, "111111@newsletter", 20)
		for _, m := range msgs {
			if m.ID == "n2" {
				return m.Reactions[emoji]
			}
		}
		return -1
	}
	if err := f.NewsletterReact(ctx, "111111@newsletter", "n2", 2, "🔥"); err != nil {
		t.Fatalf("React: %v", err)
	}
	if count("🔥") != 1 {
		t.Errorf("reaction not recorded, 🔥 = %d", count("🔥"))
	}
	// One reaction per post: the same emoji again must not pile up, and a
	// different one replaces it.
	_ = f.NewsletterReact(ctx, "111111@newsletter", "n2", 2, "🔥")
	if count("🔥") != 1 {
		t.Errorf("repeat react drifted, 🔥 = %d", count("🔥"))
	}
	_ = f.NewsletterReact(ctx, "111111@newsletter", "n2", 2, "❤️")
	if count("🔥") != 0 || count("❤️") != 1 {
		t.Errorf("switch: 🔥 = %d ❤️ = %d", count("🔥"), count("❤️"))
	}
	_ = f.NewsletterReact(ctx, "111111@newsletter", "n2", 2, "")
	if count("❤️") != 0 {
		t.Errorf("withdraw: ❤️ = %d", count("❤️"))
	}
}
