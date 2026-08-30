package client

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/types"

	"chatot/internal/store"
)

// GroupInfo fetches jid's current name/topic/membership from WhatsApp and
// persists it to the local store (best-effort — a persist failure doesn't
// fail the call, since the caller already has fresh data to show).
func (w *Whatsmeow) GroupInfo(ctx context.Context, jid string) (*GroupInfo, error) {
	j, err := types.ParseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	info, err := w.wa.GetGroupInfo(ctx, j)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: get group info: %w", err)
	}
	gi := groupInfoFromWhatsmeow(info)
	w.persistGroupInfo(gi)
	return gi, nil
}

func groupInfoFromWhatsmeow(info *types.GroupInfo) *GroupInfo {
	parts := make([]GroupParticipant, len(info.Participants))
	for i, p := range info.Participants {
		parts[i] = GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		}
	}
	return &GroupInfo{
		JID:          info.JID.String(),
		Name:         info.Name,
		Topic:        info.Topic,
		OwnerJID:     info.OwnerJID.String(),
		Participants: parts,
	}
}

// persistGroupInfo saves gi's metadata and membership to the store,
// warning (not failing) on error.
func (w *Whatsmeow) persistGroupInfo(gi *GroupInfo) {
	if err := w.store.UpsertGroup(store.GroupRow{JID: gi.JID, Name: gi.Name, Topic: gi.Topic}); err != nil {
		w.log.Warnf("chatot/client: persist group %s: %v", gi.JID, err)
	}
	parts := make([]store.GroupParticipant, len(gi.Participants))
	for i, p := range gi.Participants {
		parts[i] = store.GroupParticipant{JID: p.JID, IsAdmin: p.IsAdmin, IsSuperAdmin: p.IsSuperAdmin}
	}
	if err := w.store.SetGroupParticipants(gi.JID, parts); err != nil {
		w.log.Warnf("chatot/client: persist group participants %s: %v", gi.JID, err)
	}
}

// refreshGroupInfo re-fetches jid's group info in the background and pushes
// an EventChatUpdate so any open chat list / group-info panel refreshes.
// Called on any inbound *events.GroupInfo, which fires on membership or
// metadata changes but only carries deltas — a full re-fetch is simpler and
// robust than reassembling state from those deltas.
func (w *Whatsmeow) refreshGroupInfo(jid string) {
	go func() {
		if _, err := w.GroupInfo(context.Background(), jid); err != nil {
			w.log.Warnf("chatot/client: refresh group info for %s: %v", jid, err)
			return
		}
		w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	}()
}
