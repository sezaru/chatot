package client

import "strings"

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
	if !m.FromMe {
		unreadDelta = 1
	}
	if err := w.store.BumpChatActivity(m.ChatJID, isGroup, m.TS, unreadDelta); err != nil {
		return err
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
	if r == nil || !r.Read {
		return nil
	}
	return w.store.MarkChatRead(r.ChatJID)
}

func (w *Whatsmeow) ingestReaction(r *Reaction) error {
	if r == nil {
		return nil
	}
	return w.store.UpsertReaction(storeReactionRow(r))
}
