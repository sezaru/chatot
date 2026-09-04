package ui

import (
	"testing"

	"chatot/internal/client"
)

// The sidebar rebuilds on the events that can change it and ignores the
// chatter that can't: presence and reactions arrive constantly on a live
// account, and each refresh costs a store query.
func TestSidebarRefreshEvent(t *testing.T) {
	yes := []client.EventKind{
		client.EventMessage, client.EventReceipt, client.EventRevoke, client.EventChatUpdate,
		client.EventHistorySync, client.EventLabelUpdate, client.EventConnection,
		client.EventPairSuccess, client.EventLoggedOut,
	}
	no := []client.EventKind{
		client.EventPresence, client.EventChatPresence, client.EventReaction,
		client.EventPollVote, client.EventCall, client.EventQR, client.EventAvatar,
		client.EventNewsletterUpdate,
	}
	for _, k := range yes {
		if !sidebarRefreshEvent(k) {
			t.Errorf("kind %d should refresh the sidebar", k)
		}
	}
	for _, k := range no {
		if sidebarRefreshEvent(k) {
			t.Errorf("kind %d should not refresh the sidebar", k)
		}
	}
}
