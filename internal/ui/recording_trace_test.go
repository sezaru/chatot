package ui

import "testing"

func TestTraceColumnsNewestAtRight(t *testing.T) {
	cols := traceColumns(50, 20, []float64{0, 1})
	if len(cols) != 10 {
		t.Fatalf("got %d columns, want 10", len(cols))
	}
	// Unfilled columns are dots; the last two carry the two samples.
	if cols[0][1] != traceDot || cols[7][1] != traceDot {
		t.Errorf("empty columns should be dots: %v", cols)
	}
	if cols[8][1] != traceDot || cols[9][1] != 20 {
		t.Errorf("samples misplaced: %v", cols[8:])
	}
	if cols[9][0] <= cols[8][0] {
		t.Errorf("columns must advance rightwards: %v", cols[8:])
	}
	if got := traceColumns(0, 20, nil); got != nil {
		t.Errorf("zero width should yield no columns, got %v", got)
	}
}
