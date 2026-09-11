package client

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waVnameCert"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"chatot/internal/store"
)

// A message this device sent is stored and announced as a chat update, so
// the chat list re-sorts right away rather than on the next receipt.
func TestIngestSentPublishesChatUpdate(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := &Whatsmeow{log: waLog.Noop, store: s, events: newEventBus(nil)}
	events := w.Events()

	msg := &Message{ID: "m1", ChatJID: "1@s.whatsapp.net", FromMe: true, Text: "hi", TS: 10}
	if err := w.ingestSent(msg); err != nil {
		t.Fatal(err)
	}
	if _, known, _ := s.MessageByID("1@s.whatsapp.net", "m1"); !known {
		t.Fatal("message not stored")
	}
	select {
	case ev := <-events:
		if ev.Kind != EventChatUpdate || ev.ChatUpdate == nil || ev.ChatUpdate.JID != "1@s.whatsapp.net" {
			t.Fatalf("event = %+v, want a ChatUpdate for the chat", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event published for the sent message")
	}
}

func TestVerifiedNameOf(t *testing.T) {
	if got := verifiedNameOf(nil); got != "" {
		t.Errorf("nil = %q", got)
	}
	if got := verifiedNameOf(&types.VerifiedName{}); got != "" {
		t.Errorf("no details = %q", got)
	}
	v := &types.VerifiedName{Details: &waVnameCert.VerifiedNameCertificate_Details{VerifiedName: proto.String(" Vita Essência ")}}
	if got := verifiedNameOf(v); got != "Vita Essência" {
		t.Errorf("verified = %q", got)
	}
}
