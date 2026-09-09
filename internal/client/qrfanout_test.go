package client

import "testing"

func TestQRFanoutReplaysCurrentCodeToLateSubscriber(t *testing.T) {
	var f qrFanout
	early := f.Subscribe()
	f.Publish("one")
	late := f.Subscribe()
	if got := <-early; got != "one" {
		t.Fatalf("early subscriber got %q, want one", got)
	}
	select {
	case got := <-late:
		if got != "one" {
			t.Fatalf("late subscriber got %q, want the current code", got)
		}
	default:
		t.Fatal("late subscriber was not handed the current code")
	}
}

func TestQRFanoutLatestCodeWins(t *testing.T) {
	var f qrFanout
	sub := f.Subscribe()
	f.Publish("stale")
	f.Publish("fresh")
	if got := <-sub; got != "fresh" {
		t.Fatalf("unread code = %q, want the newest", got)
	}
	select {
	case got := <-sub:
		t.Fatalf("a second code %q was queued; only the newest should remain", got)
	default:
	}
}

func TestQRFanoutClearForgetsExpiredCode(t *testing.T) {
	var f qrFanout
	f.Publish("expired")
	f.Clear()
	select {
	case got := <-f.Subscribe():
		t.Fatalf("subscriber after Clear got %q, want nothing", got)
	default:
	}
}
