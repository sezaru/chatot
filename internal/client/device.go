package client

import (
	"os"
	"runtime"
	"strings"
	"sync"

	wastore "go.mau.fi/whatsmeow/store"
)

// deviceVersion is the version whatsmeow reports for this companion in the
// pairing payload; it rides along with the name in the phone's device list.
var deviceVersion = [3]uint32{0, 1, 0}

var announceOnce sync.Once

// announceDevice sets the companion identity whatsmeow sends when pairing,
// which is what the phone lists under Linked devices: "Chatot (Linux,
// <hostname>)" instead of the library's default "whatsmeow". The props are
// package globals in whatsmeow and only read at pair time, so this runs once
// before the first client is built.
func announceDevice() {
	announceOnce.Do(func() {
		host, _ := os.Hostname()
		wastore.SetOSInfo(deviceName(runtime.GOOS, host), deviceVersion)
	})
}

// deviceName renders "Chatot (Linux, kepler)"; a missing hostname collapses
// to "Chatot (Linux)". goos is Go's runtime.GOOS spelling.
func deviceName(goos, host string) string {
	parts := []string{osLabel(goos)}
	if host = strings.TrimSpace(host); host != "" {
		parts = append(parts, host)
	}
	return "Chatot (" + strings.Join(parts, ", ") + ")"
}

func osLabel(goos string) string {
	switch goos {
	case "linux":
		return "Linux"
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	case "":
		return "Unknown"
	}
	return strings.ToUpper(goos[:1]) + goos[1:]
}
