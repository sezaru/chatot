package client

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFlightRunsOneCallPerKey(t *testing.T) {
	var f flight
	var runs atomic.Int32
	release := make(chan struct{})
	fn := func() (string, error) {
		runs.Add(1)
		<-release
		return "/media/a.zip", nil
	}

	var wg sync.WaitGroup
	results := make([]string, 5)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = f.do(context.Background(), "a", fn)
		}()
	}
	// Let every caller reach the flight before the call returns.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if n := runs.Load(); n != 1 {
		t.Fatalf("fn ran %d times, want 1", n)
	}
	for i, r := range results {
		if r != "/media/a.zip" {
			t.Errorf("caller %d got %q", i, r)
		}
	}
	// Once landed, the key is free again.
	if _, err := f.do(context.Background(), "a", func() (string, error) { return "", errors.New("again") }); err == nil {
		t.Fatal("a later call should run fn afresh")
	}
}

func TestFlightWaiterGivesUpWithItsContext(t *testing.T) {
	var f flight
	release := make(chan struct{})
	defer close(release)
	go f.do(context.Background(), "a", func() (string, error) { <-release; return "x", nil })
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := f.do(ctx, "a", func() (string, error) { return "y", nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the waiter's deadline", err)
	}
}
