// Package ui holds the gotk4/libadwaita widgets (chat list, conversation,
// composer, ...) built up feature by feature.
package ui

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed style.css
var StyleCSS string

// StyleDarkCSS redefines style.css's surface tokens for a dark scheme.
//
//go:embed style-dark.css
var StyleDarkCSS string

// InstallStyles loads the app stylesheet for the default display and keeps
// the dark override sheet stacked on top exactly while the style manager
// reports a dark scheme (system preference or the Dark setting).
//
// The sheets normally sit at APPLICATION priority, above libadwaita's. GTK
// also loads $XDG_CONFIG_HOME/gtk-4.0/gtk.css at USER priority, above both,
// and a GTK theme installed there (home-manager's gtk.theme does this)
// restyles the whole window: dark surfaces, traffic-light window controls,
// its own entries and buttons. chatot is drawn to its mockup, not to a
// theme, so when that file exists libadwaita's stylesheet and the app's are
// re-applied above it; nothing changes on a system without one.
func InstallStyles() {
	display := gdk.DisplayGetDefault()
	sm := adw.StyleManagerGetDefault()
	prio := uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	if sheet := userStylesheetPath(glib.GetUserConfigDir()); sheet != "" {
		prio = reassertAdwaita(display, sm, loadUserSheet(sheet, 4))
		styleLift = prio - uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	}

	light := gtk.NewCSSProvider()
	light.LoadFromString(StyleCSS)
	gtk.StyleContextAddProviderForDisplay(display, light, prio)

	dark := gtk.NewCSSProvider()
	dark.LoadFromString(StyleDarkCSS)
	installed := false
	apply := func() {
		if sm.Dark() == installed {
			return
		}
		installed = sm.Dark()
		if installed {
			gtk.StyleContextAddProviderForDisplay(display, dark, prio+1)
		} else {
			gtk.StyleContextRemoveProviderForDisplay(display, dark)
		}
	}
	apply()
	sm.NotifyProperty("dark", apply)
}

// isDark reports whether the style manager is currently dark, for the
// cairo-drawn bits that can't read the stylesheet's tokens.
func isDark() bool { return adw.StyleManagerGetDefault().Dark() }

// userStylesheetPath returns the user stylesheet GTK will load,
// configDir/gtk-4.0/gtk.css, or "" when there is none.
func userStylesheetPath(configDir string) string {
	p := filepath.Join(configDir, "gtk-4.0", "gtk.css")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// reassertAdwaita re-adds libadwaita's stylesheet and its accent colours
// above the user stylesheet (GTK's USER priority), mirroring what
// AdwStyleManager installs at THEME priority: the sheet itself switches on
// the display's colour scheme and contrast via media queries, and the accent
// provider is regenerated whenever the system accent changes. Returns the
// priority the app's own sheets should use to sit above both.
func reassertAdwaita(display *gdk.Display, sm *adw.StyleManager, userSheet string) uint {
	settings := gtk.SettingsGetDefault()

	// First a reset of everything the user sheet declares, so properties
	// neither libadwaita nor the app set (a theme's window-control images,
	// button gradients, …) cannot leak through from below.
	reset := gtk.NewCSSProvider()
	reset.LoadFromString(userSheetReset(userSheet))
	mirrorMediaState(settings, reset)
	gtk.StyleContextAddProviderForDisplay(display, reset, gtk.STYLE_PROVIDER_PRIORITY_USER+1)

	base := gtk.NewCSSProvider()
	base.LoadFromResource("/org/gnome/Adwaita/styles/gtk.css")
	mirrorMediaState(settings, base)
	gtk.StyleContextAddProviderForDisplay(display, base, gtk.STYLE_PROVIDER_PRIORITY_USER+2)

	accent := gtk.NewCSSProvider()
	reaccent := func() { accent.LoadFromString(accentCSS(sm.AccentColorRGBA().String())) }
	reaccent()
	sm.NotifyProperty("accent-color-rgba", reaccent)
	gtk.StyleContextAddProviderForDisplay(display, accent, gtk.STYLE_PROVIDER_PRIORITY_USER+3)
	return gtk.STYLE_PROVIDER_PRIORITY_USER + 4
}

// accentCSS is the colour sheet AdwStyleManager generates for the system
// accent bg; the stylesheet derives accent_color and the CSS variables from
// these two names.
func accentCSS(bg string) string {
	return fmt.Sprintf("@define-color accent_bg_color %s;\n@define-color accent_fg_color white;\n", bg)
}

// mediaBindings pairs each GtkSettings interface property with the
// GtkCssProvider property that its @media queries read.
var mediaBindings = [][2]string{
	{"gtk-interface-color-scheme", "prefers-color-scheme"},
	{"gtk-interface-contrast", "prefers-contrast"},
	{"gtk-interface-reduced-motion", "prefers-reduced-motion"},
}

// mirrorMediaState keeps provider's media-query state (prefers-color-scheme
// and friends) equal to the display's, now and on every change. GTK binds
// these only on the providers it creates itself; a provider the app adds
// would otherwise evaluate every @media block as light, normal contrast.
func mirrorMediaState(settings *gtk.Settings, provider *gtk.CSSProvider) {
	for _, b := range mediaBindings {
		src, dst := b[0], b[1]
		sync := func() { provider.SetObjectProperty(dst, settings.ObjectProperty(src)) }
		sync()
		settings.NotifyProperty(src, sync)
	}
}

// styleLift is how far InstallStyles raised the app sheets above their usual
// APPLICATION priority (0 unless a user theme forced them above USER).
var styleLift uint

// widgetPriority maps the priority a per-widget provider would normally use
// (APPLICATION or USER, as the call site chose) onto the current sheet
// stack, so those providers keep sitting above the app sheets exactly as
// they do on a plain system.
func widgetPriority(p uint) uint { return p + styleLift }
