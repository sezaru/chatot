package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserStylesheetPath(t *testing.T) {
	dir := t.TempDir()
	if p := userStylesheetPath(dir); p != "" {
		t.Fatalf("empty config dir reported a user stylesheet %q", p)
	}
	if err := os.MkdirAll(filepath.Join(dir, "gtk-4.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gtk-4.0", "gtk.css"), []byte("@import url(\"theme.css\");"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := userStylesheetPath(dir); p != filepath.Join(dir, "gtk-4.0", "gtk.css") {
		t.Fatalf("gtk-4.0/gtk.css present but got %q", p)
	}
}

func TestAccentCSSDefinesBothNames(t *testing.T) {
	css := accentCSS("rgb(53,132,228)")
	for _, want := range []string{"@define-color accent_bg_color rgb(53,132,228);", "@define-color accent_fg_color white;"} {
		if !strings.Contains(css, want) {
			t.Errorf("accent css lacks %q:\n%s", want, css)
		}
	}
}
