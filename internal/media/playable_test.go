package media

import (
	"strings"
	"testing"
)

func TestNeedsTranscode(t *testing.T) {
	cases := []struct {
		path, mime string
		want       bool
	}{
		{"song.mp3", "", true},
		{"song.MP3", "", true},
		{"blob", "audio/mpeg", true},
		{"blob", "audio/mpeg; codecs=mp3", true},
		{"voice.ogg", "audio/ogg; codecs=opus", false},
		{"tone.flac", "", false},
		{"blob", "audio/mp4", false},
	}
	for _, c := range cases {
		if got := NeedsTranscode(c.path, c.mime); got != c.want {
			t.Errorf("NeedsTranscode(%q, %q) = %v, want %v", c.path, c.mime, got, c.want)
		}
	}
}

func TestParsePDFPages(t *testing.T) {
	out := "Title:          x\nPages:          8\nEncrypted:      no\n"
	if got := parsePDFPages(out); got != 8 {
		t.Fatalf("parsePDFPages = %d, want 8", got)
	}
	if got := parsePDFPages("garbage"); got != 0 {
		t.Fatalf("parsePDFPages(garbage) = %d, want 0", got)
	}
}

func TestSpeedCacheName(t *testing.T) {
	base := speedCacheName("/tmp/a.ogg", 10, 20, 1)
	if !strings.HasSuffix(base, ".flac") || strings.Contains(base, "-x") {
		t.Fatalf("1× name = %q, want a plain .flac", base)
	}
	fast := speedCacheName("/tmp/a.ogg", 10, 20, 1.5)
	if !strings.HasSuffix(fast, "-x1.5.flac") {
		t.Fatalf("1.5× name = %q, want the -x1.5 suffix", fast)
	}
	if fast[:16] != base[:16] {
		t.Fatalf("speeds of one file should share the source key: %q vs %q", base, fast)
	}
	if speedCacheName("/tmp/a.ogg", 11, 20, 1) == base {
		t.Fatal("a changed size should change the name")
	}
}
