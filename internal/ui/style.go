// Package ui holds the gotk4/libadwaita widgets (chat list, conversation,
// composer, ...) built up feature by feature.
package ui

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
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
func InstallStyles() {
	display := gdk.DisplayGetDefault()
	light := gtk.NewCSSProvider()
	light.LoadFromString(StyleCSS)
	gtk.StyleContextAddProviderForDisplay(display, light, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	dark := gtk.NewCSSProvider()
	dark.LoadFromString(StyleDarkCSS)
	installed := false
	sm := adw.StyleManagerGetDefault()
	apply := func() {
		if sm.Dark() == installed {
			return
		}
		installed = sm.Dark()
		if installed {
			gtk.StyleContextAddProviderForDisplay(display, dark, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION+1)
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
