package client

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow"
	wastore "go.mau.fi/whatsmeow/store"
)

// A logout, whether the phone removed this device or Logout was called
// here, leaves whatsmeow's device deleted and its client inert: nothing
// would ever produce a new QR code, and the linking screen sat empty until
// the app was restarted. relink builds a fresh device and client on the
// same store, then starts pairing again, so the QR appears in place.

// ErrStillLinked is returned by Relink for an account that is still signed
// in: whatsmeow only pairs a fresh session, so the caller has to sign out
// first rather than expect a code.
var ErrStillLinked = errors.New("chatot/client: this account is still linked")

// errAccountStopped means the account was removed, or switched away from with
// keep-connected off, so there is no window left to show a code in. Routine
// rather than a failure, and named so relink can log it as such.
var errAccountStopped = errors.New("this account is stopped, so it has nowhere to show a code")

// newWAClient makes the whatsmeow client for device with chatot's settings
// and event handler.
func (w *Whatsmeow) newWAClient(device *wastore.Device) *whatsmeow.Client {
	wa := whatsmeow.NewClient(device, w.log)
	// The first app-state sync after linking is a "full sync", and whatsmeow
	// swallows its events by default, which is exactly where the phone's
	// pins, mutes and labels arrive.
	wa.EmitAppStateEventsOnFullSync = true
	// The handler closes over wa so it knows which session raised the event.
	// whatsmeow tells a handler nothing about its client, and it dispatches
	// events.LoggedOut from a goroutine, so reading w.wa inside the handler
	// reports whichever session is current by the time it runs — which after
	// a relink is the replacement, not the one that logged out.
	wa.AddEventHandler(func(evt interface{}) { w.handleRaw(wa, evt) })
	return wa
}

// relink retires the logged-out session on old and starts pairing again.
//
// old is the client whose logout the caller saw. Identifying the session by
// pointer is what makes this safe to call from both reporters of the same
// logout (our own Logout, and whatsmeow's device-removed handler): the
// second one finds w.wa already moved on and returns.
//
// It deliberately does not gate on old.Store.Deleted. whatsmeow dispatches
// events.LoggedOut from its own goroutine *before* it finishes deleting the
// device, so that flag is usually still false here — gating on it lost the
// race, skipped the relink, and left the pairing screen with no code at all
// until the app was restarted.
func (w *Whatsmeow) relink(old *whatsmeow.Client) {
	if w.offline {
		return
	}
	w.relinkMu.Lock()
	defer w.relinkMu.Unlock()
	if !shouldRelink(w.wa, old) {
		return
	}
	w.log.Infof("chatot/client: session logged out; starting a new pairing")
	switch err := w.freshSessionLocked(); {
	case err == nil:
	case errors.Is(err, errAccountStopped):
		// Routine: the account was removed or switched away with
		// keep-connected off between the logout and this call.
		w.log.Infof("chatot/client: %v", err)
	default:
		w.log.Errorf("chatot/client: %v", err)
	}
}

// shouldRelink reports whether a logout reported against old still needs a
// relink, given that cur is the client the session holds now.
//
// The rule is deliberately about session identity and nothing else. Gating
// on cur.Store.Deleted instead is the bug this replaces: whatsmeow deletes
// the device *after* dispatching events.LoggedOut, so the flag is still
// false when the handler runs and the relink was skipped every time the
// phone (or a stream error) reported the logout.
func shouldRelink(cur, old *whatsmeow.Client) bool {
	if cur == nil || old == nil {
		return false
	}
	// A second report of the same logout finds the session already replaced.
	return cur == old
}

// Relink starts a fresh pairing round on demand, which is what the UI's
// "Relink" offers. Unlike the internal relink above it does not need a
// logout to have just happened, so it also recovers an account whose codes
// expired while its dialog was closed, or one whose automatic relink never
// got off the ground. A round that is already handing out codes is left
// alone; a still-linked account is refused with ErrStillLinked.
func (w *Whatsmeow) Relink() error {
	if w.offline {
		return fmt.Errorf("chatot/client: offline mode never pairs")
	}
	w.relinkMu.Lock()
	defer w.relinkMu.Unlock()
	if w.wa == nil {
		return fmt.Errorf("chatot/client: no session to relink")
	}
	// A device that still has an ID is linked, whether or not its socket is
	// up right now. Relinking it would throw away a working session over a
	// dropped connection and force a rescan, so the test is the stored
	// identity and never IsLoggedIn.
	if w.wa.Store.ID != nil {
		return ErrStillLinked
	}
	if w.pairing.Load() {
		// A round is already running; its next code lands on QRCodes(), and
		// a subscriber joining now is handed the current one.
		return nil
	}
	return w.freshSessionLocked()
}

// freshSessionLocked swaps in a new device and client and opens a pairing
// round on it. The caller holds relinkMu.
func (w *Whatsmeow) freshSessionLocked() error {
	w.wa.Disconnect()
	device := w.container.NewDevice()
	w.device = device
	w.wa = w.newWAClient(device)

	w.presenceMu.Lock()
	w.presenceSubscribed = nil
	w.presenceMu.Unlock()
	w.avatarMu.Lock()
	w.avatarMemo = nil
	w.avatarMu.Unlock()

	ctx := w.startCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		// The account was stopped (removed, or switched away with
		// keep-connected off): nobody is left to scan a code.
		return errAccountStopped
	}
	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("restart pairing after logout: %w", err)
	}
	return nil
}
