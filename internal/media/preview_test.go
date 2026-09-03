package media

import "testing"

func TestParseProbe(t *testing.T) {
	got := parseProbe("1280,720\n6.550000\n")
	if got.Width != 1280 || got.Height != 720 || got.Seconds != 7 {
		t.Errorf("parseProbe = %+v, want 1280x720, 7s", got)
	}
	// ffprobe may print the format line first; the parser must not care.
	got = parseProbe("12.2\n640,480")
	if got.Width != 640 || got.Seconds != 12 {
		t.Errorf("parseProbe (reversed) = %+v", got)
	}
	if got := parseProbe(""); got != (VideoInfo{}) {
		t.Errorf("parseProbe(\"\") = %+v, want zero", got)
	}
}

func TestScaleFilterNeverUpscales(t *testing.T) {
	f := scaleFilter()
	for _, want := range []string{"min(512,iw)", "min(512,ih)", "force_original_aspect_ratio=decrease"} {
		if !contains(f, want) {
			t.Errorf("scaleFilter %q lacks %q", f, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
