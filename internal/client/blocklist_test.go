package client

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// WhatsApp lists blocked people by LID, with the phone number when it has
// one: a chat filed under either is blocked, and the list names the chat.
func TestBlockedCacheTakesEveryForm(t *testing.T) {
	w := &Whatsmeow{}
	pn := types.NewJID("5511999990000", types.DefaultUserServer)
	lid := types.NewJID("123456789", types.HiddenUserServer)
	bare := types.NewJID("987654321", types.HiddenUserServer)
	w.setBlockedCache(&types.Blocklist{Items: []types.BlocklistItem{
		{LID: lid, PN: pn, Active: true},
		{LID: bare, Active: true},
	}})
	for _, jid := range []types.JID{pn, lid, bare} {
		if !w.IsBlocked(jid.String()) {
			t.Errorf("%s is not blocked", jid)
		}
	}
	if w.IsBlocked("5511888880000@s.whatsapp.net") {
		t.Error("someone never blocked reads as blocked")
	}
	if got := w.blockedChatJID(types.BlocklistItem{LID: lid, PN: pn}); got != pn.String() {
		t.Errorf("blocked chat = %q, want the phone number %q", got, pn)
	}
	if got := w.blockedChatJID(types.BlocklistItem{LID: bare}); got != bare.String() {
		t.Errorf("blocked chat with no number = %q, want the LID %q", got, bare)
	}
}
