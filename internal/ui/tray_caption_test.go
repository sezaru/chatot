package ui

import "testing"

func TestTakesCaptionRefusesAudioFiles(t *testing.T) {
	for path, want := range map[string]bool{
		"/tmp/song.mp3":  false,
		"/tmp/memo.ogg":  false,
		"/tmp/photo.jpg": true,
		"/tmp/clip.mp4":  true,
		"/tmp/notes.pdf": true,
	} {
		if got := takesCaption(path); got != want {
			t.Errorf("takesCaption(%q) = %v, want %v", path, got, want)
		}
	}
}
