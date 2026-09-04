package ui

import (
	"math"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Wheel and touchpad scrolling over the thread is handled here rather than
// by GtkScrolledWindow. The list view's scrollable height is an estimate
// wherever rows are not realized (the median height of the realized ones,
// gtklistview.c), and it moves every frame as rows come and go, so the
// same scroll value maps to different content from one frame to the next.
// GTK keeps the row under the reader in place by re-deriving the value
// from that row after each layout; a value written from outside as an
// absolute position, which is what its kinetic deceleration does every
// frame, lands on the new estimate instead, and the thread jumped up and
// down through every fling. Every change made here is relative to the
// value GTK last derived, and the deceleration after a touchpad swipe is
// done the same way.

// scrollFriction is GtkScrolledWindow's DECELERATION_FRICTION: the
// velocity of a fling decays by e^-friction per second.
const scrollFriction = 4.0

// wheelStepFriction is the decay used to smooth a wheel notch over a few
// frames instead of jumping by its full step.
const wheelStepFriction = 14.0

// scrollSurfaceFactor is GtkScrolledWindow's MAGIC_SCROLL_FACTOR for
// touchpad deltas in surface pixels.
const scrollSurfaceFactor = 2.5

// installScrollInput takes over scroll events on the thread's scroller.
// The controller runs in the capture phase and, being added after the
// scrolled window's own, before it.
func (cv *ConversationView) installScrollInput() {
	cv.scroller.SetKineticScrolling(false)
	ctl := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical | gtk.EventControllerScrollKinetic)
	ctl.SetPropagationPhase(gtk.PhaseCapture)
	ctl.ConnectScrollBegin(func() { cv.stopFling() })
	ctl.ConnectScroll(func(_, dy float64) bool {
		if cv.model.Len() == 0 {
			return false
		}
		step := cv.scrollStep(ctl.Unit())
		if ctl.Unit() == gdk.ScrollUnitWheel {
			// A notch: glide over its step rather than jump it.
			cv.startFling(dy*step*wheelStepFriction, wheelStepFriction)
		} else {
			cv.stopFling()
			cv.scrollBy(dy * step)
		}
		return true
	})
	ctl.ConnectDecelerate(func(_, vy float64) {
		if cv.model.Len() == 0 {
			return
		}
		cv.startFling(vy*cv.scrollStep(ctl.Unit()), scrollFriction)
	})
	cv.scroller.AddController(ctl)
}

// scrollStep is the value change per unit of dy, as GtkScrolledWindow
// computes it: a wheel notch moves page^(2/3), a touchpad pixel 2.5.
func (cv *ConversationView) scrollStep(unit gdk.ScrollUnit) float64 {
	if unit == gdk.ScrollUnitWheel {
		return math.Pow(cv.scroller.VAdjustment().PageSize(), 2.0/3.0)
	}
	return scrollSurfaceFactor
}

// scrollBy moves the thread by delta from where GTK has it now.
func (cv *ConversationView) scrollBy(delta float64) {
	adj := cv.scroller.VAdjustment()
	adj.SetValue(adj.Value() + delta)
}

// startFling scrolls on at velocity (value units per second), decaying
// by e^-friction per second, until it is spent or the thread ends.
func (cv *ConversationView) startFling(velocity, friction float64) {
	cv.flingGen++
	gen := cv.flingGen
	last := time.Now()
	gtk.BaseWidget(cv.scroller).AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		if gen != cv.flingGen {
			return false
		}
		now := time.Now()
		dt := now.Sub(last).Seconds()
		last = now
		if dt > 0.1 {
			dt = 0.1
		}
		adj := cv.scroller.VAdjustment()
		before := adj.Value()
		cv.scrollBy(velocity * dt)
		velocity *= math.Exp(-friction * dt)
		if math.Abs(velocity) < 2 || adj.Value() == before {
			return false
		}
		return true
	})
}

// stopFling ends a fling in progress.
func (cv *ConversationView) stopFling() { cv.flingGen++ }
