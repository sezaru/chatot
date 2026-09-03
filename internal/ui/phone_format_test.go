package ui

import "testing"

func TestFormatPhoneDisplay(t *testing.T) {
	cases := map[string]string{
		"554899010873":     "+55 48 9901-0873",
		"5548988073648":    "+55 48 98807-3648",
		"+14155550132":     "+1 415 555 0132",
		"351912004118":     "+351 912 004 118",
		"447911123456":     "+44 7911 123456",
		"33612345678":      "+33 6 12 34 56 78",
		"99812345678901":   "+99 812 345 678 901",
		"Nina":             "Nina",
		"+55 48 9901-0873": "+55 48 9901-0873",
	}
	for in, want := range cases {
		if got := formatPhoneDisplay(in); got != want {
			t.Errorf("formatPhoneDisplay(%q) = %q, want %q", in, got, want)
		}
	}
}
