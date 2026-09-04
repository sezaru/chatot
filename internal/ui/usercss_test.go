package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserSheetResetUnsetsEveryDeclaration(t *testing.T) {
	in := `/* theme */
@define-color window_bg_color #222;
@import url("other.css");
window, .titlebar { background-color: #222; color: white; }
windowcontrols button image { -gtk-icon-source: image(url("assets/close.png")); min-width: 0 }
@media (prefers-color-scheme: dark) {
  entry { background: rgba(0,0,0,0.3); border-radius: 6px; }
}
button:hover { box-shadow: inset 0 0 0 1px alpha(currentColor, 0.1); content: "a;b}" }`
	got := userSheetReset(in)
	for _, want := range []string{
		"window, .titlebar { background-color: unset; color: unset; }",
		"windowcontrols button image { -gtk-icon-source: unset; min-width: unset; }",
		"@media (prefers-color-scheme: dark) {",
		"entry { background: unset; border-radius: unset; }",
		"button:hover { box-shadow: unset; content: unset; }",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reset lacks %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"@define-color", "@import", "#222", "close.png", "a;b}"} {
		if strings.Contains(got, banned) {
			t.Errorf("reset still carries %q:\n%s", banned, got)
		}
	}
	if strings.Count(got, "{") != strings.Count(got, "}") {
		t.Errorf("unbalanced braces:\n%s", got)
	}
}

func TestLoadUserSheetInlinesImports(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "theme.css"), []byte("button { color: red; }"), 0o644)
	os.WriteFile(filepath.Join(dir, "gtk.css"), []byte(`@import url("file://`+filepath.Join(dir, "theme.css")+`");`+"\n@import 'theme.css';\nentry { color: blue; }"), 0o644)
	got := loadUserSheet(filepath.Join(dir, "gtk.css"), 3)
	if strings.Count(got, "button { color: red; }") != 2 || !strings.Contains(got, "entry { color: blue; }") {
		t.Errorf("imports not inlined:\n%s", got)
	}
	if strings.Contains(got, "@import") {
		t.Errorf("@import left behind:\n%s", got)
	}
}
