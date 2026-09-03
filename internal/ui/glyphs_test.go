package ui

import "testing"

func TestCheckLineWidth(t *testing.T) {
	if got := checkLineWidth(13); got != 1.5 {
		t.Errorf("13px tick width = %v, want the 1.5 floor", got)
	}
	if got := checkLineWidth(20); got < 2.2 || got > 2.4 {
		t.Errorf("20px tick width = %v, want ≈2.3", got)
	}
}
