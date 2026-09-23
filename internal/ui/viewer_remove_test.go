package ui

import (
	"strconv"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

func viewerDocs(chat string, n int) []client.Message {
	msgs := make([]client.Message, n)
	for i := range msgs {
		id := "d" + strconv.Itoa(i)
		msgs[i] = client.Message{ID: id, ChatJID: chat, TS: int64(i + 1),
			Attachment: &client.Attachment{Kind: "document", Filename: id + ".txt", MimeType: "text/plain"}}
	}
	return msgs
}

// A message deleted while the viewer shows it leaves the viewer: the stage
// moves on, the strip and the counter shrink, and the last one gone takes
// the viewer back to the chat. It used to stay on show.
func TestAttachmentViewerRemove(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	const chat = "1@s.whatsapp.net"
	backs := 0
	v := NewAttachmentViewer(client.NewFake(), func() { backs++ })
	v.Open(client.Chat{JID: chat}, viewerDocs(chat, 3), "d1")

	v.Remove("other@s.whatsapp.net", "d1")
	if m, _ := v.Current(); m.ID != "d1" {
		t.Fatalf("a revoke in another chat moved the viewer to %q", m.ID)
	}

	v.Remove(chat, "d1")
	if m, _ := v.Current(); m.ID != "d2" {
		t.Errorf("after deleting the shown d1, current = %q, want its successor d2", m.ID)
	}
	if got := v.counter.Label(); got != "2 / 2" {
		t.Errorf("counter = %q, want 2 / 2", got)
	}

	v.Remove(chat, "d0")
	if m, _ := v.Current(); m.ID != "d2" {
		t.Errorf("deleting another tile moved the stage to %q", m.ID)
	}
	if got := v.counter.Label(); got != "" {
		t.Errorf("counter = %q with one left, want none", got)
	}

	v.Remove(chat, "d2")
	if _, ok := v.Current(); ok {
		t.Error("the viewer still has something to show after its last item went")
	}
}

// The chevron on an album is the whole album's: Forward hands over every
// picture in it, not only the first.
func TestAlbumMenuForwardsTheRun(t *testing.T) {
	run := []client.Message{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	var got []client.Message
	h := bubbleHooks{onForward: func(msgs ...client.Message) { got = msgs }}
	activate := func(items []menuItem) {
		t.Helper()
		for _, it := range items {
			if it.Label == "Forward" && it.OnActivate != nil {
				it.OnActivate()
				return
			}
		}
		t.Fatal("no Forward row")
	}

	activate(h.runMenuItems(run[0], run, false, true))
	if len(got) != 3 || got[0].ID != "a" || got[2].ID != "c" {
		t.Errorf("album Forward sent %v, want a, b, c", got)
	}
	activate(h.menuItemsFor(run[1], false, true))
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("single Forward sent %v, want b", got)
	}
}
