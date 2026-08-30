package audio

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRecordArgs(t *testing.T) {
	args := recordArgs("/tmp/out.ogg")

	want := map[string]bool{"libopus": false, "mono": false}
	for i, a := range args {
		switch a {
		case "libopus":
			want["libopus"] = true
		case "-ac":
			if i+1 < len(args) && args[i+1] == "1" {
				want["mono"] = true
			}
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("recordArgs missing expected flag for %q: %v", k, args)
		}
	}

	last := args[len(args)-1]
	if last != "/tmp/out.ogg" || !strings.HasSuffix(last, ".ogg") {
		t.Errorf("recordArgs last element = %q, want ogg output path last", last)
	}
}

func TestRecordingOK(t *testing.T) {
	sigErr := errors.New("exit status 255")
	cases := []struct {
		name    string
		bytes   []byte
		waitErr error
		wantErr bool
	}{
		{"non-empty with signal error is ok", []byte("OggS..."), sigErr, false},
		{"non-empty clean exit is ok", []byte("OggS..."), nil, false},
		{"empty with error fails", nil, sigErr, true},
		{"empty with clean exit fails", nil, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := recordingOK(tc.bytes, tc.waitErr)
			if (err != nil) != tc.wantErr {
				t.Errorf("recordingOK(%d bytes, %v) err = %v, wantErr %v", len(tc.bytes), tc.waitErr, err, tc.wantErr)
			}
		})
	}
}

func TestDurationSeconds(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int
	}{
		{"equal times", base, base, 0},
		{"3.4s truncates to 3", base, base.Add(3400 * time.Millisecond), 3},
		{"end before start clamps to 0", base, base.Add(-5 * time.Second), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := durationSeconds(tc.start, tc.end); got != tc.want {
				t.Errorf("durationSeconds(%v, %v) = %d, want %d", tc.start, tc.end, got, tc.want)
			}
		})
	}
}
