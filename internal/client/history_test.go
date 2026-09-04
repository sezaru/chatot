package client

import (
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"chatot/internal/store"
)

func TestHistorySender(t *testing.T) {
	chat := types.NewJID("1112223333", types.DefaultUserServer)
	own := types.NewJID("1234567890", types.DefaultUserServer)
	other := types.NewJID("9998887777", types.DefaultUserServer)

	tests := []struct {
		name    string
		fromMe  bool
		fromJID string
		want    types.JID
	}{
		{"own message", true, "", own},
		{"own message ignores stale fromJID", true, other.String(), own},
		{"other message with known sender", false, other.String(), other},
		{"DM with unrecorded sender falls back to chat", false, "", chat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := historySender(chat, own, tt.fromMe, tt.fromJID)
			if got != tt.want {
				t.Errorf("historySender(%v, %v, %v) = %v, want %v", tt.fromMe, tt.fromJID, chat, got, tt.want)
			}
		})
	}
}

func TestHistorySyncSummaryCarriesKindProgressAndCounts(t *testing.T) {
	if h := historySyncSummary(nil); h.Progress != -1 || h.Type != "" || h.Chats != 0 {
		t.Fatalf("nil chunk summary = %+v, want empty with Progress -1", *h)
	}
	data := &waHistorySync.HistorySync{
		SyncType: waHistorySync.HistorySync_FULL.Enum(),
		Progress: proto.Uint32(42),
		Conversations: []*waHistorySync.Conversation{
			{ID: proto.String("1@s.whatsapp.net"), Messages: []*waHistorySync.HistorySyncMsg{{}, {}}},
			{ID: proto.String("2@g.us"), Messages: []*waHistorySync.HistorySyncMsg{{}}},
		},
	}
	h := historySyncSummary(data)
	if h.Type != "full" || h.Progress != 42 || h.Chats != 2 || h.Messages != 3 {
		t.Fatalf("summary = %+v, want full/42/2 chats/3 messages", *h)
	}
	if len(h.ChatJIDs) != 2 || h.ChatJIDs[1] != "2@g.us" {
		t.Fatalf("ChatJIDs = %v", h.ChatJIDs)
	}
	recent := &waHistorySync.HistorySync{SyncType: waHistorySync.HistorySync_RECENT.Enum()}
	if h := historySyncSummary(recent); h.Type != "recent" || h.Progress != -1 {
		t.Fatalf("recent summary = %+v, want recent with Progress -1", *h)
	}
}

func TestHistorySyncNeedsContactsOnlyForNameBearingChunks(t *testing.T) {
	full := &waHistorySync.HistorySync{SyncType: waHistorySync.HistorySync_FULL.Enum()}
	if historySyncNeedsContacts(full) {
		t.Fatal("a bare full chunk must not trigger a contact mirror")
	}
	full.Pushnames = []*waHistorySync.Pushname{{ID: proto.String("1@s.whatsapp.net"), Pushname: proto.String("Ada")}}
	if !historySyncNeedsContacts(full) {
		t.Fatal("a chunk carrying push names must trigger a contact mirror")
	}
	boot := &waHistorySync.HistorySync{SyncType: waHistorySync.HistorySync_INITIAL_BOOTSTRAP.Enum()}
	if !historySyncNeedsContacts(boot) {
		t.Fatal("bootstrap must trigger a contact mirror")
	}
	if historySyncNeedsContacts(nil) {
		t.Fatal("nil chunk must not trigger a contact mirror")
	}
}

// A backfilled conversation reports its own unread count; the inbound
// messages inside it must not be counted on top of it.
func TestApplyHistoryConversationKeepsPhoneUnreadCount(t *testing.T) {
	w := newIngestFixture(t)
	jid := "5551112222@s.whatsapp.net"
	conv := &waHistorySync.Conversation{
		ID:          proto.String(jid),
		UnreadCount: proto.Uint32(2),
		Messages: []*waHistorySync.HistorySyncMsg{
			historyText(jid, "m1", false, 1700000001, "one"),
			historyText(jid, "m2", false, 1700000002, "two"),
			historyText(jid, "m3", false, 1700000003, "three"),
			historyText(jid, "m4", true, 1700000004, "mine"),
		},
	}
	w.applyHistoryConversation(conv)
	c, ok := chatByJID(t, w, jid)
	if !ok {
		t.Fatalf("chat %s missing after history sync", jid)
	}
	if c.UnreadCount != 2 {
		t.Fatalf("unread after history sync = %d, want the phone's 2", c.UnreadCount)
	}
	msgs, err := w.store.Messages(jid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("stored %d messages, want 4", len(msgs))
	}
}

