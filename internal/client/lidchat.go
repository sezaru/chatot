package client

import (
	"context"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow/types"
)

// canonicalChatJID is the chat a JID's messages are filed under: a
// LID-addressed DM lands in its phone-number chat whenever the mapping is
// known, so one person is one chat however WhatsApp addressed them. Groups,
// channels, phone-number DMs and LIDs with no known number pass through.
func (w *Whatsmeow) canonicalChatJID(jid string) string {
	return canonicalChat(jid, w.pnForLID)
}

// pnForLID resolves a LID to its phone number: this account's own LID from
// the device record, everyone else from whatsmeow's LID map (filled from
// each message's alt address before the event is dispatched) and then from
// the mappings history sync wrote into the contacts table.
func (w *Whatsmeow) pnForLID(lid types.JID) types.JID {
	if w.wa != nil && w.wa.Store != nil {
		if own := w.wa.Store.LID; !own.IsEmpty() && own.User == lid.User && w.wa.Store.ID != nil {
			return w.wa.Store.ID.ToNonAD()
		}
		if w.wa.Store.LIDs != nil {
			if pn, err := w.wa.Store.LIDs.GetPNForLID(context.Background(), lid); err == nil && !pn.IsEmpty() {
				return pn
			}
		}
	}
	if w.store != nil {
		if pn, err := w.store.ContactPNJID(lid.String()); err == nil && pn != "" {
			if parsed, err := types.ParseJID(pn); err == nil {
				return parsed
			}
		}
	}
	return types.EmptyJID
}

// canonicalChat is canonicalChatJID with the LID lookup injected.
func canonicalChat(jid string, pnForLID func(types.JID) types.JID) string {
	parsed, err := types.ParseJID(jid)
	if err != nil || parsed.Server != types.HiddenUserServer {
		return jid
	}
	pn := pnForLID(parsed.ToNonAD())
	if pn.IsEmpty() {
		return jid
	}
	return pn.ToNonAD().String()
}

// canonicalizeEvent points every chat reference in e at the canonical chat.
func (w *Whatsmeow) canonicalizeEvent(e *Event) {
	if e == nil {
		return
	}
	if m := e.Message; m != nil {
		m.ChatJID = w.canonicalChatJID(m.ChatJID)
		if m.ReplyTo != nil {
			m.ReplyTo.ChatJID = w.canonicalChatJID(m.ReplyTo.ChatJID)
		}
	}
	if r := e.Receipt; r != nil {
		r.ChatJID = w.canonicalChatJID(r.ChatJID)
	}
	if r := e.Reaction; r != nil {
		r.ChatJID = w.canonicalChatJID(r.ChatJID)
	}
	if r := e.Revoke; r != nil {
		r.ChatJID = w.canonicalChatJID(r.ChatJID)
	}
	if p := e.ChatPresence; p != nil {
		p.ChatJID = w.canonicalChatJID(p.ChatJID)
	}
	if p := e.PollVote; p != nil {
		p.ChatJID = w.canonicalChatJID(p.ChatJID)
	}
	if u := e.ChatUpdate; u != nil {
		u.JID = w.canonicalChatJID(u.JID)
	}
	if c := e.Call; c != nil {
		c.ChatJID = w.canonicalChatJID(c.ChatJID)
	}
	if h := e.HistorySync; h != nil {
		for i, jid := range h.ChatJIDs {
			h.ChatJIDs[i] = w.canonicalChatJID(jid)
		}
	}
}

// mergeChatInto folds a LID chat into its phone-number chat once the
// mapping is known; a no-op when nothing was filed under the LID.
func (w *Whatsmeow) mergeChatInto(lid, pn string) {
	if w.store == nil || lid == "" || pn == "" || lid == pn {
		return
	}
	w.lidMu.Lock()
	defer w.lidMu.Unlock()
	merged, err := w.store.MergeChat(lid, pn)
	if err != nil {
		w.log.Warnf("chatot/client: merge chat %s into %s: %v", lid, pn, err)
		return
	}
	if merged {
		w.adoptAvatar(lid, pn)
		w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: pn}})
	}
}

// adoptAvatar hands a LID's cached picture to its phone number, so the
// merged chat keeps its avatar without a fetch (and offline).
func (w *Whatsmeow) adoptAvatar(lid, pn string) {
	if w.avatarDir == "" {
		return
	}
	from := filepath.Join(w.avatarDir, avatarCacheName(lid))
	to := filepath.Join(w.avatarDir, avatarCacheName(pn))
	if _, err := os.Stat(to); err == nil {
		return
	}
	if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
		w.log.Warnf("chatot/client: move avatar %s to %s: %v", lid, pn, err)
	}
}

// mergeLIDChats folds every LID chat whose number is known by now into its
// phone-number chat, so a store written before messages were filed
// canonically shows one chat per person.
func (w *Whatsmeow) mergeLIDChats() {
	if w.store == nil {
		return
	}
	w.lidMu.Lock()
	defer w.lidMu.Unlock()
	jids, err := w.store.LIDChatJIDs()
	if err != nil {
		w.log.Warnf("chatot/client: list lid chats: %v", err)
		return
	}
	for _, jid := range jids {
		if pn := w.canonicalChatJID(jid); pn != jid {
			if _, err := w.store.MergeChat(jid, pn); err != nil {
				w.log.Warnf("chatot/client: merge chat %s into %s: %v", jid, pn, err)
				continue
			}
			w.adoptAvatar(jid, pn)
		}
	}
}

// repairUnreadOnce caps stale unread counts a single time per store: the
// counts history sync wrote for number chats were never cleared while reads
// landed on the LID twin, and the merge brought them to the top of the list.
func (w *Whatsmeow) repairUnreadOnce() {
	if w.store == nil {
		return
	}
	if done, err := w.store.Meta(unreadRepairKey); err != nil || done != "" {
		return
	}
	n, err := w.store.RepairUnreadCounts()
	if err != nil {
		w.log.Warnf("chatot/client: repair unread counts: %v", err)
		return
	}
	if n > 0 {
		w.log.Infof("chatot/client: capped stale unread counts on %d chats", n)
	}
	if err := w.store.SetMeta(unreadRepairKey, "1"); err != nil {
		w.log.Warnf("chatot/client: record unread repair: %v", err)
	}
}

// groupSenderRepairKey marks a store whose group messages were checked once
// for a sender equal to the group (see store.RepairGroupSenders).
const groupSenderRepairKey = "group_sender_repair_v1"

// repairGroupSendersOnce blanks, a single time per store, the group
// senders an earlier history sync misfiled as the group itself.
func (w *Whatsmeow) repairGroupSendersOnce() {
	if w.store == nil {
		return
	}
	if done, err := w.store.Meta(groupSenderRepairKey); err != nil || done != "" {
		return
	}
	n, err := w.store.RepairGroupSenders()
	if err != nil {
		w.log.Warnf("chatot/client: repair group senders: %v", err)
		return
	}
	if n > 0 {
		w.log.Infof("chatot/client: blanked the misfiled sender on %d group messages", n)
	}
	if err := w.store.SetMeta(groupSenderRepairKey, "1"); err != nil {
		w.log.Warnf("chatot/client: record group sender repair: %v", err)
	}
}
