package transcribe

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// blockingRun is a fake engine that records the order jobs reach it and
// holds each one until released, so a test can pile jobs up behind it.
type blockingRun struct {
	mu      sync.Mutex
	order   []string
	release chan struct{}
}

func (b *blockingRun) run(_ context.Context, _, path string) (string, error) {
	b.mu.Lock()
	b.order = append(b.order, path)
	b.mu.Unlock()
	<-b.release
	if path == "bad" {
		return "", errors.New("boom")
	}
	return "text:" + path, nil
}

func (b *blockingRun) got() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.order...)
}

func TestQueueRunsOneAtATimeRequestedFirstThenNewestAuto(t *testing.T) {
	eng := &blockingRun{release: make(chan struct{})}
	q := NewQueue(eng.run)
	done := make(chan string, 10)
	add := func(key string, requested bool) {
		q.Enqueue(Job{Key: key, Path: key, Requested: requested, OnDone: func(text string, err error) { done <- key }})
	}
	add("first", false) // starts at once and blocks the engine
	time.Sleep(20 * time.Millisecond)
	add("a1", false)
	add("a2", false)
	add("r1", true)
	add("a3", false)
	add("r2", true)
	if got := q.Pending(); got != 5 {
		t.Fatalf("Pending = %d, want 5", got)
	}
	if got := eng.got(); len(got) != 1 {
		t.Fatalf("engine took %v before the first job was released, want one at a time", got)
	}
	for i := 0; i < 6; i++ {
		eng.release <- struct{}{}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("job %d never finished", i)
		}
	}
	want := []string{"first", "r1", "r2", "a3", "a2", "a1"}
	if got := eng.got(); len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("order = %v, want %v", got, want)
			}
		}
	}
	if q.Pending() != 0 {
		t.Fatalf("Pending after drain = %d", q.Pending())
	}
}

func TestQueueDedupesPromotesAndCapsAuto(t *testing.T) {
	eng := &blockingRun{release: make(chan struct{})}
	q := NewQueue(eng.run)
	q.MaxAuto = 2
	q.Enqueue(Job{Key: "busy", Path: "busy"})
	time.Sleep(20 * time.Millisecond)
	if !q.Enqueue(Job{Key: "busy", Path: "busy"}) {
		t.Fatal("re-enqueue of the running key reported dropped")
	}
	if !q.Enqueue(Job{Key: "a", Path: "a"}) || !q.Enqueue(Job{Key: "b", Path: "b"}) {
		t.Fatal("automatic jobs within the cap were dropped")
	}
	if q.Enqueue(Job{Key: "c", Path: "c"}) {
		t.Fatal("automatic job past the cap was taken")
	}
	if !q.Enqueue(Job{Key: "a", Path: "a"}) || q.Pending() != 2 {
		t.Fatalf("duplicate key changed the queue: pending %d", q.Pending())
	}
	// A click on a waiting automatic job moves it ahead.
	if !q.Enqueue(Job{Key: "a", Path: "a", Requested: true}) || q.Pending() != 2 {
		t.Fatalf("promotion changed the count: pending %d", q.Pending())
	}
	if !q.Enqueue(Job{Key: "d", Path: "d", Requested: true}) {
		t.Fatal("requested job dropped")
	}
	for i := 0; i < 4; i++ {
		eng.release <- struct{}{}
	}
	deadline := time.After(2 * time.Second)
	for q.Pending() > 0 || len(eng.got()) < 4 {
		select {
		case <-deadline:
			t.Fatalf("queue never drained: %v", eng.got())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	want := []string{"busy", "a", "d", "b"}
	got := eng.got()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestQueueReportsStartAndFailure(t *testing.T) {
	eng := &blockingRun{release: make(chan struct{}, 1)}
	eng.release <- struct{}{}
	q := NewQueue(eng.run)
	started := make(chan struct{}, 1)
	result := make(chan error, 1)
	q.Enqueue(Job{Key: "bad", Path: "bad", OnStart: func() { started <- struct{}{} }, OnDone: func(_ string, err error) { result <- err }})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("OnStart never ran")
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("failure not reported")
		}
	case <-time.After(time.Second):
		t.Fatal("OnDone never ran")
	}
}

// Cancelling the job the engine is on stops that run and reports it as
// cancelled, whatever the engine did on its way out: a whisper.cpp that was
// killed exits with a signal, which is not the caller's business.
func TestQueueCancelStopsTheRunningJob(t *testing.T) {
	started := make(chan struct{})
	q := NewQueue(func(ctx context.Context, _, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", errors.New("signal: killed")
	})
	done := make(chan error, 1)
	q.Enqueue(Job{Key: "one", Path: "one", Requested: true, OnDone: func(_ string, err error) { done <- err }})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the job never reached the engine")
	}
	if !q.Cancel("one") {
		t.Fatal("Cancel did not find the running job")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("OnDone err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the cancelled job never finished")
	}
}

// A job that is only waiting is dropped where it stands: it never started,
// so there is no result to report, and the key is free for a fresh ask.
func TestQueueCancelDropsAWaitingJob(t *testing.T) {
	eng := &blockingRun{release: make(chan struct{})}
	q := NewQueue(eng.run)
	q.Enqueue(Job{Key: "running", Path: "running", Requested: true})
	time.Sleep(20 * time.Millisecond)
	waiting := make(chan struct{}, 1)
	q.Enqueue(Job{Key: "waiting", Path: "waiting", Requested: true, OnDone: func(string, error) { waiting <- struct{}{} }})
	if !q.Cancel("waiting") {
		t.Fatal("Cancel did not find the waiting job")
	}
	if got := q.Pending(); got != 0 {
		t.Errorf("Pending = %d after cancelling the only waiting job, want 0", got)
	}
	if q.Cancel("waiting") {
		t.Error("Cancel found the job twice")
	}
	eng.release <- struct{}{}
	time.Sleep(50 * time.Millisecond)
	if got := eng.got(); len(got) != 1 || got[0] != "running" {
		t.Errorf("engine ran %v, want the cancelled job never to reach it", got)
	}
	select {
	case <-waiting:
		t.Error("a job that never started reported a result")
	default:
	}
}
