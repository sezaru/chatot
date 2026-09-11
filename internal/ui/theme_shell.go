package ui

import (
	"context"
	"log"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// A desktop shell's palette can restate the sheet's tokens (and
// libadwaita's named colours) from a provider above the app sheets, so the
// whole window follows the wallpaper or the theme the shell is set to.
// Each shell is a shellSource: where its palette is, how it maps onto the
// tokens, and which files to watch for a change. DMS is in theme_dms.go,
// Omarchy Quattro in theme_omarchy.go. The design's own colours come back
// the moment the source is switched off or its files are gone.

// shellSource describes one shell's palette.
type shellSource struct {
	// name is the settings.ThemeSource value.
	name string
	// label names the shell in the preference row.
	label string
	// available reports whether the shell has written a palette to follow.
	available func() bool
	// load reads the palette and returns the sheet for either scheme.
	load func() (func(dark bool) string, error)
	// watchDir is the directory whose entries named watchNames signal a
	// new palette (shells replace files rather than edit them in place,
	// so the directory is watched, not the file).
	watchDir   func() string
	watchNames []string
}

// shellSources lists the shells, in the order "auto" tries them.
var shellSources = []*shellSource{dmsSource, omarchySource}

func shellSourceNamed(name string) *shellSource {
	for _, s := range shellSources {
		if s.name == name {
			return s
		}
	}
	return nil
}

// AvailableThemeSources lists the shells that have a palette right now,
// for the preference row.
func AvailableThemeSources() []*shellSource {
	var out []*shellSource
	for _, s := range shellSources {
		if s.available() {
			out = append(out, s)
		}
	}
	return out
}

// ThemeSourceLabel names a ThemeSource value for the preference row.
func ThemeSourceLabel(name string) string {
	if s := shellSourceNamed(name); s != nil {
		return s.label
	}
	return "chatot's design"
}

// ResolveThemeSource turns a settings.ThemeSource value into the source in
// effect: a shell's name, or "" for the design's own colours. "auto" takes
// the first shell with a palette.
func ResolveThemeSource(source string) string {
	switch source {
	case "auto":
		for _, s := range shellSources {
			if s.available() {
				return s.name
			}
		}
	case "none", "":
	default:
		if shellSourceNamed(source) != nil {
			return source
		}
	}
	return ""
}

func (s *shellSource) watches(f gio.Filer) bool {
	if f == nil {
		return false
	}
	name := f.Basename()
	for _, n := range s.watchNames {
		if n == name {
			return true
		}
	}
	return false
}

// shellTheme is the provider slot above the app sheets that a shell's
// palette fills, with the watch that keeps it current.
type shellTheme struct {
	display   *gdk.Display
	sm        *adw.StyleManager
	prio      uint
	provider  *gtk.CSSProvider
	installed bool
	ready     bool
	wanted    string
	src       *shellSource
	sheet     func(dark bool) string
	monitor   *gio.FileMonitor
	pending   glib.SourceHandle
}

var shell shellTheme

// ApplyThemeSource selects where the sheet's colours come from (a
// settings.ThemeSources value). Safe before InstallStyles: the choice is
// kept and applied once the sheets are in.
func ApplyThemeSource(source string) {
	shell.wanted = source
	if shell.ready {
		shell.apply()
	}
}

// init prepares the slot at prio, above the app sheets, and re-derives the
// sheet when the scheme flips between dark and light.
func (t *shellTheme) init(display *gdk.Display, sm *adw.StyleManager, prio uint) {
	t.display, t.sm, t.prio = display, sm, prio
	t.provider = gtk.NewCSSProvider()
	t.ready = true
	sm.NotifyProperty("dark", func() {
		if t.installed && t.sheet != nil {
			t.provider.LoadFromString(t.sheet(sm.Dark()))
		}
	})
	t.apply()
}

func (t *shellTheme) apply() {
	src := shellSourceNamed(ResolveThemeSource(t.wanted))
	if src == nil {
		t.uninstall()
		t.unwatch()
		t.src = nil
		return
	}
	if t.src != src {
		t.unwatch()
		t.src = src
	}
	t.reload()
	t.watch()
}

// reload reads the palette and (re)loads its sheet; an unreadable palette
// drops the sheet so the design's colours show rather than stale ones.
func (t *shellTheme) reload() {
	sheet, err := t.src.load()
	if err != nil {
		log.Printf("chatot: shell theme: %s: %v", t.src.label, err)
		t.sheet = nil
		t.uninstall()
		return
	}
	t.sheet = sheet
	t.provider.LoadFromString(sheet(t.sm.Dark()))
	if !t.installed {
		gtk.StyleContextAddProviderForDisplay(t.display, t.provider, t.prio)
		t.installed = true
	}
	trace(1, "shell theme: %s palette loaded (dark=%v, accent text %s)", t.src.label, t.sm.Dark(), tokenHex("chatot_accent_text", "unresolved"))
}

func (t *shellTheme) uninstall() {
	if t.installed {
		gtk.StyleContextRemoveProviderForDisplay(t.display, t.provider)
		t.installed = false
	}
}

// watch follows the source's directory and reloads a moment after the
// last event of a burst.
func (t *shellTheme) watch() {
	if t.monitor != nil {
		return
	}
	src := t.src
	dir := gio.NewFileForPath(src.watchDir())
	m, err := dir.MonitorDirectory(context.Background(), gio.FileMonitorNone)
	if err != nil {
		log.Printf("chatot: shell theme: watch %s: %v", src.watchDir(), err)
		return
	}
	mon := gio.BaseFileMonitor(m)
	mon.ConnectChanged(func(file, other gio.Filer, _ gio.FileMonitorEvent) {
		if !src.watches(file) && !src.watches(other) {
			return
		}
		if t.pending != 0 {
			glib.SourceRemove(t.pending)
		}
		t.pending = glib.TimeoutAdd(200, func() bool {
			t.pending = 0
			if t.src == src && ResolveThemeSource(t.wanted) == src.name {
				t.reload()
			}
			return false
		})
	})
	t.monitor = mon
}

func (t *shellTheme) unwatch() {
	if t.monitor == nil {
		return
	}
	t.monitor.Cancel()
	t.monitor = nil
	if t.pending != 0 {
		glib.SourceRemove(t.pending)
		t.pending = 0
	}
}
