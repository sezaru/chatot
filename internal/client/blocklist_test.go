package client

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
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

// The list is kept with the server's hash of it. Changes made to that very
// version keep it current under the new hash; changes to any other version
// drop the hash, so the next fetch asks for the whole list again.
func TestBlocklistHashFollowsChanges(t *testing.T) {
	w := &Whatsmeow{}
	pn := types.NewJID("5511999990000", types.DefaultUserServer)
	lid := types.NewJID("123456789", types.HiddenUserServer)
	other := types.NewJID("987654321", types.HiddenUserServer)
	w.setBlockedCache(&types.Blocklist{DHash: "h1", Items: []types.BlocklistItem{{LID: lid, PN: pn, Active: true}}})
	if w.blockHash != "h1" {
		t.Fatalf("hash after a fetch = %q, want h1", w.blockHash)
	}

	w.applyBlocklistEvent(&events.Blocklist{PrevDHash: "h1", DHash: "h2", Changes: []events.BlocklistChange{
		{JID: pn, Action: events.BlocklistChangeActionUnblock},
		{JID: other, Action: events.BlocklistChangeActionBlock},
	}})
	if w.blockHash != "h2" {
		t.Errorf("hash after a change to our version = %q, want h2", w.blockHash)
	}
	if got := w.blockedChats(w.blockItems); len(got) != 1 || got[0] != other.String() {
		t.Errorf("list after the change = %v, want only %s", got, other)
	}
	if w.IsBlocked(pn.String()) || !w.IsBlocked(other.String()) {
		t.Error("the blocked set did not follow the change")
	}

	w.applyBlocklistEvent(&events.Blocklist{PrevDHash: "h7", DHash: "h8", Changes: []events.BlocklistChange{
		{JID: lid, Action: events.BlocklistChangeActionBlock},
	}})
	if w.blockHash != "" {
		t.Errorf("hash after a change to another version = %q, want none", w.blockHash)
	}
}
