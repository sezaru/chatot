package ui

import (
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// traceSlots is how many level samples the recording trace keeps: one per
// column across the pill, newest at the right.
const traceSlots = 96

// traceStep is the horizontal pitch between columns, in px; traceDot the
// size of a column at silence (which is what draws the dotted baseline the
// bar shows before anyone speaks).
const (
	traceStep = 5.0
	traceDot  = 2.0
)

// levelTrace is the recording bar's meter: a row of dots along the pill
// that swell into bars with the microphone level, scrolling left as new
// readings arrive. Pure drawing over a ring of samples; the composer
// pushes one level every tick while recording.
type levelTrace struct {
	*gtk.DrawingArea
	levels []float64 // oldest first
}

func newLevelTrace() *levelTrace {
	t := &levelTrace{DrawingArea: gtk.NewDrawingArea(), levels: make([]float64, 0, traceSlots)}
	t.AddCSSClass("chatot-record-trace")
	t.SetHExpand(true)
	t.SetVAlign(gtk.AlignFill)
	t.SetContentHeight(20)
	t.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawTrace(cr, float64(w), float64(h), t.levels, t.Color())
	})
	return t
}

// Push appends a level (0..1) and redraws, dropping the oldest sample once
// the ring is full.
func (t *levelTrace) Push(level float64) {
	if len(t.levels) == traceSlots {
		copy(t.levels, t.levels[1:])
		t.levels = t.levels[:traceSlots-1]
	}
	t.levels = append(t.levels, clamp01(level))
	t.QueueDraw()
}

// Reset clears the ring for a new recording.
func (t *levelTrace) Reset() {
	t.levels = t.levels[:0]
	t.QueueDraw()
}

// traceColumns is the pure layout half of the trace: for a width w and the
// sample ring, the (x, barHeight) of every column to draw, newest at the
// right and any unfilled columns to the left drawn at the silence size.
// maxH is the tallest bar; a column with no sample yet is a dot.
func traceColumns(w, maxH float64, levels []float64) [][2]float64 {
	n := int(w / traceStep)
	if n <= 0 {
		return nil
	}
	cols := make([][2]float64, n)
	for i := 0; i < n; i++ {
		x := w - traceStep*float64(n-i) + traceStep/2
		h := traceDot
		if j := len(levels) - (n - i); j >= 0 {
			h = traceDot + levels[j]*(maxH-traceDot)
		}
		cols[i] = [2]float64{x, h}
	}
	return cols
}

func drawTrace(cr *cairo.Context, w, h float64, levels []float64, fg [4]float64) {
	cr.SetSourceRGBA(fg[0], fg[1], fg[2], fg[3]*0.55)
	mid := h / 2
	for _, c := range traceColumns(w, h, levels) {
		x, bh := c[0], c[1]
		roundedRectPath(cr, x-traceDot/2, mid-bh/2, traceDot, bh, traceDot/2)
		cr.Fill()
	}
}

// Color reads the widget's CSS foreground so the trace follows the pill's
// text colour in both themes.
func (t *levelTrace) Color() [4]float64 {
	c := t.DrawingArea.Color()
	return [4]float64{float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha())}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
