package main

import (
	"strings"
	"testing"
)

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("/opt/chatot/bin/chatot")
	for _, want := range []string{"Exec=/opt/chatot/bin/chatot\n", "Icon=" + appID + "\n", "StartupWMClass=" + appID + "\n", "[Desktop Entry]\n"} {
		if !strings.Contains(e, want) {
			t.Errorf("desktop entry lacks %q:\n%s", want, e)
		}
	}
}
