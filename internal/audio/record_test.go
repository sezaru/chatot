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

func TestConcatList(t *testing.T) {
	got := concatList([]string{"/tmp/a.ogg", "/tmp/it's.ogg"})
	want := "file '/tmp/a.ogg'\nfile '/tmp/it'\\''s.ogg'\n"
	if got != want {
		t.Errorf("concatList = %q, want %q", got, want)
	}
}

func TestConcatArgs(t *testing.T) {
	got := concatArgs("/tmp/list.txt", "/tmp/out.ogg")
	// The concat demuxer must be told the list is trusted (-safe 0) or it
	// refuses absolute paths, and the join re-encodes rather than copies.
	if got[0] != "-y" || got[len(got)-1] != "/tmp/out.ogg" {
		t.Fatalf("concatArgs = %v", got)
	}
	joined := " " + join(got) + " "
	for _, frag := range []string{" -f concat ", " -safe 0 ", " -i /tmp/list.txt ", " -c:a libopus "} {
		if !contains(joined, frag) {
			t.Errorf("concatArgs missing %q in %v", frag, got)
		}
	}
}

func TestElapsedExcludesPauses(t *testing.T) {
	// A paused recorder reports only what its finished segments hold; the
	// clock must not keep running between Pause and Resume.
	r := &Recorder{captured: 4 * 1e9, segments: []string{"x"}}
	if got := r.Elapsed(); got != 4*1e9 {
		t.Errorf("paused Elapsed = %v, want 4s", got)
	}
	if !r.Paused() {
		t.Error("Paused() = false with a finished segment and no live ffmpeg")
	}
	if (&Recorder{}).Paused() {
		t.Error("a never-started recorder reports paused")
	}
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestParseLevelLine(t *testing.T) {
	cases := []struct {
		line string
		want float64
		ok   bool
	}{
		{"lavfi.astats.Overall.RMS_level=-25", 0.5, true},
		{"frame:12   pts:28800   pts_time:0.6\nlavfi.astats.Overall.RMS_level=0.0", 1, true},
		{"lavfi.astats.Overall.RMS_level=-inf", 0, true},
		{"lavfi.astats.Overall.RMS_level=-80", 0, true},
		{"size=      12kB time=00:00:01.02 bitrate=  96.0kbits/s", 0, false},
		{"lavfi.astats.Overall.RMS_level=nan", 0, false},
	}
	for _, c := range cases {
		got, ok := parseLevelLine(c.line)
		if ok != c.ok || (ok && (got < c.want-0.01 || got > c.want+0.01)) {
			t.Errorf("parseLevelLine(%q) = %v,%v want %v,%v", c.line, got, ok, c.want, c.ok)
		}
	}
}

func TestRecordArgsMeter(t *testing.T) {
	args := recordArgs("/tmp/x.ogg")
	found := false
	for i, a := range args {
		if a == "-af" && i+1 < len(args) && args[i+1] == levelFilter {
			found = true
		}
	}
	if !found {
		t.Errorf("recordArgs lacks the level meter filter: %v", args)
	}
}
