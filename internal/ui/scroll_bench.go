package ui

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ScrollBench is the CHATOT_SHOT=scrollbench dev hook: it drives adj down
// and back up at speed px/s from w's frame clock for dur, then logs how the
// frames kept up (count, effective fps, the slow tail). The wall-clock gap
// between ticks is what is measured, so a frame the renderer or layout
// held up shows as a long gap.
func ScrollBench(w gtk.Widgetter, adj *gtk.Adjustment, name string, dur time.Duration, speed float64) {
	var (
		start, last time.Time
		gaps        []float64 // ms
		dir         = 1.0
		set         = -1.0
		jumps       int
		maxJump     float64
	)
	gtk.BaseWidget(w).AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		now := time.Now()
		if start.IsZero() {
			start, last = now, now
			return true
		}
		// The value moved since the bench set it: something else (a
		// prepend re-anchoring, a stick-to-bottom) scrolled the view.
		if d := adj.Value() - set; set >= 0 && (d > 1 || d < -1) {
			jumps++
			if d < 0 {
				d = -d
			}
			if d > maxJump {
				maxJump = d
			}
		}
		dt := now.Sub(last).Seconds() * 1000
		last = now
		gaps = append(gaps, dt)
		v := adj.Value() + dir*speed*dt/1000
		if max := adj.Upper() - adj.PageSize(); v >= max {
			v, dir = max, -1
		}
		if v <= adj.Lower() {
			v, dir = adj.Lower(), 1
		}
		adj.SetValue(v)
		set = adj.Value()
		if now.Sub(start) < dur {
			return true
		}
		log.Printf("scrollbench %s: %d external jumps, largest %.0f px", name, jumps, maxJump)
		reportScrollBench(name, gaps, now.Sub(start))
		return false
	})
}

func reportScrollBench(name string, gaps []float64, total time.Duration) {
	if len(gaps) == 0 {
		log.Printf("scrollbench %s: no frames", name)
		return
	}
	sorted := append([]float64(nil), gaps...)
	sort.Float64s(sorted)
	pct := func(p float64) float64 { return sorted[int(float64(len(sorted)-1)*p)] }
	slow := 0
	for _, g := range gaps {
		if g > 20 {
			slow++
		}
	}
	log.Printf("scrollbench %s: %d frames in %.1fs = %.1f fps; gap ms p50 %.1f p90 %.1f p99 %.1f max %.1f; %d frames over 20ms",
		name, len(gaps), total.Seconds(), float64(len(gaps))/total.Seconds(), pct(.5), pct(.9), pct(.99), sorted[len(sorted)-1], slow)
}

// Scroller is the thread's scrolled window (dev hooks).
func (cv *ConversationView) Scroller() *gtk.ScrolledWindow { return cv.scroller }

// ListScroller is the chat rows' scrolled window (dev hooks).
func (cl *ChatList) ListScroller() *gtk.ScrolledWindow { return cl.listScroller }

// RefreshBench rebuilds the chat list n times and logs the cost of one
// rebuild (dev hook CHATOT_SHOT=refreshbench), which is what every event
// that reaches ChatList.queueRefresh charges the main loop.
func (cl *ChatList) RefreshBench(n int) {
	var total, worst time.Duration
	for i := 0; i < n; i++ {
		t := time.Now()
		cl.refresh()
		d := time.Since(t)
		total += d
		if d > worst {
			worst = d
		}
	}
	log.Printf("refreshbench: %d rows; %d rebuilds; mean %.1f ms, worst %.1f ms; rows in order: %v",
		len(cl.rowJIDs), n, float64(total.Microseconds())/1000/float64(n), float64(worst.Microseconds())/1000, cl.rowsInOrder())
}

// RefreshBreakdown times the pieces of one refresh on the current store
// (dev hook CHATOT_SHOT=refreshbreakdown).
func (cl *ChatList) RefreshBreakdown() {
	t := time.Now()
	chats, _ := cl.c.Chats(0)
	dChats := time.Since(t)
	t = time.Now()
	for _, c := range chats {
		cl.c.LabelsForChat(c.JID)
	}
	dLabels := time.Since(t)
	t = time.Now()
	cl.updateChipRow(chats)
	dChip := time.Since(t)
	t = time.Now()
	cl.refreshChats(chats)
	dRows := time.Since(t)
	t = time.Now()
	cl.updateTabBadges(chats)
	dBadges := time.Since(t)
	t = time.Now()
	cl.loadStatusFeed()
	dStatus := time.Since(t)
	log.Printf("refreshbreakdown: %d chats; Chats %v; LabelsForChat×N %v; updateChipRow %v; refreshChats %v; updateTabBadges %v (loadStatusFeed %v)",
		len(chats), dChats, dLabels, dChip, dRows, dBadges, dStatus)
}

