// Command chatot is a standalone GTK4/libadwaita WhatsApp client.
package main

import (
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
	"chatot/internal/ui"
)

const appID = "com.sezdm.chatot"

func main() {
	app := adw.NewApplication(appID, gio.ApplicationFlagsNone)

	// Fake stands in for the whatsmeow-backed Client until F2; the UI is
	// built against the Client interface either way.
	c := client.NewFake()

	app.ConnectActivate(func() { activate(app, c) })

	os.Exit(app.Run(os.Args))
}

func activate(app *adw.Application, _ client.Client) {
	loadCSS()

	sidebar := adw.NewNavigationPage(placeholderLabel("Chats", "chatot-sidebar"), "Chats")
	content := adw.NewNavigationPage(placeholderLabel("Select a chat", ""), "Conversation")

	split := adw.NewNavigationSplitView()
	split.SetSidebar(sidebar)
	split.SetContent(content)

	win := adw.NewApplicationWindow(&app.Application)
	win.SetTitle("chatot")
	win.SetDefaultSize(1000, 700)
	win.SetContent(split)
	win.Present()
}

func placeholderLabel(text, cssClass string) *gtk.Label {
	label := gtk.NewLabel(text)
	label.AddCSSClass("chatot-placeholder")
	if cssClass != "" {
		label.AddCSSClass(cssClass)
	}
	return label
}

func loadCSS() {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(ui.StyleCSS)
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
