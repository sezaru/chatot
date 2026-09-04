package ui

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// trayWatcherName is the well-known bus name a desktop's tray host owns.
const trayWatcherName = "org.kde.StatusNotifierWatcher"

// TrayAvailable reports whether a StatusNotifierWatcher is on the session
// bus right now, so a tray item would actually be shown somewhere. systray
// itself only logs when registration fails, and Close to tray must never
// hide the window with nothing to bring it back.
func TrayAvailable() bool {
	conn, err := gio.BusGetSync(context.Background(), gio.BusTypeSession)
	if err != nil {
		return false
	}
	reply, err := conn.CallSync(context.Background(),
		"org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "NameHasOwner",
		glib.NewVariantTuple([]*glib.Variant{glib.NewVariantString(trayWatcherName)}),
		glib.NewVariantType("(b)"), gio.DBusCallFlagsNone, 1000)
	if err != nil || reply.NChildren() == 0 {
		return false
	}
	return reply.ChildValue(0).Boolean()
}