// ReconcileCheck (dev hook CHATOT_SHOT=reconcilecheck, fake account) walks
// the row reconciliation through a pin, an archive, an unarchive and a
// mark-unread, checking after each that the ListBox matches rowJIDs.
func (cl *ChatList) ReconcileCheck() {
	if len(cl.rowJIDs) < 12 {
		log.Printf("reconcilecheck: need 12+ rows, have %d", len(cl.rowJIDs))
		return
	}
	pin, arch, unread := cl.rowJIDs[10], cl.rowJIDs[3], cl.rowJIDs[5]
	ctx := context.Background()
	index := func(jid string) int {
		for i, j := range cl.rowJIDs {
			if j == jid {
				return i
			}
		}
		return -1
	}
	fails := 0
	check := func(step string, ok bool) {
		if !ok || !cl.rowsInOrder() {
			fails++
			log.Printf("reconcilecheck: FAIL at %s (cond %v, order %v)", step, ok, cl.rowsInOrder())
		}
	}
	steps := []struct {
		act   func()
		check func()
	}{
		{func() { cl.c.PinChat(ctx, pin, true) }, func() { check("pin", index(pin) < 10 && cl.rows[pin].vm.Pinned) }},
		{func() { cl.c.ArchiveChat(ctx, arch, true) }, func() { check("archive", index(arch) < 0 && cl.rows[arch] == nil) }},
		{func() { cl.c.ArchiveChat(ctx, arch, false) }, func() { check("unarchive", index(arch) >= 0) }},
		{func() { cl.c.MarkChatUnread(ctx, unread, true) }, func() { check("unread", cl.rows[unread] != nil && cl.rows[unread].vm.ShowUnread) }},
		{func() { cl.c.PinChat(ctx, pin, false) }, func() { check("unpin", index(pin) > 0) }},
	}
	i := 0
	var tick func() bool
	tick = func() bool {
		if i > 0 {
			steps[i-1].check()
		}
		if i == len(steps) {
			log.Printf("reconcilecheck: done, %d failures, %d rows", fails, len(cl.rowJIDs))
			return false
		}
		steps[i].act()
		i++
		glib.TimeoutAdd(500, tick)
		return false
	}
	glib.TimeoutAdd(500, tick)
}

// AnchorCheck (dev hook CHATOT_SHOT=anchorcheck) records where the first
// bubble under the viewport's top edge sits on screen, loads an older page
// into the open thread, and logs how far that bubble moved once the page
// has streamed in. Zero is the goal.
func (cv *ConversationView) AnchorCheck() {
	adj := cv.scroller.VAdjustment()
	adj.SetValue(600)
	glib.TimeoutAdd(300, func() bool {
		ref := -1
		for i, row := range cv.rows {
			if y, h, ok := cv.rowBounds(row); ok && y+h > adj.Value() {
				ref = i
				break
			}
		}
		if ref < 0 {
			log.Printf("anchorcheck: no reference row")
			return false
		}
		refMsg := cv.msgs[ref].ID
		b0, _ := cv.rows[ref].ComputeBounds(cv.scroller)
		v0 := adj.Value()
		cv.loadOlder()
		// Sample the row's drawn position every frame while the page lands:
		// a tick runs before layout, so it sees what the last frame showed.
		var seen []float64
		start := time.Now()
		gtk.BaseWidget(cv.scroller).AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
			if pos := cv.positionOf(refMsg); pos >= 0 {
				if b, ok := cv.rows[pos].ComputeBounds(cv.scroller); ok {
					y := float64(b.Y())
					if len(seen) == 0 || seen[len(seen)-1] != y {
						seen = append(seen, y)
					}
				}
			}
			return time.Since(start) < 2*time.Second
		})
		glib.TimeoutAdd(2500, func() bool {
			log.Printf("anchorcheck: drawn y per frame: %v", seen)
			pos := cv.positionOf(refMsg)
			if pos < 0 {
				log.Printf("anchorcheck: reference row lost")
				return false
			}
			b1, _ := cv.rows[pos].ComputeBounds(cv.scroller)
			log.Printf("anchorcheck: ref row %d→%d screen y %.0f→%.0f (moved %.0f px); value %.0f→%.0f upper %.0f",
				ref, pos, b0.Y(), b1.Y(), b1.Y()-b0.Y(), v0, adj.Value(), adj.Upper())
			return false
		})
		return false
	})
}
