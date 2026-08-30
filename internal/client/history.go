package client

import (
	"strings"

	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"

	"chatot/internal/store"
)

// applyHistorySync backfills chats, groups and messages from a HistorySync
// payload (delivered right after linking, and on demand). Per-contact name
// data (business/full/push/system name) arrives separately via app-state
// sync / message push names and isn't part of this payload; Chats() falls
// back to the group name or the +number rule until a contact row exists.
func (w *Whatsmeow) applyHistorySync(data *waHistorySync.HistorySync) {
	if data == nil || w.store == nil {
		return
	}
	for _, conv := range data.GetConversations() {
		w.applyHistoryConversation(conv)
	}
}

func (w *Whatsmeow) applyHistoryConversation(conv *waHistorySync.Conversation) {
	jid := conv.GetID()
	if jid == "" {
		return
	}
	isGroup := strings.HasSuffix(jid, "@g.us")
	if err := w.store.UpsertChat(store.ChatRow{
		JID:           jid,
		IsGroup:       isGroup,
		Name:          conv.GetName(),
		Pinned:        conv.GetPinned() != 0,
		Muted:         conv.GetMuteEndTime() != 0,
		UnreadCount:   int(conv.GetUnreadCount()),
		LastMessageTS: int64(conv.GetConversationTimestamp()),
	}); err != nil {
		w.log.Warnf("history: upsert chat %s: %v", jid, err)
	}
	if isGroup {
		if err := w.store.UpsertGroup(store.GroupRow{
			JID:             jid,
			Name:            conv.GetName(),
			IsParent:        conv.GetIsParentGroup(),
			LinkedParentJID: conv.GetParentGroupID(),
		}); err != nil {
			w.log.Warnf("history: upsert group %s: %v", jid, err)
		}
	}
	for _, hm := range conv.GetMessages() {
		w.applyHistoryMessage(jid, hm.GetMessage())
	}
}

func (w *Whatsmeow) applyHistoryMessage(chatJID string, wmi *waWeb.WebMessageInfo) {
	if wmi == nil {
		return
	}
	key := wmi.GetKey()
	msg := Message{
		ID:      key.GetID(),
		ChatJID: chatJID,
		FromJID: key.GetParticipant(),
		FromMe:  key.GetFromMe(),
		TS:      int64(wmi.GetMessageTimestamp()),
	}
	if msg.FromJID == "" && !msg.FromMe {
		msg.FromJID = chatJID // DM: the only other participant is the chat peer
	}
	extractText(wmi.GetMessage(), &msg)
	if err := w.ingestMessage(&msg); err != nil {
		w.log.Warnf("history: ingest message %s/%s: %v", chatJID, msg.ID, err)
	}
}
