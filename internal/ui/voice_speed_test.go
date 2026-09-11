package ui

import "testing"

func TestSpeedLabel(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want string
	}{{1, "1×"}, {1.5, "1.5×"}, {2, "2×"}} {
		if got := speedLabel(c.in); got != c.want {
			t.Errorf("speedLabel(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlayerTimelineFollowsSpeed(t *testing.T) {
	// A 60 s note rendered at 2× is a 30 s file, but the row keeps
	// reporting the note's own length.
	p := &mediaPlayer{seconds: 60, wantSeek: -1, rate: 2}
	if got := p.streamSeconds(); got != 30 {
		t.Fatalf("streamSeconds = %v, want 30", got)
	}
	if got := p.Duration(); got != 60 {
		t.Fatalf("Duration = %v, want 60", got)
	}
	plain := &mediaPlayer{seconds: 60, wantSeek: -1}
	if plain.Speed() != 1 || plain.Duration() != 60 {
		t.Fatalf("an unrendered player is 1×: speed %v, duration %v", plain.Speed(), plain.Duration())
	}
	if plain.speedable() {
		t.Fatal("a player without a source is not speedable")
	}
}
