package ui

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"chatot/internal/client"
)

func TestMediaGridRows(t *testing.T) {
	now := time.Date(2026, time.November, 26, 12, 0, 0, 0, time.Local)
	today := now.Add(-time.Hour).Unix()
	yesterday := now.Add(-25 * time.Hour).Unix()
	var media []client.Message
	// Oldest first: seven from yesterday, then two from today.
	for range 7 {
		media = append(media, client.Message{TS: yesterday})
	}
	for range 2 {
		media = append(media, client.Message{TS: today})
	}
	got := mediaGridRows(media, now)
	want := []mediaGridRow{
		{day: "Today", items: []int{8, 7}},
		{day: "Yesterday", items: []int{6, 5, 4, 3, 2}},
		{items: []int{1, 0}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mediaGridRows = %+v, want %+v", got, want)
	}
	if rows := mediaGridRows(nil, now); len(rows) != 0 {
		t.Errorf("no media = %+v, want no rows", rows)
	}
}

// TestThumbQueueOrder checks the newest wanted request runs first and one
// whose tile scrolled away is dropped.
func TestThumbQueueOrder(t *testing.T) {
	var mu sync.Mutex
	var ran []string
	block := make(chan struct{})
	started := make(chan struct{})
	done := make(chan struct{}, 8)
	q := newThumbQueue(1, func(f func()) { f() })
	job := func(key string) thumbJob {
		return thumbJob{key: key, run: func() func() {
			if key == "first" {
				close(started)
				<-block
			}
			mu.Lock()
			ran = append(ran, key)
			mu.Unlock()
			return func() { done <- struct{}{} }
		}}
	}
	for _, k := range []string{"first", "a", "b", "c", "d"} {
		q.want(k)
	}
	// "first" occupies the only worker while the rest queue up.
	q.push(job("first"))
	<-started
	q.push(job("a"))
	q.push(job("b"))
	q.push(job("b")) // a key already waiting is not queued twice
	q.push(job("c"))
	q.push(job("d"))
	q.unwant("c")
	close(block)
	for range 4 {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("queue stalled")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"first", "d", "b", "a"}; !reflect.DeepEqual(ran, want) {
		t.Errorf("ran %v, want %v", ran, want)
	}
}

func TestThumbQueueClear(t *testing.T) {
	q := newThumbQueue(0, func(f func()) { f() }) // no workers: jobs only queue
	q.want("a")
	q.push(thumbJob{key: "a", run: func() func() { return nil }})
	q.clear()
	if _, ok := q.next(); ok {
		t.Error("a job survived clear")
	}
	// A key cleared can be queued again.
	q.push(thumbJob{key: "a", run: func() func() { return nil }})
	if j, ok := q.next(); !ok || j.key != "a" {
		t.Errorf("next after re-push = %v, %v", j.key, ok)
	}
}
