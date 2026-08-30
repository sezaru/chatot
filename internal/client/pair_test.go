package client

import "testing"

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
