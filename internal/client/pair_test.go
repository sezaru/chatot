package client

import (
	"regexp"
	"testing"
)

func TestNormalizePairingPhone(t *testing.T) {
	cases := []struct {
		in      string
		wantOK  bool
		wantOut string
	}{
		{"+1 (555) 123-4567", true, "15551234567"},
		{"15551234567", true, "15551234567"},
		{"555-123", false, ""},
		{"5551234", true, "5551234"},
		{"1234567890123456", false, ""}, // 16 digits, too long
		{"+44 20 7946 0958", true, "442079460958"},
		{"abc1234567", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		got, ok := normalizePairingPhone(c.in)
		if ok != c.wantOK || got != c.wantOut {
			t.Errorf("normalizePairingPhone(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.wantOut, c.wantOK)
		}
	}
}

func TestPairDisplayNameIsBrowserOS(t *testing.T) {
	// WhatsApp validates the pairing-code display name as "Browser (OS)"
	// and answers 400 bad-request to anything else.
	if !regexp.MustCompile(`^(Chrome|Firefox|Safari|Edge|Opera) \((Linux|Windows|macOS|Mac OS|Android|iOS)\)$`).MatchString(pairDisplayName) {
		t.Errorf("pairDisplayName = %q, want a Browser (OS) name WhatsApp accepts", pairDisplayName)
	}
}
