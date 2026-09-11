package ui

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// Omarchy Quattro keeps the active theme under
// ~/.local/state/omarchy/current: theme.name, and theme/colors.toml with the
// palette every app gets retinted from (background, foreground, accent,
// the selection pair, color0 to color15, and mode = "light" for a light
// theme). omarchy-theme-set stages the next theme beside it and swaps the
// directory in, so a watch on "current" sees a new theme as an event on
// the "theme" entry. Earlier Omarchy layouts (~/.config/omarchy, colours
// pulled from alacritty.toml) are not read.

const (
	omarchyThemeDir  = "theme"
	omarchyThemeName = "theme.name"
	omarchyColors    = "colors.toml"
)

// omarchyStateDir is the directory Omarchy keeps the current theme in.
func omarchyStateDir() string {
	return filepath.Join(glib.GetUserStateDir(), "omarchy", "current")
}

func omarchyColorsPath() string {
	return filepath.Join(omarchyStateDir(), omarchyThemeDir, omarchyColors)
}

// omarchySource is Omarchy Quattro as a shell palette source.
var omarchySource = &shellSource{
	name:  "omarchy",
	label: "Omarchy",
	available: func() bool {
		st, err := os.Stat(omarchyColorsPath())
		return err == nil && !st.IsDir()
	},
	load: func() (func(dark bool) string, error) {
		p, err := loadOmarchyPalette(omarchyColorsPath())
		if err != nil {
			return nil, err
		}
		css := omarchyCSS(p)
		// The theme is one palette with a mode of its own; it is applied
		// as-is whichever scheme the window is in.
		return func(bool) string { return css }, nil
	},
	watchDir:   omarchyStateDir,
	watchNames: []string{omarchyThemeDir, omarchyThemeName},
}

// omarchyPalette is colors.toml: every string value by key.
type omarchyPalette map[string]string

// loadOmarchyPalette reads colors.toml. A palette without a background,
// foreground and accent is refused.
func loadOmarchyPalette(path string) (omarchyPalette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := parseFlatTOML(string(data))
	for _, key := range []string{"background", "foreground", "accent"} {
		if _, ok := parseHex(p[key]); !ok {
			return nil, fmt.Errorf("%s: no usable %s", filepath.Base(path), key)
		}
	}
	if len(p) == 0 {
		return nil, errors.New(filepath.Base(path) + ": empty")
	}
	return p, nil
}

// parseFlatTOML reads the `key = "value"` lines of a flat TOML file (the
// only shape colors.toml has), ignoring tables, comments and anything it
// does not understand. Values keep their case; keys are lower-cased.
func parseFlatTOML(src string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') {
			if end := strings.IndexByte(val[1:], val[0]); end >= 0 {
				val = val[1 : 1+end]
			}
		} else if i := strings.IndexByte(val, '#'); i >= 0 {
			val = strings.TrimSpace(val[:i]) // an unquoted value, then a comment
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}

// omarchyDark reports whether the palette is a dark theme: its mode key
// when it has one, else the background's luminance.
func omarchyDark(p omarchyPalette) bool {
	switch strings.ToLower(p["mode"]) {
	case "light":
		return false
	case "dark":
		return true
	}
	return luminance(p["background"]) < 0.5
}

