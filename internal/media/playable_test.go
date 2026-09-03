package media

import "testing"

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
