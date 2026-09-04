package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
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
	// Pin and mute live in app state; a conversation only sometimes repeats
	// them, and an absent field must not clear what app state already set.
	muted, hasMute := historyMuted(conv, time.Now())
	if err := w.store.UpsertChatFlags(store.ChatRow{
		JID:           jid,
		IsGroup:       isGroup,
		Name:          conv.GetName(),
		Pinned:        conv.GetPinned() != 0,
		Muted:         muted,
		UnreadCount:   int(conv.GetUnreadCount()),
		LastMessageTS: int64(conv.GetConversationTimestamp()),
	}, conv.Pinned != nil, hasMute); err != nil {
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
	// The conversation's own unreadCount (applied by applyHistoryConversation)
	// is authoritative; counting every inbound message in the backlog on top
	// of it would report a chat with 3 unread as having hundreds.
	if err := w.ingestMessageUnread(&msg, 0); err != nil {
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

// historySyncTypeName maps whatsmeow's chunk kind onto HistorySync.Type.
func historySyncTypeName(t waHistorySync.HistorySync_HistorySyncType) string {
	switch t {
	case waHistorySync.HistorySync_INITIAL_BOOTSTRAP:
		return "bootstrap"
	case waHistorySync.HistorySync_INITIAL_STATUS_V3:
		return "status"
	case waHistorySync.HistorySync_FULL:
		return "full"
	case waHistorySync.HistorySync_RECENT:
		return "recent"
	case waHistorySync.HistorySync_PUSH_NAME:
		return "pushname"
	case waHistorySync.HistorySync_NON_BLOCKING_DATA:
		return "nonblocking"
	case waHistorySync.HistorySync_ON_DEMAND:
		return "ondemand"
	}
	return ""
}

// historySyncSummary condenses a chunk into the HistorySync event the UI
// consumes: the touched chat JIDs plus kind, progress and volume.
func historySyncSummary(data *waHistorySync.HistorySync) *HistorySync {
	h := &HistorySync{Progress: -1}
	if data == nil {
		return h
	}
	h.Type = historySyncTypeName(data.GetSyncType())
	if data.Progress != nil {
		h.Progress = int(data.GetProgress())
	}
	for _, c := range data.GetConversations() {
		if jid := c.GetID(); jid != "" {
			h.ChatJIDs = append(h.ChatJIDs, jid)
		}
		h.Chats++
		h.Messages += len(c.GetMessages())
	}
	return h
}

// historySyncNeedsContacts reports whether a chunk can have changed
// whatsmeow's contact store (push names ride on bootstrap and push-name
// chunks); mirroring contacts after every one of the hundreds of "full"
// chunks a large account streams is pure churn.
func historySyncNeedsContacts(data *waHistorySync.HistorySync) bool {
	if data == nil {
		return false
	}
	switch data.GetSyncType() {
	case waHistorySync.HistorySync_INITIAL_BOOTSTRAP, waHistorySync.HistorySync_PUSH_NAME:
		return true
	}
	return len(data.GetPushnames()) > 0
}

// historyMuted reads a conversation's mute: (muted, known). Unknown when
// the field is absent. 0 is unmuted, all-ones is WhatsApp's "always", and
// anything else is an expiry (seconds, or milliseconds on some builds)
// that only counts while it lies in the future.
func historyMuted(conv *waHistorySync.Conversation, now time.Time) (bool, bool) {
	if conv == nil || conv.MuteEndTime == nil {
		return false, false
	}
	t := conv.GetMuteEndTime()
	switch {
	case t == 0:
		return false, true
	case t == ^uint64(0):
		return true, true
	}
	return muteExpiryActive(int64(t), now), true
}

// muteExpiryActive reports whether a mute ending at end (unix seconds, or
// milliseconds when implausibly large) is still in force at now.
func muteExpiryActive(end int64, now time.Time) bool {
	if end > 1e12 {
		end /= 1000
	}
	return end > now.Unix()
}

// muteActive reads an app-state mute action: a mute with an expiry in the
// past is over even though the phone never sends an explicit unmute for it;
// -1 (or no expiry at all) is "always".
func muteActive(a *waSyncAction.MuteAction, now time.Time) bool {
	if a == nil || !a.GetMuted() {
		return false
	}
	end := a.GetMuteEndTimestamp()
	if end <= 0 {
		return true
	}
	return muteExpiryActive(end, now)
}
