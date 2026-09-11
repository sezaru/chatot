package ui

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// chipSliderH is the slider's drawn height; chipSliderPad the room around it.
const (
	chipSliderH   = 3.0
	chipSliderPad = 2
)

// newChipSlider is the filter strip's scroll indicator: a 3px track with a
// thumb sized to the visible share, shown only while the chips overflow.
// Dragging the thumb or clicking the track scrolls the strip, so a pointer
// with no sideways wheel still reaches every chip.
func newChipSlider(adj *gtk.Adjustment) *gtk.DrawingArea {
	s := gtk.NewDrawingArea()
	s.AddCSSClass("chatot-chip-slider")
	s.SetContentHeight(int(chipSliderH))
	s.SetMarginTop(chipSliderPad)
	s.SetMarginBottom(chipSliderPad)
	s.SetMarginStart(14)
	s.SetMarginEnd(14)
	s.SetVAlign(gtk.AlignStart)
	s.SetVExpand(false)
	s.SetVisible(false)

	thumb := func(w float64) (x, tw float64) {
		return sliderThumb(adj.Upper(), adj.PageSize(), adj.Value(), w)
	}
	s.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		fg := s.StyleContext().Color()
		r, g, b := float64(fg.Red()), float64(fg.Green()), float64(fg.Blue())
		pollPill(cr, 0, 0, float64(w), float64(h))
		cr.SetSourceRGBA(r, g, b, 0.1)
		cr.Fill()
		x, tw := thumb(float64(w))
		pollPill(cr, x, 0, tw, float64(h))
		cr.SetSourceRGBA(r, g, b, 0.4)
		cr.Fill()
	})
	// The adjustment changes while the strip is being allocated; showing
	// or hiding the slider then would resize mid-allocation, so it waits
	// for the next idle.
	refresh := func() {
		glib.IdleAdd(func() {
			s.SetVisible(adj.Upper() > adj.PageSize()+0.5)
			s.QueueDraw()
		})
	}
	adj.ConnectChanged(refresh)
	adj.ConnectValueChanged(refresh)

	// Drag moves the thumb by the pointer's travel; a press elsewhere on
	// the track centres the thumb there first.
	drag := gtk.NewGestureDrag()
	startValue := 0.0
	drag.ConnectDragBegin(func(x, _ float64) {
		w := float64(s.AllocatedWidth())
		tx, tw := thumb(w)
		if x < tx || x > tx+tw {
			adj.SetValue(sliderValueAt(adj.Upper(), adj.PageSize(), w, x-tw/2))
		}
		startValue = adj.Value()
	})
	drag.ConnectDragUpdate(func(dx, _ float64) {
		w := float64(s.AllocatedWidth())
		_, tw := thumb(w)
		if w-tw <= 0 {
			return
		}
		adj.SetValue(startValue + dx*(adj.Upper()-adj.PageSize())/(w-tw))
	})
	s.AddController(drag)
	return s
}

// sliderMinThumb keeps the thumb graspable on a long strip.
const sliderMinThumb = 24.0

// sliderThumb is the thumb's left edge and width on a track w wide for a
// scroll range (upper, page) at value; the whole track when nothing
// overflows.
func sliderThumb(upper, page, value, w float64) (x, tw float64) {
	if upper <= 0 || page >= upper {
		return 0, w
	}
	tw = w * page / upper
	if tw < sliderMinThumb {
		tw = sliderMinThumb
	}
	if tw > w {
		tw = w
	}
	x = (w - tw) * value / (upper - page)
	return x, tw
}

// sliderValueAt is the value that puts the thumb's left edge at x on a
// track w wide.
func sliderValueAt(upper, page, w, x float64) float64 {
	if upper <= page {
		return 0
	}
	_, tw := sliderThumb(upper, page, 0, w)
	if w-tw <= 0 {
		return 0
	}
	return clampF(x/(w-tw)*(upper-page), 0, upper-page)
}

// chipFadeW is the width of the strip's edge fades.
const chipFadeW = 28

// newChipStrip wraps the chip scroller in an overlay with a fade at each
// edge. A fade shows while chips lie beyond its edge and goes once the
// strip is scrolled to that edge, so the row reads as cut off only where
// it is. The fades are not targets: the chips under them stay clickable.
func newChipStrip(scroller *gtk.ScrolledWindow) *gtk.Overlay {
	o := gtk.NewOverlay()
	o.SetChild(scroller)
	fade := func(class string, align gtk.Align) *gtk.Box {
		f := gtk.NewBox(gtk.OrientationHorizontal, 0)
		f.AddCSSClass("chatot-chip-fade")
		f.AddCSSClass(class)
		f.SetHAlign(align)
		f.SetSizeRequest(chipFadeW, -1)
		f.SetCanTarget(false)
		o.AddOverlay(f)
		o.SetMeasureOverlay(f, false)
		return f
	}
	start := fade("chatot-chip-fade-start", gtk.AlignStart)
	end := fade("chatot-chip-fade-end", gtk.AlignEnd)
	adj := scroller.HAdjustment()
	refresh := func() {
		v := adj.Value()
		setCSSClass(start, "chatot-chip-fade-on", v > 0.5)
		setCSSClass(end, "chatot-chip-fade-on", v < adj.Upper()-adj.PageSize()-0.5)
	}
	adj.ConnectChanged(refresh)
	adj.ConnectValueChanged(refresh)
	return o
}

// ScrollChips scrolls the filter strip to px pixels, or to its end for
// "end". A screenshot hook.
func (cl *ChatList) ScrollChips(px string) {
	adj := cl.chipScroller.HAdjustment()
	if px == "end" {
		adj.SetValue(adj.Upper() - adj.PageSize())
		return
	}
	var v float64
	fmt.Sscanf(px, "%g", &v)
	adj.SetValue(v)
}