// omarchyCSS maps the palette onto the sheet's tokens and libadwaita's
// names. Omarchy gives one surface, so the others are steps from it
// towards color8 (the bright black, a mid grey in both light and dark
// themes) the way the design's own surfaces step: the thread is the
// background, the sidebar a step off it, bars and cards the far step in
// dark themes and a lift towards white in light ones. The accent fills
// buttons and tints the outgoing bubble; color1 is the danger tone.
func omarchyCSS(p omarchyPalette) string {
	var b strings.Builder
	def := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "@define-color %s %s;\n", name, value)
		}
	}
	hex := func(key string) string {
		v := strings.TrimSpace(p[key])
		if _, ok := parseHex(v); !ok {
			return ""
		}
		return v
	}
	dark := omarchyDark(p)
	bg, fg := hex("background"), hex("foreground")
	step := hex("color8")
	if step == "" {
		step = fg
	}
	sidebar := mixHex(bg, step, 0.2)
	surface := mixHex(bg, step, 0.32)
	if !dark {
		sidebar = mixHex(bg, step, 0.1)
		surface = mixHex(bg, "#ffffff", 0.5)
	}
	def("chatot_thread", bg)
	def("chatot_sidebar", sidebar)
	def("sidebar_bg_color", sidebar)
	def("sidebar_backdrop_color", sidebar)
	def("chatot_surface", surface)
	for _, n := range []string{"popover_bg_color", "card_bg_color", "dialog_bg_color", "headerbar_bg_color", "headerbar_backdrop_color"} {
		def(n, surface)
	}
	def("window_bg_color", bg)
	def("view_bg_color", bg)
	for _, n := range []string{"window_fg_color", "view_fg_color", "headerbar_fg_color", "popover_fg_color", "card_fg_color", "dialog_fg_color", "sidebar_fg_color"} {
		def(n, fg)
	}
	def("chatot_hairline", withAlpha(fg, 0.1))
	def("chatot_hairline_strong", withAlpha(fg, 0.16))
	def("chatot_wash", withAlpha(fg, 0.05))
	def("chatot_ring_viewed", withAlpha(fg, 0.3))

	accent := hex("accent")
	if accent == "" {
		accent = hex("color4")
	}
	onAccent := contrastText(accent, bg, fg)
	def("chatot_accent", accent)
	def("chatot_on_accent", onAccent)
	def("chatot_accent_hover", shadeHex(accent, 0.86))
	def("chatot_accent_active", shadeHex(accent, 0.93))
	def("accent_bg_color", accent)
	def("accent_fg_color", onAccent)
	def("accent_color", accent)
	for _, n := range []string{"chatot_accent_text", "chatot_accent_text_soft", "chatot_accent_text_lift", "chatot_transcript_head", "chatot_tick_read"} {
		def(n, accent)
	}
	bubble := mixHex(bg, accent, 0.3)
	if !dark {
		bubble = mixHex(bg, accent, 0.22)
	}
	def("chatot_bubble_out", bubble)
	def("chatot_on_bubble_out", fg)

	danger := hex("color1")
	if danger == "" {
		danger = "#c01c28"
	}
	onDanger := contrastText(danger, bg, fg)
	def("chatot_danger", danger)
	def("chatot_danger_text", danger)
	def("chatot_on_danger", onDanger)
	for _, n := range []string{"destructive_bg_color", "error_bg_color", "error_color"} {
		def(n, danger)
	}
	for _, n := range []string{"destructive_fg_color", "error_fg_color"} {
		def(n, onDanger)
	}
	return b.String()
}

// mixHex blends a towards b by t (0 keeps a, 1 gives b); "" if either is
// not a colour.
func mixHex(a, b string, t float64) string {
	ca, oka := parseHex(a)
	cb, okb := parseHex(b)
	if !oka || !okb {
		return ""
	}
	var c [3]int
	for i := range c {
		c[i] = int(float64(ca[i])*(1-t) + float64(cb[i])*t + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
}

// luminance is the relative luminance of a colour, 0 for black and 1 for
// white; 0 when it is not a colour.
func luminance(hex string) float64 {
	c, ok := parseHex(hex)
	if !ok {
		return 0
	}
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

// contrastText picks the text colour for fills of bg: white or black,
// whichever contrasts more, unless the palette's own background or
// foreground does about as well, which keeps the theme's tone.
func contrastText(fill, paletteBg, paletteFg string) string {
	l := luminance(fill)
	contrast := func(other string) float64 {
		lo := luminance(other)
		hi, lo2 := l, lo
		if lo > hi {
			hi, lo2 = lo, l
		}
		return (hi + 0.05) / (lo2 + 0.05)
	}
	best, bestC := "#ffffff", contrast("#ffffff")
	if c := contrast("#000000"); c > bestC {
		best, bestC = "#000000", c
	}
	for _, own := range []string{paletteFg, paletteBg} {
		if _, ok := parseHex(own); ok && contrast(own) >= 4.5 && contrast(own) >= bestC*0.8 {
			return own
		}
	}
	return best
}
