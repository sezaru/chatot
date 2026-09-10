package ui

import (
	"errors"
	"testing"

	"chatot/internal/client"
)

// The header's subline is the number when there is one and the status
// otherwise, which is what a signed-out account falls back to. The red is
// keyed to "has to be scanned again", not to "not connected": an account
// between sockets is fine and must stay neutral.
func TestAccountHeaderSubline(t *testing.T) {
	tests := []struct {
		name string
		meta client.AccountMeta
		text string
		mono bool
		bad  bool
	}{
		{
			name: "linked account shows its number in mono",
			meta: client.AccountMeta{Phone: "+351912345678", Status: "Connected"},
			text: "+351912345678", mono: true, bad: false,
		},
		{
			name: "reconnecting account keeps its number and stays neutral",
			meta: client.AccountMeta{Phone: "+351912345678", Status: "Reconnecting…"},
			text: "+351912345678", mono: true, bad: false,
		},
		{
			name: "signed-out account shows the prompt in red",
			meta: client.AccountMeta{Status: "Logged out · scan to relink", NeedsRelink: true},
			text: "Logged out · scan to relink", mono: false, bad: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, mono, bad := accountHeaderSubline(tc.meta)
			if text != tc.text || mono != tc.mono || bad != tc.bad {
				t.Errorf("accountHeaderSubline() = (%q, %v, %v), want (%q, %v, %v)",
					text, mono, bad, tc.text, tc.mono, tc.bad)
			}
		})
	}
}

// The ⋮ popover is rebuilt on the first pass and then only when the link
// state its last row depends on moves. Rebuilding on every connection event
// would drop the popover out from under an open menu.
func TestAppMenuNeedsRebuild(t *testing.T) {
	if !appMenuNeedsRebuild(false, false, false) {
		t.Error("the menu must be built the first time, whatever the state")
	}
	if !appMenuNeedsRebuild(true, true, false) {
		t.Error("a link that dropped must rebuild the menu: Unlink has to become Relink")
	}
	if !appMenuNeedsRebuild(true, false, true) {
		t.Error("a fresh link must rebuild the menu: Relink has to become Unlink")
	}
	if appMenuNeedsRebuild(true, true, true) {
		t.Error("an unchanged link state must not rebuild the menu")
	}
}

// The Relink card explains why it has no code rather than leaving the
// placeholder up, which is what the card used to do forever.
func TestRelinkStatusMessage(t *testing.T) {
	if got := relinkStatusMessage(nil); got != "" {
		t.Errorf("relinkStatusMessage(nil) = %q, want empty", got)
	}
	got := relinkStatusMessage(client.ErrStillLinked)
	if got != "This account is already linked. Unlink it first to pair a new session." {
		t.Errorf("a still-linked account should be told to unlink first, got %q", got)
	}
	got = relinkStatusMessage(errors.New("boom"))
	if got != "Couldn't start pairing: boom" {
		t.Errorf("relinkStatusMessage(other) = %q", got)
	}
}
