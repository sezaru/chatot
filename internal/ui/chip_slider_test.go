package ui

import "testing"

func TestSliderThumb(t *testing.T) {
	if x, tw := sliderThumb(100, 100, 0, 200); x != 0 || tw != 200 {
		t.Errorf("no overflow: thumb = (%v, %v), want the whole track", x, tw)
	}
	if x, tw := sliderThumb(400, 200, 0, 200); x != 0 || tw != 100 {
		t.Errorf("half visible at start: thumb = (%v, %v), want (0, 100)", x, tw)
	}
	if x, tw := sliderThumb(400, 200, 200, 200); x != 100 || tw != 100 {
		t.Errorf("half visible at end: thumb = (%v, %v), want (100, 100)", x, tw)
	}
	if _, tw := sliderThumb(10000, 100, 0, 200); tw != sliderMinThumb {
		t.Errorf("tiny share keeps the minimum thumb: %v", tw)
	}
	if x, tw := sliderThumb(400, 200, 200, 20); x != 0 || tw != 20 {
		t.Errorf("track narrower than the minimum thumb: (%v, %v)", x, tw)
	}
}

func TestSliderValueAt(t *testing.T) {
	if v := sliderValueAt(400, 200, 200, 100); v != 200 {
		t.Errorf("thumb at the far end = %v, want 200", v)
	}
	if v := sliderValueAt(400, 200, 200, 50); v != 100 {
		t.Errorf("thumb halfway = %v, want 100", v)
	}
	if v := sliderValueAt(400, 200, 200, -30); v != 0 {
		t.Errorf("past the start clamps: %v", v)
	}
	if v := sliderValueAt(400, 200, 200, 900); v != 200 {
		t.Errorf("past the end clamps: %v", v)
	}
	if v := sliderValueAt(100, 200, 200, 50); v != 0 {
		t.Errorf("nothing to scroll: %v", v)
	}
	if v := sliderValueAt(400, 200, 10, 5); v != 0 {
		t.Errorf("track narrower than the thumb: %v", v)
	}
}
