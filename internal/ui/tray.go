package ui

import (
	_ "embed"
	"strconv"
	"sync/atomic"

	"fyne.io/systray"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// trayIconLightPNG / trayIconDarkPNG are the tray mark (the chatot alone,
// no green tile; assets/chatot-icon-tray-{light,dark}.svg rasterised at
// 128px on a square transparent canvas), one body colour per theme: grey
// for a light panel, white for a dark one. The ⋮ menu's About row draws
// the same pair.
//
//go:embed assets/chatot-icon-tray-light.png
var trayIconLightPNG []byte

//go:embed assets/chatot-icon-tray-dark.png
var trayIconDarkPNG []byte

// trayIconPNG picks the tray mark for the current theme.
func trayIconPNG(dark bool) []byte {
	if dark {
		return trayIconDarkPNG
	}
	return trayIconLightPNG
}

// Tray is a StatusNotifierItem placed in the desktop system tray. Its menu
// raises the main window (Open chatot) or exits (Quit); both actions are
// dispatched onto the GTK main loop via glib.IdleAdd so callers may touch
// GTK state directly.
type Tray struct {
	end   func()
	ready atomic.Bool
}

// SetupTray registers the tray via fyne.io/systray's external-loop mode so it
// never owns the GTK main loop: RunWithExternalLoop returns a start func that
// runs the D-Bus plumbing on its own goroutine, and every menu click hops back
// onto GTK through glib.IdleAdd. It degrades to a no-op when there's no D-Bus
// session or StatusNotifierWatcher on the bus — systray logs and returns rather
// than crashing, so the app runs fine with no tray. Call on the GTK main loop:
// the icon follows the style manager's theme from here on.
func SetupTray(showWindow, quit func()) *Tray {
	t := &Tray{}
	sm := adw.StyleManagerGetDefault()
	dark := sm.Dark()
	onReady := func() {
		systray.SetIcon(trayIconPNG(dark))
		systray.SetTitle("chatot")
		systray.SetTooltip(trayTooltip(0))
		systray.SetOnTapped(func() { glib.IdleAdd(showWindow) })

		open := systray.AddMenuItem("Open chatot", "Show the chatot window")
		quitItem := systray.AddMenuItem("Quit", "Quit chatot")
		t.ready.Store(true)
		go trayMenuLoop(open.ClickedCh, quitItem.ClickedCh, showWindow, quit)
	}
	// The theme can flip after the tray is up (system preference or the
	// Theme setting); the notify runs on the GTK loop and systray's setters
	// are goroutine-safe.
	sm.NotifyProperty("dark", func() {
		if t.ready.Load() {
			systray.SetIcon(trayIconPNG(sm.Dark()))
		}
	})

	start, end := systray.RunWithExternalLoop(onReady, func() {})
	go start()
	t.end = end
	return t
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
