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
	// velocity and friction are the running glide's; running is set
	// while its tick callback is alive.
	velocity, friction float64
	running            bool
}

func newGlider(scroller *gtk.ScrolledWindow) *glider {
	return &glider{scroller: scroller}
}

// stop ends a glide in progress.
func (g *glider) stop() {
	g.gen++
	g.running = false
}

// add puts velocity on top of a glide in progress with the same friction,
// or starts one. A glide covers velocity/friction in all; adding to it
// adds that distance, so a wheel notch arriving before the previous one
// is spent (a few frames, at wheelStepFriction) still scrolls its full
// step. Restarting instead dropped what was left of every notch but the
// last, and a quick spin of the wheel moved a fraction of its notches.
func (g *glider) add(velocity, friction float64) {
	if g.running && g.friction == friction {
		g.velocity += velocity
		return
	}
	g.start(velocity, friction)
}

// start scrolls on at velocity (value units per second), decaying by
// e^-friction per second, until it is spent or the content ends.
func (g *glider) start(velocity, friction float64) {
	g.gen++
	gen := g.gen
	g.velocity, g.friction, g.running = velocity, friction, true
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
		adj.SetValue(before + g.velocity*dt)
		g.velocity *= math.Exp(-g.friction * dt)
		if math.Abs(g.velocity) < 2 || adj.Value() == before {
			g.running = false
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
		g.add(dy*step*wheelStepFriction, wheelStepFriction)
		return true
	})
	scroller.AddController(ctl)
}
