package ui

import (
	"math"
	"testing"
)

func TestGlyphAlign(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

	// Ink that fills its logical box symmetrically needs no correction.
	x, y := glyphAlign(28, 28, 0, 0, 10, 16, 0, 0, 10, 16)
	if !near(x, 0.5) || !near(y, 0.5) {
		t.Errorf("symmetric = (%v, %v), want (0.5, 0.5)", x, y)
	}

	// A capital: 16px logical box, ink from 2 to 12 (high, leaving the
	// descender space empty). Centring the box would put the ink 1px high,
	// so yalign moves down: (14 - 2 - 5) / (28 - 16) = 7/12.
	_, y = glyphAlign(28, 28, 0, 2, 10, 10, 0, 0, 10, 16)
	if !near(y, 7.0/12) {
		t.Errorf("capital yalign = %v, want %v", y, 7.0/12)
	}

	// An emoji whose ink starts left of the logical origin shifts right.
	x, _ = glyphAlign(28, 28, -1, 0, 14, 14, 0, 0, 16, 14)
	if want := (14.0 + 1 - 7) / 12; !near(x, want) {
		t.Errorf("emoji xalign = %v, want %v", x, want)
	}

	// No room on an axis: keep GTK's default rather than dividing by zero.
	x, y = glyphAlign(10, 10, 0, 0, 10, 10, 0, 0, 10, 10)
	if !near(x, 0.5) || !near(y, 0.5) {
		t.Errorf("no room = (%v, %v), want (0.5, 0.5)", x, y)
	}

	// The result is clamped to an alignment fraction.
	x, _ = glyphAlign(12, 12, -20, 0, 4, 4, 0, 0, 10, 10)
	if x != 1 {
		t.Errorf("clamped xalign = %v, want 1", x)
	}
}
