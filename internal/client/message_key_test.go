package client

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestMessageSenderJID(t *testing.T) {
	own := "5551234@s.whatsapp.net"
	direct := types.NewJID("1234567890", types.DefaultUserServer)
	group := types.NewJID("123-456", types.GroupServer)

	// An own message keys on our JID whatever the chat.
	got, err := messageSenderJID(direct, true, "", own)
	if err != nil || got.User != "5551234" {
		t.Errorf("own = (%v, %v)", got, err)
	}
	// A received direct message keys on the chat itself: a zero sender would
	// make whatsmeow's BuildMessageKey mark the key FromMe.
	got, err = messageSenderJID(direct, false, "1234567890@s.whatsapp.net", own)
	if err != nil || got != direct {
		t.Errorf("received direct = (%v, %v), want %v", got, err, direct)
	}
	// A received group message keys on the participant.
	got, err = messageSenderJID(group, false, "9876@s.whatsapp.net", own)
	if err != nil || got.User != "9876" {
		t.Errorf("group = (%v, %v)", got, err)
	}
	if _, err := messageSenderJID(group, false, "", own); err == nil {
		t.Error("group message with no participant should error")
	}
	if _, err := messageSenderJID(direct, true, "", ""); err == nil {
		t.Error("own message with no own jid (logged out) should error")
	}
}
