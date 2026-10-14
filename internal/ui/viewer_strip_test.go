package ui

import (
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// Stepping keeps the strip's tiles and only moves the selected mark. Each
// step used to rebuild every tile, decoding every picture with no embedded
// thumbnail from its file on the main loop: seconds per step in a chat of
// screenshots.
func TestAttachmentViewerStepKeepsStrip(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	const chat = "1@s.whatsapp.net"
	v := NewAttachmentViewer(client.NewFake(), func() {})
	v.Open(client.Chat{JID: chat}, viewerDocs(chat, 4), "d1")
	before := make([]*gtk.Button, len(v.tiles))
	for i, t := range v.tiles {
		before[i] = t.btn
	}

	v.step(1)
	for i, tile := range v.tiles {
		if tile.btn != before[i] {
			t.Fatalf("stepping rebuilt tile %d", i)
		}
		if on := tile.overlay.HasCSSClass("chatot-viewer-thumb-on"); on != (i == 2) {
			t.Errorf("tile %d selected = %v after stepping to 2", i, on)
		}
	}

	v.Remove(chat, "d0")
	if len(v.tiles) != 3 {
		t.Fatalf("strip has %d tiles after a removal, want 3", len(v.tiles))
	}
	if !v.tiles[1].overlay.HasCSSClass("chatot-viewer-thumb-on") {
		t.Error("the shown d2 lost its mark when the strip was rebuilt")
	}
}
