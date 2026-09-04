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
	if got := accountStatusLine(true, true); got != "Connected" {
		t.Errorf("logged-in status = %q, want %q", got, "Connected")
	}
	if got := accountStatusLine(true, false); got != "Reconnecting…" {
		t.Errorf("paired-but-offline status = %q, want %q", got, "Reconnecting…")
	}
	if got := accountStatusLine(false, false); got != "Logged out · scan to relink" {
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

// running reports whether id's client is currently started (white-box: stop is
// set while running).
func (m *AccountManager) running(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.findLocked(id)
	return a != nil && a.stop != nil
}

func TestSetKeepInactiveConnected(t *testing.T) {
	m := NewAccountManager()
	m.AddAccount("a", "A", NewFake())
	m.AddAccount("b", "B", NewFake())
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !m.running("a") || !m.running("b") {
		t.Fatal("default keepInactive should start both accounts")
	}

	m.SetKeepInactiveConnected(false)
	if !m.running("a") {
		t.Error("active account a should stay running")
	}
	if m.running("b") {
		t.Error("non-active account b should be stopped when keep-connected is off")
	}

	// Switching active starts the newly-active and stops the previously-active.
	if err := m.SetActive("b"); err != nil {
		t.Fatal(err)
	}
	if !m.running("b") {
		t.Error("newly-active account b should be started")
	}
	if m.running("a") {
		t.Error("previously-active account a should be stopped")
	}

	m.SetKeepInactiveConnected(true)
	if !m.running("a") || !m.running("b") {
		t.Error("re-enabling keep-connected should start every account")
	}
}

func TestActiveNameAndCount(t *testing.T) {
	m := NewAccountManager()
	if m.ActiveName() != "" || m.Count() != 0 {
		t.Fatal("empty manager should report no active name and zero count")
	}
	m.AddAccount("a", "A", NewFake())
	m.AddAccount("b", "B", NewFake())
	if got := m.ActiveName(); got != "A" {
		t.Errorf("ActiveName = %q, want %q", got, "A")
	}
	if got := m.Count(); got != 2 {
		t.Errorf("Count = %d, want 2", got)
	}
}

func TestSetAccountProxyUnknown(t *testing.T) {
	m := NewAccountManager()
	m.AddAccount("a", "A", NewFake())
	if err := m.SetAccountProxy("missing", "socks5://x"); err == nil {
		t.Fatal("setting a proxy on an unknown account should error")
	}
	if err := m.SetAccountProxy("a", "socks5://x"); err != nil {
		t.Fatalf("setting a proxy on a known account should succeed, got %v", err)
	}
	if got := m.AccountProxy("a"); got != "socks5://x" {
		t.Errorf("AccountProxy = %q, want %q", got, "socks5://x")
	}
}

func TestAccountDisplayNameFallsBackToProfile(t *testing.T) {
	a := &Account{ID: "default", c: NewFake()}
	if got := a.displayName(0); got != "Sezar" {
		t.Fatalf("no label: got %q, want the profile name", got)
	}
	a.Name = "Work"
	if got := a.displayName(0); got != "Work" {
		t.Fatalf("label wins: got %q", got)
	}
	m := NewAccountManager()
	m.AddAccount("default", "", NewFake())
	if got := m.ActiveName(); got != "Sezar" {
		t.Fatalf("ActiveName = %q", got)
	}
	if metas := m.Accounts(); metas[0].Name != "Sezar" {
		t.Fatalf("Accounts()[0].Name = %q", metas[0].Name)
	}
}
