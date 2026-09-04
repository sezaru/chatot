package client

import (
	"testing"

	wastore "go.mau.fi/whatsmeow/store"
)

func TestDeviceName(t *testing.T) {
	cases := map[[2]string]string{
		{"linux", "kepler"}:   "chatot (Linux, kepler)",
		{"linux", ""}:         "chatot (Linux)",
		{"linux", "  host  "}: "chatot (Linux, host)",
		{"darwin", "mbp"}:     "chatot (macOS, mbp)",
		{"freebsd", "box"}:    "chatot (FreeBSD, box)",
		{"plan9", "glenda"}:   "chatot (Plan9, glenda)",
		{"", "x"}:             "chatot (Unknown, x)",
	}
	for in, want := range cases {
		if got := deviceName(in[0], in[1]); got != want {
			t.Errorf("deviceName(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestAnnounceDeviceReplacesWhatsmeowDefault(t *testing.T) {
	announceDevice()
	if got := wastore.DeviceProps.GetOs(); got == "whatsmeow" || got == "" || got[:8] != "chatot (" {
		t.Fatalf("DeviceProps.Os = %q, want a chatot device name", got)
	}
	if wastore.DeviceProps.Version.GetPrimary() != deviceVersion[0] || wastore.DeviceProps.Version.GetSecondary() != deviceVersion[1] {
		t.Fatalf("DeviceProps.Version = %v, want %v", wastore.DeviceProps.Version, deviceVersion)
	}
}
