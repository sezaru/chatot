package client

import (
	"context"

	"go.mau.fi/whatsmeow"
	wastore "go.mau.fi/whatsmeow/store"
)

// A logout, whether the phone removed this device or Logout was called
// here, leaves whatsmeow's device deleted and its client inert: nothing
// would ever produce a new QR code, and the linking screen sat empty until
// the app was restarted. relink builds a fresh device and client on the
// same store, then starts pairing again, so the QR appears in place.

// newWAClient makes the whatsmeow client for device with chatot's settings
// and event handler.
func (w *Whatsmeow) newWAClient(device *wastore.Device) *whatsmeow.Client {
	wa := whatsmeow.NewClient(device, w.log)
	// The first app-state sync after linking is a "full sync", and whatsmeow
	// swallows its events by default, which is exactly where the phone's
	// pins, mutes and labels arrive.
	wa.EmitAppStateEventsOnFullSync = true
	wa.AddEventHandler(w.handleRaw)
	return wa
}

// relink replaces a logged-out session with a fresh device and starts
// pairing again. A no-op while the current session is still linked.
func (w *Whatsmeow) relink() {
	w.relinkMu.Lock()
	defer w.relinkMu.Unlock()
	if w.offline || w.wa == nil || !w.wa.Store.Deleted {
		return
	}
	w.log.Infof("chatot/client: session logged out; starting a new pairing")
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
	if err := w.Start(ctx); err != nil {
		w.log.Errorf("chatot/client: restart pairing after logout: %v", err)
	}
}
