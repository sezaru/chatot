package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestViewerKindSkipsVoiceNotes(t *testing.T) {
	note := client.Message{Attachment: &client.Attachment{Kind: "audio", Voice: true}}
	song := client.Message{Attachment: &client.Attachment{Kind: "audio"}}
	if k := viewerKind(note); k != "" {
		t.Errorf("voice note kind = %q, want it out of the viewer", k)
	}
	if k := viewerKind(song); k != "audio" {
		t.Errorf("audio file kind = %q, want audio", k)
	}
}

func TestViewerKindAnimatesGIFPictures(t *testing.T) {
	gifPic := client.Message{Attachment: &client.Attachment{Kind: "image", MimeType: "image/gif"}}
	photo := client.Message{Attachment: &client.Attachment{Kind: "image", MimeType: "image/jpeg"}}
	if k := viewerKind(gifPic); k != "gif" {
		t.Errorf("GIF picture kind = %q, want gif", k)
	}
	if k := viewerKind(photo); k != "photo" {
		t.Errorf("JPEG kind = %q, want photo", k)
	}
	for kind, want := range map[string]bool{"photo": true, "video": true, "gif": true, "pdf": false, "file": false, "audio": false} {
		if got := viewerAutoFetch(kind); got != want {
			t.Errorf("viewerAutoFetch(%q) = %v, want %v", kind, got, want)
		}
	}
}
