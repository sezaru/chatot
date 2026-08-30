package client

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestHistorySender(t *testing.T) {
	chat := types.NewJID("1112223333", types.DefaultUserServer)
	own := types.NewJID("1234567890", types.DefaultUserServer)
	other := types.NewJID("9998887777", types.DefaultUserServer)

	tests := []struct {
		name    string
		fromMe  bool
		fromJID string
		want    types.JID
	}{
		{"own message", true, "", own},
		{"own message ignores stale fromJID", true, other.String(), own},
		{"other message with known sender", false, other.String(), other},
		{"DM with unrecorded sender falls back to chat", false, "", chat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := historySender(chat, own, tt.fromMe, tt.fromJID)
			if got != tt.want {
				t.Errorf("historySender(%v, %v, %v) = %v, want %v", tt.fromMe, tt.fromJID, chat, got, tt.want)
			}
		})
	}
}
