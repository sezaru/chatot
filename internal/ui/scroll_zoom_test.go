package ui

import (
	"math"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
)

func TestScrollZoomFactor(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	// A wheel notch up zooms in one step, down zooms back out.
	if f := scrollZoomFactor(gdk.ScrollUnitWheel, -1); !near(f, wheelZoomPerNotch) {
		t.Errorf("wheel notch up = %v, want %v", f, wheelZoomPerNotch)
	}
	if f := scrollZoomFactor(gdk.ScrollUnitWheel, 1); !near(f, 1/wheelZoomPerNotch) {
		t.Errorf("wheel notch down = %v, want %v", f, 1/wheelZoomPerNotch)
	}
	// A touchpad swipe of 100 pixels, in ten events, stays a modest zoom
	// (it used to count each pixel as a notch: 1.12^100).
	total := 1.0
	for range 10 {
		total *= scrollZoomFactor(gdk.ScrollUnitSurface, -10)
	}
	if total < 1.5 || total > 1.8 {
		t.Errorf("100px touchpad swipe zooms %v, want about 1.65", total)
	}
}

func TestScrollSteps(t *testing.T) {
	var s scrollSteps
	// Whole notches step at once; fractions add up to one.
	if n := s.add(-1); n != -1 {
		t.Errorf("notch up = %d, want -1", n)
	}
	got := 0
	for range 8 {
		got += s.add(0.125)
	}
	if got != 1 {
		t.Errorf("eight eighth-notches = %d steps, want 1", got)
	}
	// A touchpad at 100px per level: 250px down is two levels, the rest
	// carries; turning back drops it.
	s.reset()
	got = 0
	for range 25 {
		got += s.add(10.0 / mapTouchpadPixelsPerLevel)
	}
	if got != 2 {
		t.Errorf("250px = %d levels, want 2", got)
	}
	if n := s.add(-0.6); n != 0 {
		t.Errorf("turning back 60px = %d levels, want 0 (the 50px carried down is dropped)", n)
	}
	if n := s.add(-0.5); n != -1 {
		t.Errorf("110px back up = %d levels, want -1", n)
	}
}