func historyText(chatJID, id string, fromMe bool, ts uint64, text string) *waHistorySync.HistorySyncMsg {
	return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{RemoteJID: proto.String(chatJID), ID: proto.String(id), FromMe: proto.Bool(fromMe)},
		MessageTimestamp: proto.Uint64(ts),
		Message:          &waE2E.Message{Conversation: proto.String(text)},
	}}
}

func TestHistoryMutedReadsAbsentZeroAlwaysAndExpiry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if _, known := historyMuted(&waHistorySync.Conversation{}, now); known {
		t.Fatal("absent muteEndTime must be unknown")
	}
	cases := []struct {
		end  uint64
		want bool
	}{
		{0, false},
		{^uint64(0), true},
		{1_700_000_000 + 3600, true},
		{1_700_000_000 - 3600, false},
		{(1_700_000_000 + 3600) * 1000, true},
		{(1_700_000_000 - 3600) * 1000, false},
	}
	for _, c := range cases {
		got, known := historyMuted(&waHistorySync.Conversation{MuteEndTime: proto.Uint64(c.end)}, now)
		if !known || got != c.want {
			t.Errorf("historyMuted(end=%d) = %v/%v, want %v/known", c.end, got, known, c.want)
		}
	}
}

func TestMuteActiveHonoursExpiry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if muteActive(&waSyncAction.MuteAction{Muted: proto.Bool(false)}, now) {
		t.Error("unmuted action must be inactive")
	}
	if !muteActive(&waSyncAction.MuteAction{Muted: proto.Bool(true), MuteEndTimestamp: proto.Int64(-1)}, now) {
		t.Error("always-mute must be active")
	}
	if !muteActive(&waSyncAction.MuteAction{Muted: proto.Bool(true)}, now) {
		t.Error("mute without expiry must be active")
	}
	if muteActive(&waSyncAction.MuteAction{Muted: proto.Bool(true), MuteEndTimestamp: proto.Int64(1_700_000_000 - 60)}, now) {
		t.Error("an expired mute must be inactive")
	}
	if !muteActive(&waSyncAction.MuteAction{Muted: proto.Bool(true), MuteEndTimestamp: proto.Int64((1_700_000_000 + 60) * 1000)}, now) {
		t.Error("a future millisecond expiry must be active")
	}
}

// App state reports the mute; a later history chunk for the same chat that
// carries no mute field must leave it alone.
func TestHistoryConversationKeepsAppStateMute(t *testing.T) {
	w := newIngestFixture(t)
	jid := "123456789@g.us"
	w.applyHistoryConversation(&waHistorySync.Conversation{ID: proto.String(jid), Name: proto.String("Bitcoin")})
	must(t, w.store.SetChatMuted(jid, true))
	must(t, w.store.SetChatPinned(jid, true))
	w.applyHistoryConversation(&waHistorySync.Conversation{ID: proto.String(jid), UnreadCount: proto.Uint32(9)})
	c, ok := chatByJID(t, w, jid)
	if !ok {
		t.Fatal("chat missing")
	}
	if !c.Muted || !c.Pinned {
		t.Fatalf("muted=%v pinned=%v after a flagless chunk, want both kept", c.Muted, c.Pinned)
	}
	w.applyHistoryConversation(&waHistorySync.Conversation{ID: proto.String(jid), MuteEndTime: proto.Uint64(0)})
	if c, _ := chatByJID(t, w, jid); c.Muted {
		t.Fatal("an explicit muteEndTime=0 must unmute")
	}
}

