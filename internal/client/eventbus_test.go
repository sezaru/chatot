package client

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestEventBusFanOut is the exact bug F13 fixes: two subscribers must BOTH
// receive every published event, not compete for one shared channel.
func TestEventBusFanOut(t *testing.T) {
	b := newEventBus(nil)
	a := b.Subscribe()
	c := b.Subscribe()

	want := Event{Kind: EventConnection, Connection: &Connection{Connected: true}}
	b.Publish(want)

	for i, ch := range []<-chan Event{a, c} {
		select {
		case got := <-ch:
			if got.Kind != want.Kind || got.Connection == nil || !got.Connection.Connected {
				t.Errorf("subscriber %d: got %+v", i, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d never received the event", i)
		}
	}
}

// TestEventBusFullSubscriberDoesNotBlock proves a full/absent subscriber
// neither blocks the publisher nor starves a healthy subscriber.
func TestEventBusFullSubscriberDoesNotBlock(t *testing.T) {
	var drops int32
	b := newEventBus(func(string, ...interface{}) { atomic.AddInt32(&drops, 1) })

	stuck := b.Subscribe()   // never drained
	healthy := b.Subscribe() // drained below

	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBufferSize*3; i++ {
			b.Publish(Event{Kind: EventMessage})
		}
		close(done)
	}()

	// The healthy subscriber must keep receiving even while stuck is full.
	got := 0
	timeout := time.After(2 * time.Second)
	for got < subscriberBufferSize {
		select {
		case <-healthy:
			got++
		case <-timeout:
			t.Fatalf("healthy subscriber starved: got %d", got)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publisher blocked on a full subscriber")
	}

	_ = stuck
	if atomic.LoadInt32(&drops) == 0 {
		t.Error("expected drop-on-full warnings for the stuck subscriber")
	}
}
