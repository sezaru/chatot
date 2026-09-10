package client

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	wastore "go.mau.fi/whatsmeow/store"
)

// A logout reported against the session that is still current must relink,
// whatever the store's Deleted flag says. whatsmeow dispatches
// events.LoggedOut from a goroutine and only then deletes the device, so a
// handler that waited for Deleted lost that race, skipped the relink, and
// left the pairing screen with nothing to scan.
func TestShouldRelinkIgnoresStoreDeleted(t *testing.T) {
	cur := &whatsmeow.Client{Store: &wastore.Device{}}
	if cur.Store.Deleted {
		t.Fatal("a fresh device should not read as deleted")
	}
	if !shouldRelink(cur, cur) {
		t.Error("a logout on the current session must relink even before whatsmeow deletes the device")
	}
	if shouldRelink(cur, nil) {
		t.Error("a logout that names no session must not relink: the guard is identity, so an unknown session cannot be matched")
	}
}

// The same logout is reported twice — once by our own Logout, once by
// whatsmeow's device-removed handler. Only the first may relink; the second
// would throw away the fresh session the first just started.
//
// This only holds because the event handler is bound to the client that
// raised the event (see newWAClient): whatsmeow tells a handler nothing about
// its client, so reading the current one instead would compare a replacement
// against itself and relink a second time.
func TestShouldRelinkSkipsAlreadyReplacedSession(t *testing.T) {
	old := &whatsmeow.Client{Store: &wastore.Device{}}
	cur := &whatsmeow.Client{Store: &wastore.Device{}}
	if shouldRelink(cur, old) {
		t.Error("a logout on a session that has already been replaced must not relink again")
	}
	if shouldRelink(nil, old) {
		t.Error("no client means nothing to relink")
	}
}

// Relinking a paired account would throw its session away and force a
// rescan, so it is refused — including when the socket is merely down,
// which is the state an account spends every reconnect in.
func TestRelinkRefusesALinkedAccount(t *testing.T) {
	f := NewFake()
	if err := f.Relink(); !errors.Is(err, ErrStillLinked) {
		t.Errorf("Relink() on a linked account = %v, want ErrStillLinked", err)
	}

	f.mu.Lock()
	f.loggedIn = false // paired, but between sockets
	f.mu.Unlock()
	if err := f.Relink(); !errors.Is(err, ErrStillLinked) {
		t.Errorf("Relink() on a paired account with no socket = %v, want ErrStillLinked", err)
	}
	if !f.Paired() {
		t.Error("a refused Relink must leave the session paired")
	}
}

// Relink is what the Relink dialog asks for, so a signed-out account must
// answer it with a code rather than leaving the card on "Waiting for a code…".
func TestRelinkOffersACodeAfterLogout(t *testing.T) {
	f := NewFake()
	codes := f.QRCodes()
	if err := f.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if f.Paired() {
		t.Error("a signed-out account must not report itself as paired")
	}
	if err := f.Relink(); err != nil {
		t.Fatalf("Relink after logout: %v", err)
	}
	select {
	case code := <-codes:
		if code == "" {
			t.Error("Relink produced an empty pairing code")
		}
	default:
		t.Error("Relink produced no pairing code")
	}
}

// Removing the only account can't drop the row (chatot always keeps one), so
// it resets the account instead. Signing out alone was not enough: an account
// that was already signed out had nothing to sign out of, so the click did
// nothing whatsoever.
func TestRemoveLastAccountResetsItToPairing(t *testing.T) {
	only := NewFake()
	m := NewAccountManager()
	m.AddAccount("a", "A", only)

	// Already signed out, which is exactly the state the bug report hit:
	// unlink first, then try to remove.
	if err := only.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	codes := only.QRCodes()

	if err := m.RemoveAccount("a"); err != nil {
		t.Fatalf("RemoveAccount on the only account: %v", err)
	}
	if m.Count() != 1 {
		t.Fatalf("Count() after removing the only account = %d, want 1", m.Count())
	}
	select {
	case <-codes:
	default:
		t.Error("removing the only account left it with no pairing code to scan")
	}
}

// The manager forwards Relink to the active account, so the window's own
// retry reaches the client that is actually on screen.
func TestManagerRelinkForwardsToActive(t *testing.T) {
	active, other := NewFake(), NewFake()
	m := NewAccountManager()
	m.AddAccount("a", "A", active)
	m.AddAccount("b", "B", other)
	if err := active.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	codes := active.QRCodes()
	if err := m.Relink(); err != nil {
		t.Fatalf("Relink: %v", err)
	}
	select {
	case <-codes:
	default:
		t.Error("AccountManager.Relink did not reach the active account")
	}
}

// A reaction is one tap, so the client records it and reports it before the
// send, and the thread re-renders on that event. Waiting for the server
// round-trip first is what made reacting feel like a request.
func TestFakeReactReportsItself(t *testing.T) {
	f := NewFake()
	chats, err := f.Chats(0)
	if err != nil || len(chats) == 0 {
		t.Fatalf("Chats: %v (%d)", err, len(chats))
	}
	jid := chats[0].JID
	msgs, err := f.Messages(jid, 0)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("Messages: %v (%d)", err, len(msgs))
	}
	events := f.Events()
	if err := f.React(context.Background(), jid, msgs[0].ID, "👍"); err != nil {
		t.Fatalf("React: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Kind != EventReaction || ev.Reaction == nil || ev.Reaction.ChatJID != jid {
			t.Errorf("React published %+v, want an EventReaction for %s", ev, jid)
		}
	default:
		t.Error("React published no event, so nothing would re-render the row")
	}
}
