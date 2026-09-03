package ui

import (
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// newCheckGlyph is a size×size cairo-drawn ✓ in the widget's CSS colour,
// used wherever the design shows a tick inside a disc or a box (pickers,
// polls, the verified mark, the current-account row). Drawing it beats a
// "✓" label: the glyph then needs no font that has U+2713, which some
// systems lack (the tick came out as a hex box there). Background, border
// and radius stay in CSS on the class the caller adds. on=false draws
// nothing, keeping the disc for an unticked row.
func newCheckGlyph(size int, on bool) *gtk.DrawingArea {
	area := gtk.NewDrawingArea()
	area.SetSizeRequest(size, size)
	area.SetHAlign(gtk.AlignCenter)
	area.SetVAlign(gtk.AlignCenter)
	area.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		if !on {
			return
		}
		c := area.Color()
		cr.SetSourceRGBA(float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha()))
		drawCheck(cr, float64(w), float64(h))
	})
	return area
}

// drawCheck strokes a tick centred in w×h: the short leg down to the
// baseline, the long one up, with rounded ends like the mockup's glyph.
func drawCheck(cr *cairo.Context, w, h float64) {
	s := w
	if h < s {
		s = h
	}
	ox, oy := (w-s)/2, (h-s)/2
	cr.SetLineWidth(checkLineWidth(s))
	cr.SetLineCap(cairo.LineCapRound)
	cr.SetLineJoin(cairo.LineJoinRound)
	cr.MoveTo(ox+s*0.26, oy+s*0.53)
	cr.LineTo(ox+s*0.43, oy+s*0.70)
	cr.LineTo(ox+s*0.75, oy+s*0.34)
	cr.Stroke()
}

// checkLineWidth scales the tick's stroke with its box, never thinner than
// a crisp 1.5px so the 13px verified mark still reads.
func checkLineWidth(size float64) float64 {
	if lw := size * 0.115; lw > 1.5 {
		return lw
	}
	return 1.5
}

// newRoundButton is a size×size circular button around child. The child
// rides in an overlay over a fixed-size base, so a wide emoji or a wide
// font's ⋮ can't stretch the circle into a rounded rectangle.
func newRoundButton(child gtk.Widgetter, size int) *gtk.Button {
	base := gtk.NewBox(gtk.OrientationVertical, 0)
	base.SetSizeRequest(size, size)
	gtk.BaseWidget(child).SetHAlign(gtk.AlignCenter)
	gtk.BaseWidget(child).SetVAlign(gtk.AlignCenter)
	stack := gtk.NewOverlay()
	stack.SetChild(base)
	stack.AddOverlay(child)
	btn := gtk.NewButton()
	btn.SetChild(stack)
	btn.AddCSSClass("chatot-round-btn")
	btn.SetHAlign(gtk.AlignCenter)
	btn.SetVAlign(gtk.AlignCenter)
	return btn
}

// newRoundGlyphButton is newRoundButton around a single text glyph.
func newRoundGlyphButton(glyph string, size int) *gtk.Button {
	return newRoundButton(gtk.NewLabel(glyph), size)
}

// drawPausePlay fills the current colour with two bars (paused=false: the
// clock runs, the button offers to pause) or a play triangle (the button
// offers to resume), centred in w×h.
func drawPausePlay(cr *cairo.Context, w, h float64, paused bool) {
	cr.SetSourceRGB(1, 1, 1)
	if paused {
		cr.MoveTo(w*0.22, h*0.08)
		cr.LineTo(w*0.92, h*0.5)
		cr.LineTo(w*0.22, h*0.92)
		cr.ClosePath()
		cr.Fill()
		return
	}
	cr.Rectangle(w*0.18, h*0.1, w*0.24, h*0.8)
	cr.Rectangle(w*0.58, h*0.1, w*0.24, h*0.8)
	cr.Fill()
}
