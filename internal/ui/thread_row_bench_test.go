package ui

import (
	"fmt"
	"testing"
	"time"

	"chatot/internal/client"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// What one bind of the thread costs, without a compositor in the way.
//
// The scroll bench (CHATOT_SHOT=scrollbench) reports frames per second on a
// real fling, which is the number that matters but not one that can settle an
// argument: on this machine the same binary scores anywhere from 30 to 50 fps
// depending on the hour, so an 8% change is invisible under the drift. These
// benchmarks price the same work headless and repeatably, and they agree with
// each other to within a percent across runs.
//
// Rendering a row is cheap; measuring it afterwards is not. Whatever a change
// does to BenchmarkRowRenderAndMeasure is what it will do to a fling, because
// a fling is a few thousand of these and nothing else.

func benchSetup(b *testing.B) ([]client.Message, *threadRow, bubbleHooks, time.Time) {
	if !gtk.InitCheck() {
		b.Skip("no display")
	}
	// The row's cost is mostly CSS-driven (padding, min sizes, font), so the
	// app's own stylesheet has to be loaded or the numbers mean nothing.
	display := gdk.DisplayGetDefault()
	sheet := gtk.NewCSSProvider()
	sheet.LoadFromString(StyleCSS)
	gtk.StyleContextAddProviderForDisplay(display, sheet, uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
	b.Cleanup(func() { gtk.StyleContextRemoveProviderForDisplay(display, sheet) })

	now := time.Now()
	// Distinct bodies, so every iteration invalidates the row the way a
	// scroll does; one repeated message would measure GTK's cache instead.
	msgs := make([]client.Message, 32)
	for i := range msgs {
		msgs[i] = client.Message{
			ID:      fmt.Sprintf("m%d", i),
			Text:    fmt.Sprintf("message number %d, with enough words in it to wrap onto a second line sometimes", i),
			FromJID: "1@s.whatsapp.net",
			TS:      now.Unix(),
		}
	}
	return msgs, newThreadRow(), testHooks(), now
}

// measureRow does what GTK does to a row that has just been bound: a width
// pass, then a height pass at that width.
func measureRow(r *threadRow) {
	_, w, _, _ := r.wrapper.Measure(gtk.OrientationHorizontal, -1)
	r.wrapper.Measure(gtk.OrientationVertical, w)
}

// BenchmarkRowRenderAndMeasure is the one to watch: a whole bind, the way a
// fling does it thousands of times.
func BenchmarkRowRenderAndMeasure(b *testing.B) {
	msgs, r, h, now := benchSetup(b)
	for i := 0; i < b.N; i++ {
		m := msgs[i%len(msgs)]
		r.render(m, testVM(m, nil, "", now), h)
		measureRow(r)
	}
}

// BenchmarkRowRenderOnly is the Go and widget-building half alone. It is
// roughly an eighth of a bind: the other seven eighths are GTK measuring
// what was built, which is why shaving Go work off the bind path has never
// shown up on a fling.
func BenchmarkRowRenderOnly(b *testing.B) {
	msgs, r, h, now := benchSetup(b)
	for i := 0; i < b.N; i++ {
		m := msgs[i%len(msgs)]
		r.render(m, testVM(m, nil, "", now), h)
	}
}

// BenchmarkRowBodyOnly is the floor for a bind: nothing about the row
// changes but the words in it. The gap between this and
// BenchmarkRowRenderAndMeasure is everything else a bind does — the
// separators, the chrome classes, the footer, the reactions, the avatar and
// the hover pair — and it is small, which is the point.
func BenchmarkRowBodyOnly(b *testing.B) {
	msgs, r, h, now := benchSetup(b)
	m0 := msgs[0]
	r.render(m0, testVM(m0, nil, "", now), h)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.body.SetMarkup(messageMarkup(msgs[i%len(msgs)].Text, nil, false, "", ""))
		measureRow(r)
	}
}

// BenchmarkRowMeasureUnchanged is the floor: a row nothing touched. It should
// stay near zero — GTK caches a size request until something invalidates it,
// so a row that scrolls past without rebinding costs nothing at all.
func BenchmarkRowMeasureUnchanged(b *testing.B) {
	msgs, r, h, now := benchSetup(b)
	m0 := msgs[0]
	r.render(m0, testVM(m0, nil, "", now), h)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		measureRow(r)
	}
}

// BenchmarkBodyLabelAlone prices the wrapping body label on its own, so the
// rest of a row's measure can be told apart from the pango layout that no
// arrangement of widgets can avoid.
func BenchmarkBodyLabelAlone(b *testing.B) {
	msgs, _, _, _ := benchSetup(b)
	l := gtk.NewLabel("")
	l.AddCSSClass("chatot-bubble-text")
	l.SetWrap(true)
	l.SetMaxWidthChars(48)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.SetMarkup(messageMarkup(msgs[i%len(msgs)].Text, nil, false, "", ""))
		_, w, _, _ := l.Measure(gtk.OrientationHorizontal, -1)
		l.Measure(gtk.OrientationVertical, w)
	}
}
