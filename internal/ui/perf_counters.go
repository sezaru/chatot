package ui

import (
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"
)

// The thread's scroll cost is spread over a handful of main-loop paths:
// binding a row, building its bubble, parsing its markup, reading a quote
// the store still holds, and splicing an older page onto the front of the
// model. A frame-gap number alone cannot say which of those a slow frame
// went to, so CHATOT_PERF=1 tallies them and ScrollBench prints the tally
// next to its gaps. Off by default: perfStart hands back a shared no-op
// and nothing is measured or locked.

var perfOn = os.Getenv("CHATOT_PERF") != "" && os.Getenv("CHATOT_PERF") != "0"

var (
	perfMu    sync.Mutex
	perfSpans = map[string][]time.Duration{}
	perfNoop  = func() {}
	perfOrder []string // first-seen order, so the report reads top-down
)

// perfStart begins a span; the returned func ends it. Use as
// `defer perfStart("name")()`.
func perfStart(name string) func() {
	if !perfOn {
		return perfNoop
	}
	t0 := time.Now()
	return func() { perfRecord(name, time.Since(t0)) }
}

func perfRecord(name string, d time.Duration) {
	perfMu.Lock()
	defer perfMu.Unlock()
	if _, seen := perfSpans[name]; !seen {
		perfOrder = append(perfOrder, name)
	}
	perfSpans[name] = append(perfSpans[name], d)
}

// perfReset drops every span, so a bench measures only its own run.
func perfReset() {
	if !perfOn {
		return
	}
	perfMu.Lock()
	defer perfMu.Unlock()
	perfSpans = map[string][]time.Duration{}
	perfOrder = nil
}

// perfReport logs one line per instrumented path: how often it ran, what
// it cost in total, and its slow tail. Total is the number to compare
// against the run's wall clock; a path holding tens of percent of it is
// what the frames were waiting on.
func perfReport(label string) {
	if !perfOn {
		return
	}
	perfMu.Lock()
	defer perfMu.Unlock()
	if len(perfOrder) == 0 {
		log.Printf("perf %s: nothing instrumented ran", label)
		return
	}
	for _, name := range perfOrder {
		log.Printf("perf %s: %s", label, perfLine(name, perfSpans[name]))
	}
}

func perfLine(name string, spans []time.Duration) string {
	if len(spans) == 0 {
		return fmt.Sprintf("%-16s 0 calls", name)
	}
	sorted := append([]time.Duration(nil), spans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total time.Duration
	for _, d := range spans {
		total += d
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	pct := func(p float64) time.Duration { return sorted[int(float64(len(sorted)-1)*p)] }
	return fmt.Sprintf("%-16s %5d calls, total %7.1f ms, mean %5.2f, p50 %5.2f, p90 %5.2f, max %6.2f",
		name, len(spans), ms(total), ms(total)/float64(len(spans)), ms(pct(.5)), ms(pct(.9)), ms(sorted[len(sorted)-1]))
}
