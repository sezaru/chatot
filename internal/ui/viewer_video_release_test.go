package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// Stepping past a clip, or leaving the viewer, lets go of its player. Each
// step used to only pause it, and the stage, stream and player held each
// other alive, so every clip visited kept a GStreamer pipeline: a run
// through an album of videos ate the machine's memory.
func TestAttachmentViewerReleasesClipPlayers(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	const chat = "1@s.whatsapp.net"
	dir := t.TempDir()
	msgs := make([]client.Message, 3)
	for i := range msgs {
		id := "v" + strconv.Itoa(i)
		path := filepath.Join(dir, id+".mp4")
		if err := os.WriteFile(path, []byte("not really a clip"), 0o644); err != nil {
			t.Fatal(err)
		}
		msgs[i] = client.Message{ID: id, ChatJID: chat, TS: int64(i + 1),
			Attachment: &client.Attachment{Kind: "video", MimeType: "video/mp4", LocalPath: path, DurationSecs: 5}}
	}
	v := NewAttachmentViewer(client.NewFake(), func() {})
	v.Open(client.Chat{JID: chat}, msgs, "v0")

	released := func(p *mediaPlayer, s *videoStage) {
		t.Helper()
		if p.stream != nil || len(p.watchers) != 0 {
			t.Errorf("player kept its stream (%v) or %d watchers", p.stream != nil, len(p.watchers))
		}
		if s.pic.Paintable() != nil {
			t.Error("stage still paints the old stream")
		}
	}

	for i := 1; i < len(msgs); i++ {
		p, s := v.player, v.video
		if p == nil || s == nil || p.stream == nil {
			t.Fatalf("clip %d has no player, stage or stream", i-1)
		}
		v.step(1)
		released(p, s)
		if v.player == p {
			t.Fatalf("step %d kept the same player", i)
		}
	}

	p, s := v.player, v.video
	v.Close()
	released(p, s)
	if v.player != nil || v.video != nil {
		t.Error("Close kept the viewer's player or stage")
	}
}

// The status viewer frees a clip update's player when it moves on from it,
// for the same reason as the attachment viewer.
func TestStatusPaneReleasesClipPlayer(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	path := filepath.Join(t.TempDir(), "s.mp4")
	if err := os.WriteFile(path, []byte("not really a clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &StatusPane{hidden: true}
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	placeholder := gtk.NewLabel("")
	card.Append(placeholder)
	item := client.Message{ID: "s1", Attachment: &client.Attachment{Kind: "video", MimeType: "video/mp4", DurationSecs: 5}}
	p.showStatusMedia(item, path, card, placeholder)

	player, stage := p.video, p.videoStage
	if player == nil || stage == nil || player.stream == nil {
		t.Fatal("the clip has no player, stage or stream")
	}
	p.detachVideo()
	if player.stream != nil || len(player.watchers) != 0 {
		t.Errorf("player kept its stream (%v) or %d watchers", player.stream != nil, len(player.watchers))
	}
	if stage.pic.Paintable() != nil {
		t.Error("stage still paints the old stream")
	}
	if p.video != nil || p.videoStage != nil {
		t.Error("the pane kept the clip")
	}
}
