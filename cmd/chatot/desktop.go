package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"chatot/internal/ui"
)

// installDesktopEntry drops a .desktop file and the app icon into the
// user's XDG data dir so the compositor, the shell's overview, the dock and
// notifications can show chatot's own mark for app id com.sezdm.chatot
// instead of a generic one. Wayland resolves a window's icon through its
// app id → .desktop file, so this cannot be done from the window itself.
// Re-run on every start because the Exec path follows the binary; a
// packaged install (with its own entry) can point CHATOT_NO_DESKTOP_ENTRY at
// anything to skip it, and a Flatpak (whose entry the sandbox exports, and
// whose data dir the shell never reads) is skipped on its own.
func installDesktopEntry() error {
	if os.Getenv("CHATOT_NO_DESKTOP_ENTRY") != "" || inFlatpak() {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	homes, err := desktopDataHomes()
	if err != nil {
		return err
	}
	for _, data := range homes {
		apps := filepath.Join(data, "icons", "hicolor")
		if err := writeIfChanged(filepath.Join(apps, "scalable", "apps", appID+".svg"), ui.AppMarkSVG()); err != nil {
			return err
		}
		// A raster copy beside the SVG for lookups that cannot render one
		// (gdk-pixbuf without its librsvg loader, some panels).
		if err := writeIfChanged(filepath.Join(apps, "512x512", "apps", appID+".png"), ui.AppMarkPNG()); err != nil {
			return err
		}
		entry := filepath.Join(data, "applications", appID+".desktop")
		if err := writeIfChanged(entry, []byte(desktopEntry(exe))); err != nil {
			return err
		}
	}
	return nil
}

// desktopDataHomes lists where the entry goes: the process's XDG_DATA_HOME
// and, when that is somewhere else (a dev shell that redirects it), the
// default ~/.local/share too. The entry exists for the desktop shell, and
// the shell reads the session's data home, not the app's.
func desktopDataHomes() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	def := filepath.Join(home, ".local", "share")
	if data := os.Getenv("XDG_DATA_HOME"); data != "" && data != def {
		return []string{data, def}, nil
	}
	return []string{def}, nil
}

// desktopEntry renders the .desktop file for the binary at exe.
func desktopEntry(exe string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=chatot
Comment=WhatsApp for the GNOME desktop
Exec=%s
Icon=%s
Terminal=false
Categories=Network;InstantMessaging;GTK;
StartupNotify=true
StartupWMClass=%s
`, exe, appID, appID)
}

// writeIfChanged writes data to path unless the file already holds it.
func writeIfChanged(path string, data []byte) error {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// inFlatpak reports whether the process runs inside a Flatpak sandbox:
// flatpak sets FLATPAK_ID for the app and mounts its metadata at
// /.flatpak-info.
func inFlatpak() bool {
	if os.Getenv("FLATPAK_ID") != "" {
		return true
	}
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}
