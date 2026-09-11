package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestWantsArrivalTranscript(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	voice := func(ts int64) client.Message {
		return client.Message{ID: "v", TS: ts, Attachment: &client.Attachment{Kind: "audio"}}
	}
	fresh := now.Unix() - 60
	cases := []struct {
		name string
		m    client.Message
		auto bool
		dl   string
		want bool
	}{
		{"fresh note under both preferences", voice(fresh), true, "photos", true},
		{"download set to always", voice(fresh), true, "always", true},
		{"preference off", voice(fresh), false, "photos", false},
		{"no automatic downloads, not on disk", voice(fresh), true, "never", false},
		{"no automatic downloads, already on disk", func() client.Message {
			m := voice(fresh)
			m.Attachment.LocalPath = "/tmp/x.ogg"
			return m
		}(), true, "never", true},
		{"a week-old note", voice(now.Add(-8 * 24 * time.Hour).Unix()), true, "always", false},
		{"already transcribed", func() client.Message {
			m := voice(fresh)
			m.Attachment.Transcript = "hi"
			return m
		}(), true, "always", false},
		{"a photo", client.Message{ID: "p", TS: fresh, Attachment: &client.Attachment{Kind: "image"}}, true, "always", false},
		{"plain text", client.Message{ID: "t", TS: fresh, Text: "hi"}, true, "always", false},
		{"a deleted note", func() client.Message {
			m := voice(fresh)
			m.Deleted = true
			return m
		}(), true, "always", false},
	}
	for _, c := range cases {
		if got := wantsArrivalTranscript(c.m, c.auto, c.dl, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
