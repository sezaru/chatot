package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// DankMaterialShell derives a Material palette from the wallpaper (or a
// picked theme) with matugen and writes every token, for dark and light,
// to $XDG_CACHE_HOME/DankMaterialShell/dms-colors.json; it rewrites the
// file on each wallpaper or theme change. Its own GTK route (a gtk.css
// importing dank-colors.css) never reaches this app, which re-applies
// libadwaita above the user sheet, so the file is read directly.

const dmsColorsFile = "dms-colors.json"

// dmsCacheDir is the directory DMS keeps its generated palette in.
func dmsCacheDir() string { return filepath.Join(glib.GetUserCacheDir(), "DankMaterialShell") }

func dmsColorsPath() string { return filepath.Join(dmsCacheDir(), dmsColorsFile) }

// dmsPalette is the two Material token sets DMS writes.
type dmsPalette struct {
	Dark, Light map[string]string
}

// loadDMSPalette reads dms-colors.json. A file with only one of the two
// sets (a single-mode custom theme) uses it for both.
func loadDMSPalette(path string) (dmsPalette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return dmsPalette{}, err
	}
	var doc struct {
		Colors struct {
			Dark  map[string]string `json:"dark"`
			Light map[string]string `json:"light"`
		} `json:"colors"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return dmsPalette{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	p := dmsPalette{Dark: doc.Colors.Dark, Light: doc.Colors.Light}
	switch {
	case len(p.Dark) == 0 && len(p.Light) == 0:
		return dmsPalette{}, errors.New(filepath.Base(path) + ": no colours")
	case len(p.Dark) == 0:
		p.Dark = p.Light
	case len(p.Light) == 0:
		p.Light = p.Dark
	}
	return p, nil
}

// css returns the override sheet for the current scheme.
func (p dmsPalette) css(dark bool) string {
	if dark {
		return dmsCSS(p.Dark, true)
	}
	return dmsCSS(p.Light, false)
}

// dmsCSS maps one Material token set onto the sheet's tokens and the
// libadwaita names the sheet and its widgets read. Surfaces step the way
// the design's do: the thread is the base surface, the sidebar a container
// tone above it, bars and cards the tone furthest from the base (lowest in
// light, highest in dark, so bars stay the lightest surface in dark as in
// style-dark.css). Buttons take the primary, outgoing bubbles its container
// tone. A token the set lacks is left to the design's value.
func dmsCSS(m map[string]string, dark bool) string {
	var b strings.Builder
	def := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "@define-color %s %s;\n", name, value)
		}
	}
	hex := func(key string) string {
		v := strings.TrimSpace(m[key])
		if _, ok := parseHex(v); !ok {
			return ""
		}
		return v
	}
	surfaceKey := "surface_container_lowest"
	if dark {
		surfaceKey = "surface_container_highest"
	}
	surface, sidebar, thread := hex(surfaceKey), hex("surface_container_high"), hex("surface")
	fg := hex("on_surface")
	def("chatot_thread", thread)
	def("chatot_sidebar", sidebar)
	def("sidebar_bg_color", sidebar)
	def("sidebar_backdrop_color", sidebar)
	def("chatot_surface", surface)
	for _, n := range []string{"popover_bg_color", "card_bg_color", "dialog_bg_color", "headerbar_bg_color", "headerbar_backdrop_color"} {
		def(n, surface)
	}
	def("window_bg_color", thread)
	def("view_bg_color", hex("surface_container_lowest"))
	for _, n := range []string{"window_fg_color", "view_fg_color", "headerbar_fg_color", "popover_fg_color", "card_fg_color", "dialog_fg_color", "sidebar_fg_color"} {
		def(n, fg)
	}
	def("chatot_hairline", withAlpha(hex("outline_variant"), 0.55))
	def("chatot_hairline_strong", hex("outline_variant"))
	def("chatot_wash", withAlpha(fg, 0.05))
	def("chatot_ring_viewed", hex("outline"))

	primary, onPrimary := hex("primary"), hex("on_primary")
	def("chatot_accent", primary)
	def("chatot_on_accent", onPrimary)
	def("chatot_accent_hover", shadeHex(primary, 0.86))
	def("chatot_accent_active", shadeHex(primary, 0.93))
	def("accent_bg_color", primary)
	def("accent_fg_color", onPrimary)
	def("accent_color", primary)
	for _, n := range []string{"chatot_accent_text", "chatot_accent_text_soft", "chatot_accent_text_lift", "chatot_transcript_head", "chatot_tick_read"} {
		def(n, primary)
	}
	def("chatot_bubble_out", hex("primary_container"))
	def("chatot_on_bubble_out", hex("on_primary_container"))

	danger, onDanger := hex("error"), hex("on_error")
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

// parseHex reads "#rrggbb" into its channels.
func parseHex(s string) ([3]uint8, bool) {
	var c [3]uint8
	if len(s) != 7 || s[0] != '#' {
		return c, false
	}
	for i := 0; i < 3; i++ {
		n, err := parseHexByte(s[1+2*i : 3+2*i])
		if err != nil {
			return c, false
		}
		c[i] = n
	}
	return c, true
}

func parseHexByte(s string) (uint8, error) {
	var n uint8
	for _, r := range s {
		var d uint8
		switch {
		case r >= '0' && r <= '9':
			d = uint8(r - '0')
		case r >= 'a' && r <= 'f':
			d = uint8(r-'a') + 10
		case r >= 'A' && r <= 'F':
			d = uint8(r-'A') + 10
		default:
			return 0, fmt.Errorf("bad hex %q", s)
		}
		n = n<<4 | d
	}
	return n, nil
}

// withAlpha writes hex at alpha as an rgba() colour; "" stays "".
func withAlpha(hex string, alpha float64) string {
	c, ok := parseHex(hex)
	if !ok {
		return ""
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %.2f)", c[0], c[1], c[2], alpha)
}

// shadeHex scales hex's channels by f (under 1 darkens); "" stays "".
func shadeHex(hex string, f float64) string {
	c, ok := parseHex(hex)
	if !ok {
		return ""
	}
	ch := func(v uint8) int {
		n := int(float64(v)*f + 0.5)
		if n > 255 {
			n = 255
		}
		return n
	}
	return fmt.Sprintf("#%02x%02x%02x", ch(c[0]), ch(c[1]), ch(c[2]))
}

// dmsSource is DMS as a shell palette source.
var dmsSource = &shellSource{
	name:  "dms",
	label: "DankMaterialShell",
	available: func() bool {
		st, err := os.Stat(dmsColorsPath())
		return err == nil && !st.IsDir()
	},
	load: func() (func(dark bool) string, error) {
		p, err := loadDMSPalette(dmsColorsPath())
		if err != nil {
			return nil, err
		}
		return p.css, nil
	},
	watchDir:   dmsCacheDir,
	watchNames: []string{dmsColorsFile},
}
