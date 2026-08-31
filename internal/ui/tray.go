package ui

import (
	_ "embed"
	"strconv"

	"fyne.io/systray"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

//go:embed tray_icon.png
var trayIconPNG []byte

// Tray is a StatusNotifierItem placed in the desktop system tray. Its menu
// raises the main window (Open chatot) or exits (Quit); both actions are
// dispatched onto the GTK main loop via glib.IdleAdd so callers may touch
// GTK state directly.
type Tray struct {
	end func()
}

// SetupTray registers the tray via fyne.io/systray's external-loop mode so it
// never owns the GTK main loop: RunWithExternalLoop returns a start func that
// runs the D-Bus plumbing on its own goroutine, and every menu click hops back
// onto GTK through glib.IdleAdd. It degrades to a no-op when there's no D-Bus
// session or StatusNotifierWatcher on the bus — systray logs and returns rather
// than crashing, so the app runs fine with no tray.
func SetupTray(showWindow, quit func()) *Tray {
	onReady := func() {
		systray.SetIcon(trayIconPNG)
		systray.SetTitle("chatot")
		systray.SetTooltip(trayTooltip(0))
		systray.SetOnTapped(func() { glib.IdleAdd(showWindow) })

		open := systray.AddMenuItem("Open chatot", "Show the chatot window")
		quitItem := systray.AddMenuItem("Quit", "Quit chatot")
		go trayMenuLoop(open.ClickedCh, quitItem.ClickedCh, showWindow, quit)
	}

	start, end := systray.RunWithExternalLoop(onReady, func() {})
	go start()
	return &Tray{end: end}
}

// SetUnread updates the tray tooltip to reflect the total unread count.
// systray's setters are goroutine-safe, so this needn't run on the GTK loop.
func (t *Tray) SetUnread(n int) { systray.SetTooltip(trayTooltip(n)) }

// Teardown removes the tray item. It guards against systray's Close on a
// nil D-Bus connection when the tray never registered (no session bus).
func (t *Tray) Teardown() {
	defer func() { _ = recover() }()
	t.end()
}

func trayMenuLoop(open, quit <-chan struct{}, showWindow, quitFn func()) {
	for {
		select {
		case _, ok := <-open:
			if !ok {
				return
			}
			glib.IdleAdd(showWindow)
		case _, ok := <-quit:
			if !ok {
				return
			}
			glib.IdleAdd(quitFn)
		}
	}
}

// trayTooltip is the pure hover-text formatter: the bare app name, plus the
// unread total when there is one.
func trayTooltip(unread int) string {
	if unread <= 0 {
		return "chatot"
	}
	return "chatot — " + strconv.Itoa(unread) + " unread"
}
