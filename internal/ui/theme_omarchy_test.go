package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tokyoNight = `accent = "#7aa2f7"
cursor = "#c0caf5"
foreground = "#a9b1d6"
background = "#1a1b26"
selection_foreground = "#c0caf5"
selection_background = "#7aa2f7"

color0 = "#32344a"
color1 = "#f7768e"
color4 = "#7aa2f7"
color8 = "#444b6a"
`

func writeOmarchyFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), omarchyColors)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseFlatTOML(t *testing.T) {
	got := parseFlatTOML("# palette\nAccent = \"#7aa2f7\" # the blue\nmode = 'light'\nbare = plain # note\n[table]\nx = 1\n")
	want := map[string]string{"accent": "#7aa2f7", "mode": "light", "bare": "plain", "x": "1"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestOmarchyPaletteMapsOntoSheetTokens(t *testing.T) {
	p, err := loadOmarchyPalette(writeOmarchyFixture(t, tokyoNight))
	if err != nil {
		t.Fatal(err)
	}
	if !omarchyDark(p) {
		t.Error("tokyo night read as light")
	}
	css := omarchyCSS(p)
	for _, want := range []string{
		"@define-color chatot_thread #1a1b26;",
		"@define-color chatot_sidebar #222534;", // bg stepped a fifth towards color8
		"@define-color chatot_surface #272a3c;",
		"@define-color window_fg_color #a9b1d6;",
		"@define-color chatot_accent #7aa2f7;",
		"@define-color chatot_on_accent #1a1b26;", // the theme's own background clears contrast on the blue
		"@define-color chatot_accent_text #7aa2f7;",
		"@define-color chatot_bubble_out #374465;",
		"@define-color chatot_on_bubble_out #a9b1d6;",
		"@define-color chatot_danger #f7768e;",
		"@define-color chatot_hairline rgba(169, 177, 214, 0.10);",
		"@define-color accent_bg_color #7aa2f7;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("sheet lacks %q:\n%s", want, css)
		}
	}
}

func TestOmarchyLightThemeLiftsBars(t *testing.T) {
	p := omarchyPalette{"background": "#f0f0f0", "foreground": "#333333", "accent": "#1c71d8", "color8": "#808080", "mode": "light"}
	if omarchyDark(p) {
		t.Error("mode = light read as dark")
	}
	css := omarchyCSS(p)
	for _, want := range []string{
		"@define-color chatot_surface #f8f8f8;", // halfway to white
		"@define-color chatot_sidebar #e5e5e5;",
		"@define-color chatot_on_accent #ffffff;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("sheet lacks %q:\n%s", want, css)
		}
	}
	delete(p, "mode")
	if omarchyDark(p) {
		t.Error("a light background without a mode key read as dark")
	}
}

func TestOmarchyPaletteRefusesIncomplete(t *testing.T) {
	if _, err := loadOmarchyPalette(writeOmarchyFixture(t, "background = \"#000000\"\n")); err == nil {
		t.Error("a palette without foreground and accent loaded")
	}
	if _, err := loadOmarchyPalette(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Error("a missing file loaded")
	}
}

func TestResolveThemeSource(t *testing.T) {
	for in, want := range map[string]string{"none": "", "": "", "bogus": "", "dms": "dms", "omarchy": "omarchy"} {
		if got := ResolveThemeSource(in); got != want {
			t.Errorf("ResolveThemeSource(%q) = %q, want %q", in, got, want)
		}
	}
}
