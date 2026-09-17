package ui

import (
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// TestLocalRecorderReportsMessageID pins the bubble → view hand-off: the
// recorder carries the message ID the hook was built for, and stays nil
// when the view is not listening (so downloadAndSwap can skip the call).
func TestLocalRecorderReportsMessageID(t *testing.T) {
	var gotID, gotPath string
	h := bubbleHooks{onLocalPath: func(id, path string) { gotID, gotPath = id, path }}
	rec := h.localRecorder("m11")
	if rec == nil {
		t.Fatal("recorder is nil with a listener set")
	}
	rec("/tmp/lease-2026.pdf")
	if gotID != "m11" || gotPath != "/tmp/lease-2026.pdf" {
		t.Fatalf("recorded (%q, %q), want (m11, /tmp/lease-2026.pdf)", gotID, gotPath)
	}
	if (bubbleHooks{}).localRecorder("m11") != nil {
		t.Fatal("recorder should be nil without a listener")
	}
}

// TestBubbleDownloadReportsLocalPath: the bubble's own tap-to-download
// must tell the view where the file landed, not just flip its own row to
// "View" — otherwise the viewer, opened later from the thread's messages,
// shows the not-downloaded stage for a document that is already on disk.
func TestBubbleDownloadReportsLocalPath(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	c := client.NewFake()
	msgs, err := c.Messages("1112223333@s.whatsapp.net", 100)
	if err != nil {
		t.Fatal(err)
	}
	var msg client.Message
	for _, m := range msgs {
		if m.ID == "m11" {
			msg = m
		}
	}
	if msg.Attachment == nil || msg.Attachment.LocalPath != "" {
		t.Fatalf("fixture m11 should be a not-downloaded document, got %+v", msg.Attachment)
	}
	mv := mediaVM(msg)
	got := ""
	mv.onLocal = func(path string) { got = path }

	slot := gtk.NewBox(gtk.OrientationVertical, 0)
	current := gtk.NewLabel("placeholder")
	slot.Append(current)
	downloadAndSwap(&mv, msg, c, slot, current, gtk.NewLabel("⬇"), func(string) {})

	deadline := time.Now().Add(5 * time.Second)
	for got == "" && time.Now().Before(deadline) {
		glib.MainContextDefault().Iteration(false)
		time.Sleep(5 * time.Millisecond)
	}
	if got == "" {
		t.Fatal("bubble download never reported its local path")
	}
	if !mv.HasLocal || mv.LocalPath != got {
		t.Fatalf("row state HasLocal=%v LocalPath=%q, reported %q", mv.HasLocal, mv.LocalPath, got)
	}
}
