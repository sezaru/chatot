package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dmsFixture = `{"colors": {
  "dark": {"surface": "#18130b", "surface_container_high": "#241f17", "surface_container_highest": "#2f2921",
           "surface_container_lowest": "#120d07", "on_surface": "#ede1d4", "outline": "#9b8f80", "outline_variant": "#4f4539",
           "primary": "#f2be6e", "on_primary": "#442c00", "primary_container": "#614000", "on_primary_container": "#ffddb0",
           "error": "#ffb4ab", "on_error": "#690005"},
  "light": {"surface": "#fff8f3", "surface_container_high": "#f8ecdf", "surface_container_highest": "#f2e6da",
            "surface_container_lowest": "#ffffff", "on_surface": "#201b13", "outline": "#817567", "outline_variant": "#d2c4b4",
            "primary": "#7e570f", "on_primary": "#ffffff", "primary_container": "#ffddb0", "on_primary_container": "#281800",
            "error": "#ba1a1a", "on_error": "#ffffff"}
}}`

func writeDMSFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), dmsColorsFile)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDMSPaletteMapsOntoSheetTokens(t *testing.T) {
	p, err := loadDMSPalette(writeDMSFixture(t, dmsFixture))
	if err != nil {
		t.Fatal(err)
	}
	dark := p.css(true)
	for _, want := range []string{
		"@define-color chatot_thread #18130b;",
		"@define-color chatot_sidebar #241f17;",
		"@define-color chatot_surface #2f2921;", // highest in dark: bars stay the lightest surface
		"@define-color view_bg_color #120d07;",
		"@define-color window_fg_color #ede1d4;",
		"@define-color chatot_accent #f2be6e;",
		"@define-color chatot_on_accent #442c00;",
		"@define-color chatot_accent_hover #d0a35f;",
		"@define-color chatot_bubble_out #614000;",
		"@define-color chatot_on_bubble_out #ffddb0;",
		"@define-color chatot_accent_text #f2be6e;",
		"@define-color chatot_danger #ffb4ab;",
		"@define-color chatot_on_danger #690005;",
		"@define-color chatot_hairline rgba(79, 69, 57, 0.55);",
		"@define-color accent_bg_color #f2be6e;",
	} {
		if !strings.Contains(dark, want) {
			t.Errorf("dark sheet lacks %q:\n%s", want, dark)
		}
	}
	light := p.css(false)
	for _, want := range []string{
		"@define-color chatot_surface #ffffff;", // lowest in light: bars white, as the mockup
		"@define-color chatot_sidebar #f8ecdf;",
		"@define-color chatot_accent #7e570f;",
		"@define-color chatot_on_bubble_out #281800;",
	} {
		if !strings.Contains(light, want) {
			t.Errorf("light sheet lacks %q:\n%s", want, light)
		}
	}
}

func TestDMSPaletteSkipsMissingAndBadTokens(t *testing.T) {
	css := dmsCSS(map[string]string{"primary": "#123456", "surface": "not a colour", "error": "#12345"}, false)
	if !strings.Contains(css, "@define-color chatot_accent #123456;") {
		t.Errorf("primary not mapped:\n%s", css)
	}
	for _, absent := range []string{"chatot_thread", "chatot_danger", "chatot_bubble_out", "chatot_hairline "} {
		if strings.Contains(css, absent) {
			t.Errorf("%s defined from a missing or malformed token:\n%s", absent, css)
		}
	}
}

func TestDMSPaletteSingleModeServesBoth(t *testing.T) {
	p, err := loadDMSPalette(writeDMSFixture(t, `{"colors": {"dark": {"primary": "#abcdef"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.css(false), "chatot_accent #abcdef") {
		t.Errorf("light sheet did not fall back to the dark set:\n%s", p.css(false))
	}
	if _, err := loadDMSPalette(writeDMSFixture(t, `{"colors": {}}`)); err == nil {
		t.Error("an empty palette loaded")
	}
	if _, err := loadDMSPalette(writeDMSFixture(t, `{`)); err == nil {
		t.Error("malformed JSON loaded")
	}
}

func TestShadeHex(t *testing.T) {
	if got := shadeHex("#ff8000", 0.5); got != "#804000" {
		t.Errorf("shadeHex = %s", got)
	}
	if got := shadeHex("zzz", 0.5); got != "" {
		t.Errorf("bad input gave %q", got)
	}
}
