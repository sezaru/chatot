package ui

import (
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/audio"
	"chatot/internal/settings"
)

// Microphone comes from Preferences › Network › Voice notes: the PulseAudio
// source voice notes are recorded from (audio.Source.Name), or "" for the
// system default.
var Microphone string

// listMicrophones is audio.Sources, swappable in tests.
var listMicrophones = audio.Sources

// microphoneLabelMax caps a source's name on the row and in the menu; the
// sound server's descriptions run long ("Karaoke Microphone (no
// beamformer)").
const microphoneLabelMax = 32

// microphoneLabel is how the row names the source picked by name: "System
// default" for "", else its description out of srcs, or the bare name
// while the list is unknown or the device is unplugged.
func microphoneLabel(name string, srcs []audio.Source) string {
	if name == "" {
		return "System default"
	}
	for _, src := range srcs {
		if src.Name == name {
			return clipRunes(src.Description, microphoneLabelMax)
		}
	}
	return clipRunes(name, microphoneLabelMax)
}

// microphoneMenu lists the choices: the system default first, naming the
// source it currently resolves to, then every microphone.
func microphoneMenu(srcs []audio.Source, pick func(name string)) []menuItem {
	def := "System default"
	for _, src := range srcs {
		if src.Default {
			def += " · " + clipRunes(src.Description, microphoneLabelMax)
		}
	}
	items := []menuItem{{Label: def, OnActivate: func() { pick("") }}}
	if len(srcs) > 0 {
		items = append(items, menuSeparator())
	}
	for _, src := range srcs {
		name := src.Name
		items = append(items, menuItem{
			Label: clipRunes(src.Description, microphoneLabelMax), OnActivate: func() { pick(name) },
		})
	}
	return items
}

// microphoneRow is Preferences' "Microphone" chooser. The sound server is
// asked for its sources off the main thread, once to name the saved pick
// and again on every click, so a headset plugged in since shows up.
func microphoneRow(s *settings.Settings, onChange func(settings.Settings)) gtk.Widgetter {
	var row gtk.Widgetter
	var value *gtk.Label
	setValue := func(srcs []audio.Source) {
		value.SetText(microphoneLabel(s.Microphone, srcs) + " ▾")
	}
	withSources := func(then func([]audio.Source)) {
		go func() {
			srcs, err := listMicrophones()
			glib.IdleAdd(func() {
				if value.Root() == nil {
					return
				}
				if err != nil {
					prefToast("Couldn't list microphones: " + err.Error())
				}
				then(srcs)
			})
		}()
	}
	row, value = newValueRow("Microphone", "Voice notes are recorded from it",
		microphoneLabel(s.Microphone, nil), func() {
			withSources(func(srcs []audio.Source) {
				popupMenuBelow(row, microphoneMenu(srcs, func(name string) {
					s.Microphone = name
					Microphone = name
					setValue(srcs)
					onChange(*s)
				}))
			})
		})
	if s.Microphone != "" {
		withSources(setValue)
	}
	return row
}

// clipRunes shortens s to at most n runes, ending in "…" when cut.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
