package client

import (
	"context"
	"testing"
	"time"
)

// waitEvent reads ch until pred matches (returns true) or the timeout fires;
// events not matching pred (e.g. the synthetic reload) are skipped.
func waitEvent(t *testing.T, ch <-chan Event, pred func(Event) bool) Event {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case ev := <-ch:
			if pred(ev) {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for expected event")
		}
	}
}

func mustNoEvent(t *testing.T, ch <-chan Event, pred func(Event) bool) {
	t.Helper()
	deadline := time.After(150 * time.Millisecond)
	for {
		select {
		case ev := <-ch:
			if pred(ev) {
				t.Fatalf("received an event that should not have arrived: kind=%d", ev.Kind)
			}
		case <-deadline:
			return
		}
	}
}

const testChat = "1234567890@s.whatsapp.net"

func TestAccountManagerForwardsToActive(t *testing.T) {
	a, b := NewFake(), NewFake()
	m := NewAccountManager()
	m.AddAccount("a", "A", a)
	m.AddAccount("b", "B", b)

	if _, err := m.SendText(context.Background(), testChat, "hello-a", nil); err != nil {
		t.Fatal(err)
	}
	if got := lastText(t, a, testChat); got != "hello-a" {
		t.Fatalf("active account a missing the sent message, last text = %q", got)
	}
	if got := lastText(t, b, testChat); got == "hello-a" {
		t.Fatal("non-active account b received the message")
	}

	if err := m.SetActive("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SendText(context.Background(), testChat, "hello-b", nil); err != nil {
		t.Fatal(err)
	}
	if got := lastText(t, b, testChat); got != "hello-b" {
		t.Fatalf("after switch, account b missing the sent message, last text = %q", got)
	}
	if got := lastText(t, a, testChat); got == "hello-b" {
		t.Fatal("old-active account a received a message meant for b")
	}
}

func TestAccountManagerEventIsolationAndSwitch(t *testing.T) {
	a, b := NewFake(), NewFake()
	m := NewAccountManager()
	m.AddAccount("a", "A", a)
	m.AddAccount("b", "B", b)

	sub := m.Events()

	// An event on the active account (a) reaches the manager subscriber.
	a.PushEvent(Event{Kind: EventMessage, Message: &Message{ID: "from-a"}})
	got := waitEvent(t, sub, func(e Event) bool { return e.Kind == EventMessage })
	if got.Message == nil || got.Message.ID != "from-a" {
		t.Fatalf("expected event from active account a, got %+v", got.Message)
	}

	// An event on the non-active account (b) must NOT reach the subscriber.
	b.PushEvent(Event{Kind: EventMessage, Message: &Message{ID: "from-b"}})
	mustNoEvent(t, sub, func(e Event) bool {
		return e.Kind == EventMessage && e.Message != nil && e.Message.ID == "from-b"
	})

	// After switching to b, its events reach the subscriber and a's do not.
	if err := m.SetActive("b"); err != nil {
		t.Fatal(err)
	}
	b.PushEvent(Event{Kind: EventMessage, Message: &Message{ID: "from-b2"}})
	got = waitEvent(t, sub, func(e Event) bool {
		return e.Kind == EventMessage && e.Message != nil && e.Message.ID == "from-b2"
	})
	if got.Message.ID != "from-b2" {
		t.Fatalf("expected event from newly active account b, got %+v", got.Message)
	}

	a.PushEvent(Event{Kind: EventMessage, Message: &Message{ID: "from-a2"}})
	mustNoEvent(t, sub, func(e Event) bool {
		return e.Kind == EventMessage && e.Message != nil && e.Message.ID == "from-a2"
	})
}

func lastText(t *testing.T, f *Fake, jid string) string {
	t.Helper()
	msgs, err := f.Messages(jid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		return ""
	}
	return msgs[len(msgs)-1].Text
}

func TestAccountStatusLine(t *testing.T) {
	if got := accountStatusLine(true); got != "Connected" {
		t.Errorf("logged-in status = %q, want %q", got, "Connected")
	}
	if got := accountStatusLine(false); got != "Logged out · scan to relink" {
		t.Errorf("logged-out status = %q, want %q", got, "Logged out · scan to relink")
	}
}

func TestAccountsMetaStatus(t *testing.T) {
	m := NewAccountManager()
	m.AddAccount("a", "A", NewFake())
	metas := m.Accounts()
	if len(metas) != 1 {
		t.Fatalf("want 1 account meta, got %d", len(metas))
	}
	if metas[0].Status != "Connected" {
		t.Errorf("seeded Fake is logged in, status = %q, want %q", metas[0].Status, "Connected")
	}
}
