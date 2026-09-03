package client

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ingestEvent routes a normalized Event onto the local store. It runs
// synchronously in the whatsmeow dispatch goroutine (same as pushEvent), so
// it must not block; sqlite writes here are fast local calls.
//
// This mapping lives in package client, not internal/store, so that store
// stays a pure, whatsmeow/client-agnostic package: store never imports
// client, only client imports store. Reversing that (store importing client
// for its Chat/Message value types) would create an import cycle, since
// whatsmeow.go — which must call into store — lives in this same package.
func (w *Whatsmeow) ingestEvent(e Event) {
	if w.store == nil {
		return
	}
	var err error
	switch e.Kind {
	case EventMessage:
		err = w.ingestMessage(e.Message)
	case EventReceipt:
		err = w.ingestReceipt(e.Receipt)
	case EventReaction:
		err = w.ingestReaction(e.Reaction)
	case EventRevoke:
		err = w.ingestRevoke(e.Revoke)
	}
	if err != nil {
		w.log.Warnf("store ingest failed (kind=%d): %v", e.Kind, err)
	}
}

func (w *Whatsmeow) ingestMessage(m *Message) error {
	if m == nil {
		return nil
	}
	isGroup := strings.HasSuffix(m.ChatJID, "@g.us")
	unreadDelta := 0
	if !m.FromMe && !m.Edited {
		unreadDelta = 1
	}
	// A channel post is filed under its JID for the Channels tab (like
	// persistNewsletterPost) but is not a chat: no row, no unread count.
	if !strings.HasSuffix(m.ChatJID, "@newsletter") {
		if err := w.store.BumpChatActivity(m.ChatJID, isGroup, m.TS, unreadDelta); err != nil {
			return err
		}
	}
	row := storeMessageRow(m)
	if err := w.store.UpsertMessage(row); err != nil {
		return err
	}
	if m.Attachment != nil {
		return w.store.UpsertMedia(storeMediaRow(m.ChatJID, m.ID, m.Attachment))
	}
	return nil
}

func (w *Whatsmeow) ingestReceipt(r *Receipt) error {
	if r == nil {
		return nil
	}
	if r.ReaderJID != "" {
		for _, id := range r.MsgIDs {
			if err := w.store.UpsertReadReceipt(r.ChatJID, id, r.ReaderJID, r.TS); err != nil {
				return err
			}
		}
	}
	if r.Status > 0 && len(r.MsgIDs) > 0 {
		if err := w.applyReceiptStatus(r); err != nil {
			return err
		}
	}
	// Our other device read the chat (a self receipt): the badge clears.
	// Someone else reading our message says nothing about what we read.
	if !r.Read || r.ReaderJID != "" {
		return nil
	}
	return w.store.MarkChatRead(r.ChatJID)
}

// applyReceiptStatus moves the receipt's messages to its status. In a group
// a read receipt comes from one member; WhatsApp shows the message read
// only once every other member has read it, so until then the receipt
// counts as delivered.
func (w *Whatsmeow) applyReceiptStatus(r *Receipt) error {
	if r.Status != MessageStatusRead || !strings.HasSuffix(r.ChatJID, "@g.us") {
		return w.store.SetMessagesStatus(r.ChatJID, r.MsgIDs, r.Status)
	}
	return w.applyGroupReadState(r.ChatJID, r.MsgIDs)
}

// applyGroupReadState sets each of msgIDs in the group chatJID to read when
// every other current member has a read receipt for it, else delivered.
// With the membership not stored yet nothing can be claimed read; the
// members are fetched, and reconcileGroupReadState finishes the job.
func (w *Whatsmeow) applyGroupReadState(chatJID string, msgIDs []string) error {
	parts, err := w.store.GroupParticipants(chatJID)
	if err != nil {
		return err
	}
	if len(parts) == 0 && w.wa != nil {
		w.refreshGroupInfo(chatJID)
	}
	participants := make([]string, len(parts))
	for i, p := range parts {
		participants[i] = p.JID
	}
	for _, id := range msgIDs {
		receipts, err := w.store.ReadReceipts(chatJID, id)
		if err != nil {
			return err
		}
		readers := make([]string, len(receipts))
		for i, r := range receipts {
			readers[i] = r.ReaderJID
		}
		status := groupReadStatus(readers, participants, w.ownUsers(), w.canonicalUser)
		if err := w.store.SetMessagesStatus(chatJID, []string{id}, status); err != nil {
			return err
		}
	}
	return nil
}

// reconcileGroupReadState re-derives the read state of every message in
// chatJID that has read receipts: the receipts may have all arrived before
// the membership was known.
func (w *Whatsmeow) reconcileGroupReadState(chatJID string) error {
	ids, err := w.store.ReadReceiptMsgIDs(chatJID)
	if err != nil || len(ids) == 0 {
		return err
	}
	return w.applyGroupReadState(chatJID, ids)
}

// ownUsers is this account's identities (phone number and LID user parts).
func (w *Whatsmeow) ownUsers() []string {
	if w.wa == nil || w.wa.Store == nil {
		return nil
	}
	var out []string
	if own := w.wa.Store.ID; own != nil {
		out = append(out, own.User)
	}
	if lid := w.wa.Store.LID; !lid.IsEmpty() {
		out = append(out, lid.User)
	}
	return out
}

// canonicalUser names the person behind jid: the phone number's user part
// when the LID map knows it, else the JID's own user part. Receipts arrive
// under LIDs while memberships list phone numbers.
func (w *Whatsmeow) canonicalUser(jid string) string {
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return jid
	}
	if parsed.Server == types.HiddenUserServer && w.wa != nil && w.wa.Store != nil && w.wa.Store.LIDs != nil {
		if pn, err := w.wa.Store.LIDs.GetPNForLID(context.Background(), parsed); err == nil && !pn.IsEmpty() {
			return pn.User
		}
	}
	return parsed.User
}

func (w *Whatsmeow) ingestReaction(r *Reaction) error {
	if r == nil {
		return nil
	}
	return w.store.UpsertReaction(storeReactionRow(r))
}

func (w *Whatsmeow) ingestRevoke(r *Revoke) error {
	if r == nil {
		return nil
	}
	return w.store.MarkMessageDeleted(r.ChatJID, r.MsgID, r.TS)
}
