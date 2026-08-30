package client

import "testing"

func TestNextLabelID(t *testing.T) {
	cases := []struct {
		name     string
		existing []string
		want     string
	}{
		{"empty", nil, "1"},
		{"sequential", []string{"1", "2"}, "3"},
		{"with gaps", []string{"1", "3"}, "4"},
		{"unordered", []string{"5", "2", "3"}, "6"},
		{"non-numeric ignored", []string{"abc", "2"}, "3"},
		{"all non-numeric", []string{"x", "y"}, "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextLabelID(tc.existing); got != tc.want {
				t.Fatalf("nextLabelID(%v) = %q, want %q", tc.existing, got, tc.want)
			}
		})
	}
}
