package client

import (
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestAvatarCacheName(t *testing.T) {
	cases := []struct {
		jid  string
		want string
	}{
		{"1234567890@s.whatsapp.net", "1234567890_s.whatsapp.net.jpg"},
		{"120363012345678901@g.us", "120363012345678901_g.us.jpg"},
		{"1234567890:1@s.whatsapp.net", "1234567890_1_s.whatsapp.net.jpg"},
	}
	for _, c := range cases {
		if got := avatarCacheName(c.jid); got != c.want {
			t.Errorf("avatarCacheName(%q) = %q, want %q", c.jid, got, c.want)
		}
	}
}

func TestAvatarCacheNameNoPathSeparators(t *testing.T) {
	if got := avatarCacheName("a/b@c.d"); got != "a_b_c.d.jpg" {
		t.Errorf("avatarCacheName(%q) = %q, want no '/' left in it", "a/b@c.d", got)
	}
}

func TestAvatarDefinitelyMissingOnlyForFinalAnswers(t *testing.T) {
	for _, err := range []error{whatsmeow.ErrProfilePictureNotSet, whatsmeow.ErrProfilePictureUnauthorized, whatsmeow.ErrNotInGroup} {
		if !avatarDefinitelyMissing(err) {
			t.Errorf("%v: want missing", err)
		}
	}
	for _, err := range []error{whatsmeow.ErrNotConnected, whatsmeow.ErrNotLoggedIn, whatsmeow.ErrIQTimedOut} {
		if avatarDefinitelyMissing(err) {
			t.Errorf("%v: transient, must not be memoized", err)
		}
	}
}
