package main

import (
	"os"
	"path/filepath"
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

func TestInFlatpakFromEnv(t *testing.T) {
	t.Setenv("FLATPAK_ID", appID)
	if !inFlatpak() {
		t.Fatal("FLATPAK_ID set: want inFlatpak")
	}
}

func TestPackagedDesktopEntryScansDataDirs(t *testing.T) {
	empty := t.TempDir()
	pkg := t.TempDir()
	apps := filepath.Join(pkg, "applications")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apps, appID+".desktop"), []byte("[Desktop Entry]\nExec=chatot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if packagedDesktopEntry(empty) {
		t.Error("no entry in data dirs, yet reported packaged")
	}
	if !packagedDesktopEntry(empty + string(os.PathListSeparator) + pkg) {
		t.Error("entry in second data dir not found")
	}
}

func TestRemoveDevDesktopEntryOnlyRemovesDevEntries(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev.desktop")
	packaged := filepath.Join(dir, "pkg.desktop")
	os.WriteFile(dev, []byte(desktopEntry("/tmp/go-build123/exe/chatot")), 0o644)
	os.WriteFile(packaged, []byte("[Desktop Entry]\nExec=chatot\n"), 0o644)
	removeDevDesktopEntry(dev)
	removeDevDesktopEntry(packaged)
	if _, err := os.Stat(dev); err == nil {
		t.Error("dev entry (absolute Exec) still present")
	}
	if _, err := os.Stat(packaged); err != nil {
		t.Error("packaged-style entry was removed")
	}
}
