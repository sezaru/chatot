package client

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"chatot/internal/store"
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
	case EventCall:
		err = w.ingestCall(e.Call)
	}
	if err != nil {
		w.log.Warnf("store ingest failed (kind=%d): %v", e.Kind, err)
	}
}

func (w *Whatsmeow) ingestMessage(m *Message) error {
	if m == nil {
		return nil
	}
	unreadDelta := 0
	if !m.FromMe && !m.Edited {
		unreadDelta = 1
	}
	return w.ingestMessageUnread(m, unreadDelta)
}

// ingestMessageUnread is ingestMessage with an explicit unread delta, for
// callers (history backfill) whose messages must not count as new.
func (w *Whatsmeow) ingestMessageUnread(m *Message, unreadDelta int) error {
	if m == nil {
		return nil
	}
	isGroup := strings.HasSuffix(m.ChatJID, "@g.us")
	// A message seen before (a resend, or one the history already
	// delivered) is no new unread.
	if unreadDelta > 0 {
		_, known, err := w.store.MessageByID(m.ChatJID, m.ID)
		if err != nil {
			return err
		}
		if known {
			unreadDelta = 0
		}
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

// ingestReaction stores the reaction and, for one somebody else left on a
// message of ours, describes the target on the event (for the "Reacted 👍
// to ..." notification) and bumps the chat so it surfaces in the list the
// way WhatsApp does. Reacting to someone else's message, or to our own from
// another device, is nobody's news.
func (w *Whatsmeow) ingestReaction(r *Reaction) error {
	if r == nil {
		return nil
	}
	if err := w.store.UpsertReaction(storeReactionRow(r)); err != nil {
		return err
	}
	if r.ReactorJID == "" || jidUserIn(r.ReactorJID, w.ownUsers()) {
		return nil
	}
	preview, fromMe, ok, err := w.store.MessagePreview(r.ChatJID, r.MsgID)
	if err != nil || !ok || !fromMe {
		return err
	}
	r.TargetFromMe, r.TargetPreview = true, preview
	if r.Emoji == "" {
		return nil
	}
	if err := w.store.BumpChatActivity(r.ChatJID, strings.HasSuffix(r.ChatJID, "@g.us"), r.TS, 0); err != nil {
		return err
	}
	// The sidebar ignores reaction events (they are most of a busy
	// account's traffic); this one changed the row's preview and order.
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: r.ChatJID}})
	return nil
}

// ingestCall logs a call in its chat the way WhatsApp does. The offer
// writes the row, as a missed call: this device never answers, and a call
// nobody picks up is exactly that. A later accept (the phone took it) or
// reject turns the row into an answered or declined call. The row is
// pushed as a synced message so an open thread appends it without ringing.
func (w *Whatsmeow) ingestCall(c *Call) error {
	if c == nil || c.CallID == "" {
		return nil
	}
	id := callMsgID(c.CallID)
	row, known, err := w.store.MessageByID(c.ChatJID, id)
	if err != nil {
		return err
	}
	if c.Offer {
		if known {
			return nil
		}
		msg := Message{
			ID: id, ChatJID: c.ChatJID, FromJID: c.CallerJID, TS: c.TS,
			CallLog: &CallLog{Video: c.Video, Outcome: CallMissed},
		}
		if msg.FromJID == "" {
			msg.FromJID = c.ChatJID
		}
		if msg.TS == 0 {
			msg.TS = time.Now().Unix()
		}
		// A missed call is unread, like WhatsApp's badge on the chat; an
		// accept below takes it back.
		if err := w.ingestMessageUnread(&msg, 1); err != nil {
			return err
		}
		w.pushEvent(Event{Kind: EventMessage, Synced: true, Message: &msg})
		return nil
	}
	if !known || row.Kind != "call" || c.Outcome == "" || c.Outcome == CallMissed {
		// Missed is what the row already says; nothing else to settle.
		return nil
	}
	var p callPayload
	if err := json.Unmarshal([]byte(row.Payload), &p); err != nil {
		return err
	}
	if p.Outcome == c.Outcome {
		return nil
	}
	wasMissed := p.Outcome == CallMissed
	p.Outcome = c.Outcome
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := w.store.UpsertMessage(store.MessageRow{
		ChatJID: c.ChatJID, MsgID: id, FromJID: row.FromJID, FromMe: row.FromMe, TS: row.TS,
		Kind: "call", Payload: string(b),
	}); err != nil {
		return err
	}
	if wasMissed {
		if err := w.store.BumpChatActivity(c.ChatJID, strings.HasSuffix(c.ChatJID, "@g.us"), 0, -1); err != nil {
			return err
		}
	}
	w.pushEvent(Event{Kind: EventReaction, Reaction: &Reaction{ChatJID: c.ChatJID, MsgID: id}})
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: c.ChatJID}})
	return nil
}

func (w *Whatsmeow) ingestRevoke(r *Revoke) error {
	if r == nil {
		return nil
	}
	return w.store.MarkMessageDeleted(r.ChatJID, r.MsgID, r.TS)
}
