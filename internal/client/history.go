package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"

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
	for _, pn := range data.GetPushnames() {
		if pn.GetID() != "" && pn.GetPushname() != "" {
			w.upsertContact(store.ContactRow{JID: pn.GetID(), PushName: pn.GetPushname()})
		}
	}
	for _, m := range data.GetPhoneNumberToLidMappings() {
		if m.GetLidJID() != "" && m.GetPnJID() != "" {
			w.upsertContact(store.ContactRow{JID: m.GetLidJID(), PNJID: m.GetPnJID()})
		}
	}
}

func (w *Whatsmeow) applyHistoryConversation(conv *waHistorySync.Conversation) {
	jid := conv.GetID()
	if jid == "" {
		return
	}
	// A LID-addressed DM carries its phone-number twin (and vice versa):
	// record the mapping so the chat resolves to "+number" until a name
	// arrives, and file the conversation under the number.
	isGroup := strings.HasSuffix(jid, "@g.us")
	if strings.HasSuffix(jid, "@lid") && conv.GetPnJID() != "" {
		w.upsertContact(store.ContactRow{JID: jid, PNJID: conv.GetPnJID()})
	} else if !isGroup && conv.GetLidJID() != "" {
		w.upsertContact(store.ContactRow{JID: conv.GetLidJID(), PNJID: jid})
	}
	jid = w.canonicalChatJID(jid)
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
	// The phone's archive flag rides on the conversation itself; the
	// app-state Archive event only covers changes made after linking.
	if conv.Archived != nil {
		if err := w.store.SetChatArchived(jid, conv.GetArchived()); err != nil {
			w.log.Warnf("history: archive chat %s: %v", jid, err)
		}
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
	switch wmi.GetMessageStubType() {
	case waWeb.WebMessageInfo_REVOKE:
		// History keeps a deleted message as a content-less REVOKE stub; keep
		// the "This message was deleted" tombstone the live path would leave.
		msg.Deleted = true
	case waWeb.WebMessageInfo_UNKNOWN:
		extractText(wmi.GetMessage(), &msg)
		if !hasContent(&msg) {
			return
		}
	default:
		// Group/system stubs (participant added, subject changed, ...) have
		// no body chatot renders yet.
		return
	}
	if err := w.ingestMessage(&msg); err != nil {
		w.log.Warnf("history: ingest message %s/%s: %v", chatJID, msg.ID, err)
	}
}

// historySender derives the MessageInfo.Sender BuildHistorySyncRequest needs
// from the oldest stored message: own JID for our own messages, the parsed
// FromJID for others, falling back to the chat itself for a DM whose FromJID
// wasn't recorded.
func historySender(chat, own types.JID, fromMe bool, fromJID string) types.JID {
	if fromMe {
		return own
	}
	if fromJID == "" {
		return chat
	}
	if parsed, err := types.ParseJID(fromJID); err == nil {
		return parsed
	}
	return chat
}

// RequestMoreHistory asks the phone (via BuildHistorySyncRequest, sent as a
// peer message to ourselves) for up to count messages older than
// oldestMsgID in chatJID. It only sends the request; the reply arrives later
// as an events.HistorySync, handled by applyHistorySync/handleRaw, which
// pushes EventHistorySync so the conversation view can page again.
func (w *Whatsmeow) RequestMoreHistory(ctx context.Context, chatJID, oldestMsgID string, count int) error {
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", chatJID, err)
	}
	oldest, ok, err := w.store.MessageByID(chatJID, oldestMsgID)
	if err != nil {
		return fmt.Errorf("chatot/client: request more history: lookup %s: %w", oldestMsgID, err)
	}
	if !ok {
		return fmt.Errorf("chatot/client: request more history: message %s not found in %s", oldestMsgID, chatJID)
	}
	own, err := types.ParseJID(w.ownJID())
	if err != nil {
		return fmt.Errorf("chatot/client: request more history: own jid: %w", err)
	}
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   historySender(chat, own, oldest.FromMe, oldest.FromJID),
			IsFromMe: oldest.FromMe,
			IsGroup:  strings.HasSuffix(chatJID, "@g.us"),
		},
		ID:        oldestMsgID,
		Timestamp: time.Unix(oldest.TS, 0),
	}
	req := w.wa.BuildHistorySyncRequest(info, count)
	if _, err := w.wa.SendMessage(ctx, own, req, whatsmeow.SendRequestExtra{Peer: true}); err != nil {
		return fmt.Errorf("chatot/client: request more history: send: %w", err)
	}
	return nil
}
