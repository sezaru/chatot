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