func TestApplyHistoryMessageNamesGroupSenderFromParticipant(t *testing.T) {
	w := newIngestFixture(t)
	group := "120363000000000000@g.us"
	sender := "5559998888@s.whatsapp.net"
	hm := historyText(group, "g1", false, 1700000001, "hi all")
	hm.Message.Participant = proto.String(sender)
	unknown := historyText(group, "g2", false, 1700000002, "no sender recorded")
	w.applyHistoryConversation(&waHistorySync.Conversation{
		ID:       proto.String(group),
		Messages: []*waHistorySync.HistorySyncMsg{hm, unknown},
	})
	msgs, err := w.store.Messages(group, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("stored %d messages, want 2", len(msgs))
	}
	if msgs[0].FromJID != sender {
		t.Errorf("group message sender = %q, want the participant %q", msgs[0].FromJID, sender)
	}
	if msgs[1].FromJID != "" {
		t.Errorf("group message with no participant has sender %q, want none rather than the group", msgs[1].FromJID)
	}
}

func TestApplyHistoryConversationKeepsLiveUnreadNewerThanSnapshot(t *testing.T) {
	w := newIngestFixture(t)
	jid := "5551112222@s.whatsapp.net"
	live := &Message{ID: "live", ChatJID: jid, FromJID: jid, TS: 1700000010, Text: "just now"}
	if err := w.ingestMessage(live); err != nil {
		t.Fatal(err)
	}
	w.applyHistoryConversation(&waHistorySync.Conversation{
		ID:                    proto.String(jid),
		UnreadCount:           proto.Uint32(0),
		ConversationTimestamp: proto.Uint64(1700000002),
		Messages: []*waHistorySync.HistorySyncMsg{
			historyText(jid, "m1", false, 1700000001, "one"),
			historyText(jid, "m2", true, 1700000002, "mine"),
		},
	})
	c, ok := chatByJID(t, w, jid)
	if !ok {
		t.Fatalf("chat %s missing", jid)
	}
	if c.UnreadCount != 1 {
		t.Errorf("unread after an older history snapshot = %d, want the live message's 1", c.UnreadCount)
	}
	if c.LastMessageTS != 1700000010 {
		t.Errorf("last message ts = %d, want the live message's", c.LastMessageTS)
	}
}

func TestRepairGroupSendersOnceBlanksGroupAsSender(t *testing.T) {
	w := newIngestFixture(t)
	group := "120363000000000000@g.us"
	dm := "5551112222@s.whatsapp.net"
	for _, m := range []*Message{
		{ID: "g1", ChatJID: group, FromJID: group, TS: 1, Text: "misfiled"},
		{ID: "g2", ChatJID: group, FromJID: "5559998888@s.whatsapp.net", TS: 2, Text: "fine"},
		{ID: "d1", ChatJID: dm, FromJID: dm, TS: 3, Text: "dm peer is the chat"},
	} {
		if err := w.ingestMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	w.repairGroupSendersOnce()
	got := map[string]string{}
	for _, chat := range []string{group, dm} {
		msgs, err := w.store.Messages(chat, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range msgs {
			got[m.ID] = m.FromJID
		}
	}
	if got["g1"] != "" || got["g2"] != "5559998888@s.whatsapp.net" || got["d1"] != dm {
		t.Errorf("senders after repair = %v", got)
	}
	if done, _ := w.store.Meta(groupSenderRepairKey); done == "" {
		t.Error("repair not recorded as done")
	}
}

func TestNewWhatsmeowRepairsGroupSendersOnce(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "chatot.db"))
	if err != nil {
		t.Fatal(err)
	}
	group := "120363000000000000@g.us"
	if err := s.BumpChatActivity(group, true, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(store.MessageRow{ChatJID: group, MsgID: "g1", FromJID: group, TS: 1, Text: "misfiled"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	w, err := NewWhatsmeow(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.store.Close()
	msgs, err := w.store.Messages(group, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].FromJID != "" {
		t.Fatalf("after NewWhatsmeow: %+v, want the group sender blanked", msgs)
	}
	if done, _ := w.store.Meta(groupSenderRepairKey); done == "" {
		t.Error("repair not recorded")
	}
}
