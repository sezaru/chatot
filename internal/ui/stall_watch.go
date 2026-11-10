package ui

import (
	"bytes"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

const (
	// stallPing is how often the watcher asks the main loop to answer.
	stallPing = 250 * time.Millisecond
	// stallThreshold is how long the main loop may go without answering
	// before the window counts as frozen.
	stallThreshold = time.Second
	// stallStackMax caps what one report writes to the log.
	stallStackMax = 64 << 10
)

// WatchMainLoop logs when the GTK main loop stops answering for longer than
// stallThreshold: once while it is stuck, with the stack of the goroutine
// running it (what it was doing), and once when it answers again, with how
// long the freeze lasted. A freeze that cannot be reproduced at will then
// says where it was.
func WatchMainLoop() {
	stallWatch.Do(watchMainLoop)
}

var stallWatch sync.Once

func watchMainLoop() {
	var answered atomic.Int64
	var asked atomic.Bool
	answered.Store(time.Now().UnixNano())
	go func() {
		var frozenAt time.Time
		for range time.Tick(stallPing) {
			if !asked.Swap(true) {
				glib.IdleAddPriority(glib.PriorityDefault, func() {
					answered.Store(time.Now().UnixNano())
					asked.Store(false)
				})
			}
			last := time.Unix(0, answered.Load())
			switch quiet := time.Since(last); {
			case quiet >= stallThreshold && frozenAt.IsZero():
				frozenAt = last
				log.Printf("chatot: main loop stuck for %s; it is at:\n%s", quiet.Round(time.Millisecond), mainLoopStack())
			case quiet < stallThreshold && !frozenAt.IsZero():
				log.Printf("chatot: main loop answered again after %s", last.Sub(frozenAt).Round(time.Millisecond))
				frozenAt = time.Time{}
			}
		}
	}()
}

// mainLoopStack is the stack of the goroutine running the GTK main loop:
// the one whose stack goes down to the application's Run. All goroutines
// when none is found.
func mainLoopStack() []byte {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	if g := goroutineRunning(buf, []byte("Application).Run(")); g != nil {
		return g
	}
	return buf[:min(len(buf), stallStackMax)]
}

// goroutineRunning picks, out of an all-goroutines dump, the first
// goroutine whose stack contains frame; nil if none does.
func goroutineRunning(dump, frame []byte) []byte {
	for _, g := range bytes.Split(dump, []byte("\n\n")) {
		if bytes.Contains(g, frame) {
			return g[:min(len(g), stallStackMax)]
		}
	}
	return nil
}
