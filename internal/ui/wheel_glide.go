package ui

import (
	"math"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// glider scrolls a scrolled window on at a velocity that decays every
// frame. Each step is relative to the value GTK holds at that moment, never
// an absolute position (thread_input.go explains why that matters over a
// GtkListView), so the same code serves the thread and the sidebar.
type glider struct {
	scroller *gtk.ScrolledWindow
	// gen tells a running glide from a stopped one.
	gen int
}

func newGlider(scroller *gtk.ScrolledWindow) *glider {
	return &glider{scroller: scroller}
}

// stop ends a glide in progress.
func (g *glider) stop() { g.gen++ }

// start scrolls on at velocity (value units per second), decaying by
// e^-friction per second, until it is spent or the content ends.
func (g *glider) start(velocity, friction float64) {
	g.gen++
	gen := g.gen
	last := time.Now()
	gtk.BaseWidget(g.scroller).AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		if gen != g.gen {
			return false
		}
		now := time.Now()
		dt := now.Sub(last).Seconds()
		last = now
		if dt > 0.1 {
			dt = 0.1
		}
		adj := g.scroller.VAdjustment()
		before := adj.Value()
		adj.SetValue(before + velocity*dt)
		velocity *= math.Exp(-friction * dt)
		if math.Abs(velocity) < 2 || adj.Value() == before {
			return false
		}
		return true
	})
}

// smoothWheel makes a wheel notch over scroller glide over a few frames
// instead of jumping its whole step at once, as the thread's input does.
// Touchpad deltas and kinetic scrolling are left to GtkScrolledWindow. A
// notch's step is GTK's own: page^(2/3).
func smoothWheel(scroller *gtk.ScrolledWindow) {
	g := newGlider(scroller)
	ctl := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical)
	ctl.SetPropagationPhase(gtk.PhaseCapture)
	ctl.ConnectScroll(func(_, dy float64) bool {
		if ctl.Unit() != gdk.ScrollUnitWheel {
			g.stop()
			return false
		}
		step := math.Pow(scroller.VAdjustment().PageSize(), 2.0/3.0)
		g.start(dy*step*wheelStepFriction, wheelStepFriction)
		return true
	})
	scroller.AddController(ctl)
}
