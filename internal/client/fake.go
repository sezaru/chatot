package client

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"chatot/internal/store"
)

var _ Client = (*Fake)(nil)

// Fake is an in-memory Client for unit tests and UI development without a
// live WhatsApp link. It comes pre-seeded with a couple of chats/messages.
type Fake struct {
	mu       sync.Mutex
	chats    []Chat
	messages map[string][]Message // chatJID -> messages, oldest first
	events   *eventBus
	qrCodes  chan string
	loggedIn bool
	// markReads records MarkRead calls for tests (see MarkReadCalls).
	markReads []MarkReadCall
	// stickers is the picker library, most recent first.
	stickers []Sticker
	// pairing marks a demo account that never links: Start emits a demo QR
	// instead of flipping loggedIn, so the add-account and relink dialogs
	// have a code to render in CHATOT_FAKE=1 builds.
	pairing bool
	nextID  int
	blocked map[string]bool
	labels  []Label
	// labelChats maps a labelID to the set of chat JIDs it's associated with.
	labelChats map[string]map[string]bool
	// groups holds mutable in-memory group state keyed by JID, so F27's
	// mutations are reflected in a subsequent GroupInfo.
	groups     map[string]*GroupInfo
	nextGroupN int
	// joinRequests holds mutable in-memory pending join requests keyed by
	// group JID, seeded so the join-approval banner has something to show.
	joinRequests map[string][]JoinRequest
	// historySynced tracks chats RequestMoreHistory has already backfilled,
	// so a chat only grows one extra page of synthetic older history rather
	// than without bound.
	historySynced map[string]bool
	// newsletters holds mutable in-memory channel state keyed by JID (never
	// persisted); newsletterMsgs holds a few seeded posts per channel.
	newsletters    map[string]*Newsletter
	newsletterMsgs map[string][]NewsletterMessage
	// newsletterMine is our current reaction per "jid/messageID" post.
	newsletterMine map[string]string
	// statusMuted is the set of muted status posters; statusViewers the
	// read receipts on our own updates by message ID; statusHidden who we
	// hid our status from; privacy the account privacy settings.
	statusMuted   map[string]bool
	statusViewers map[string][]StatusViewer
	statusHidden  map[string]bool
	privacy       map[string]string
	// newsletterViewed is the set of "jid/messageID" posts already counted
	// as viewed.
	newsletterViewed map[string]bool
	// directory is the channel directory DiscoverNewsletters searches:
	// channels this account may or may not follow, keyed by JID.
	directory map[string]*Newsletter
	// communities holds mutable in-memory community state, seeded like the
	// mockup's two communities (see seedTabs).
	communities []Community
}

// fakeOwnJID is the Fake's own user JID. It matches the canned group's owner
// so admin-only controls render during CHATOT_FAKE=1 development.
const fakeOwnJID = "1234567890@s.whatsapp.net"

// NewFake returns a Fake seeded with canned chats and messages, already
// logged in (no QR codes will be emitted).
func NewFake() *Fake {
	now := time.Now().Unix()
	f := &Fake{
		messages: make(map[string][]Message),
		events:   newEventBus(nil),
		qrCodes:  make(chan string, 1),
		loggedIn: true,
		blocked:  make(map[string]bool),
		labels: []Label{
			{ID: "1", Name: "Work", Color: 0},
			{ID: "2", Name: "Family", Color: 5},
		},
		labelChats: map[string]map[string]bool{
			"1": {"1234567890@s.whatsapp.net": true},
			"2": {"1112223333@s.whatsapp.net": true},
		},
		groups: map[string]*GroupInfo{
			"weekendtrip@g.us": {
				JID: "weekendtrip@g.us", Name: "Weekend Trip", Topic: "Planning for the cabin trip", OwnerJID: fakeOwnJID,
				Participants: []GroupParticipant{
					{JID: fakeOwnJID, IsAdmin: true, IsSuperAdmin: true},
					{JID: "1112223333@s.whatsapp.net"},
					{JID: "4445556666@s.whatsapp.net"},
					{JID: "7778889999@s.whatsapp.net"},
				},
			},
		},
		historySynced: make(map[string]bool),
		joinRequests: map[string][]JoinRequest{
			"weekendtrip@g.us": {
				{JID: "1998887777@s.whatsapp.net", RequestedAt: time.Unix(now-1800, 0)},
			},
		},
		newsletters: map[string]*Newsletter{
			"111111@newsletter": {ID: "111111@newsletter", Name: "Chatot News", Description: "Release notes and updates", Muted: false,
				Verified: true, Subscribers: 4218, InviteCode: "releases8ka2", Created: now - 400*86400, Following: true, Category: "Technology"},
			"222222@newsletter": {ID: "222222@newsletter", Name: "Weather Alerts", Description: "Daily local forecast", Muted: true,
				Verified: true, Subscribers: 182000, InviteCode: "alerts3rv7", Created: now - 900*86400, Following: true, Category: "News"},
		},
		newsletterMsgs: map[string][]NewsletterMessage{
			"111111@newsletter": {
				{ID: "n1", ServerID: 1, Text: "Welcome to the channel!", TS: now - 7200, Views: 120, Reactions: map[string]int{"👍": 8, "❤️": 3}},
				{ID: "n2", ServerID: 2, Text: "v2.0 is out with channels support.", TS: now - 3600, Views: 64, Reactions: map[string]int{"🎉": 5}},
				{ID: "n2b", ServerID: 3, Text: "The new tab bar, straight from the mockup.", TS: now - 1200, Views: 31, Reactions: map[string]int{"🔥": 2},
					Attachment: &Attachment{Kind: "image", MimeType: "image/png", Caption: "The new tab bar, straight from the mockup.", Thumbnail: fakeMapThumbnail()}},
			},
			"222222@newsletter": {
				{ID: "n3", ServerID: 1, Text: "Rain expected this afternoon.", TS: now - 1800, Views: 42, Reactions: map[string]int{}},
			},
		},
	}

	f.chats = []Chat{
		{JID: "1234567890@s.whatsapp.net", Name: "Ada Lovelace", Preview: "See you tomorrow!", UnreadCount: 2, LastMessageTS: now - 60, Pinned: true},
		{JID: "1112223333@s.whatsapp.net", Name: "Grace Hopper", Preview: "Bug found in the relay", UnreadCount: 0, LastMessageTS: now - 3600},
		{JID: "weekendtrip@g.us", Name: "Weekend Trip", Preview: "See everyone Friday!", UnreadCount: 1, LastMessageTS: now - 7200, IsGroup: true},
	}

	f.messages["1234567890@s.whatsapp.net"] = []Message{
		{ID: "m1", ChatJID: "1234567890@s.whatsapp.net", FromJID: "1234567890@s.whatsapp.net", FromMe: false, Text: "Hey, are we still on for tomorrow?", TS: now - 120},
		{ID: "m2", ChatJID: "1234567890@s.whatsapp.net", FromJID: "me", FromMe: true, Text: "Yep!", TS: now - 90, Status: MessageStatusRead,
			Reactions: map[string][]string{"❤️": {"1234567890@s.whatsapp.net"}}},
		{ID: "m3", ChatJID: "1234567890@s.whatsapp.net", FromJID: "1234567890@s.whatsapp.net", FromMe: false, Text: "See you tomorrow!", TS: now - 60, Forwarded: true},
	}
	f.messages["1112223333@s.whatsapp.net"] = []Message{
		// Two people on 👍 (a counted pill) beside a lone 😮: the mockup's
		// multi-reaction row, with one of the 👍s being ours so it toggles.
		{ID: "m4", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, Text: "Bug found in the relay", TS: now - 3600,
			Reactions: map[string][]string{"👍": {"1112223333@s.whatsapp.net", fakeOwnJID}, "😮": {"4445556666@s.whatsapp.net"}}},
		{ID: "m5", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 3000,
			Location: &Location{Name: "Bletchley Park", Address: "Sherwood Dr, Bletchley, Milton Keynes", Latitude: 51.9976, Longitude: -0.7406, Thumbnail: fakeMapThumbnail()}},
		{ID: "m6", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2900,
			Contact: &Contact{DisplayName: "Alan Turing", Phones: []string{"+44 20 7946 0958"}}},
		{ID: "m7", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2800,
			Poll: &Poll{Name: "Lunch tomorrow?", SelectableCount: 1, Options: []PollOption{
				{Name: "Pizza", Count: 1},
				{Name: "Sushi"},
				{Name: "Salad"},
			}}},
		{ID: "m8", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2700,
			Attachment: &Attachment{Kind: "video", MimeType: "video/mp4", IsGIF: true, Size: 860160, DurationSecs: 34}},
		// No LocalPath: renders via the tap-to-load placeholder path, exercising
		// the no-bubble sticker render without needing a bundled webp asset.
		{ID: "m9", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2600,
			Attachment: &Attachment{Kind: "sticker", MimeType: "image/webp", Size: 24576}},
		// m10-m12 seed the Media/Links/Docs page (F43): an image (Media tab,
		// alongside m8's video), a document (Docs tab) and a URL-bearing text
		// message (Links tab).
		{ID: "m10", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2500,
			Attachment: &Attachment{Kind: "image", MimeType: "image/jpeg", Size: 860160}},
		{ID: "m11", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2400,
			Attachment: &Attachment{Kind: "document", Filename: "lease-2026.pdf", MimeType: "application/pdf", Size: 1258291}},
		{ID: "m12", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2300,
			Text: "Cabin listing — 3 bedrooms: https://stay.example.com/cabin/4412"},
		// m13 seeds F49's view-once bubble: unopened, so it renders the
		// "Click to open · closes after viewing" placeholder.
		{ID: "m13", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2200,
			Attachment: &Attachment{Kind: "image", MimeType: "image/jpeg", ViewOnce: true, Size: 512000}},
		// m14 seeds F50's live-location bubble, sharing until an hour from now.
		{ID: "m14", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2100,
			Location: &Location{Latitude: 51.5007, Longitude: -0.1246, IsLive: true, LiveUntil: now + 3600}},
		// m15 seeds F52's event bubble: a group-style scheduled event a week out.
		{ID: "m15", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2000,
			EventInvite: &EventInvite{Name: "Team offsite", Location: "Bletchley Park", StartTS: now + 7*86400}},
		// m16 seeds the voice-note bubble, which had no fake coverage at all:
		// undownloaded, so it renders the "🎤 Voice message · 0:12" row.
		{ID: "m16", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 1900,
			Attachment: &Attachment{Kind: "audio", MimeType: "audio/ogg", Size: 49152, DurationSecs: 12}},
	}

	f.messages[statusBroadcastJID] = []Message{
		{ID: "s1", ChatJID: statusBroadcastJID, FromJID: "1234567890@s.whatsapp.net", FromMe: false, Text: "Off to the mountains! 🏔️", TS: now - 1800},
		{ID: "s2", ChatJID: statusBroadcastJID, FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 900,
			Attachment: &Attachment{Kind: "image"}},
	}

	f.seedGroupThread(now)
	f.seedTabs(now)
	if os.Getenv("CHATOT_FAKE_BIG") != "" {
		f.seedBig(now)
	}
	// After seedTabs: it lays the status feed afresh, and the dev media
	// adds statuses to it.
	if dir := os.Getenv("CHATOT_FAKE_MEDIA"); dir != "" {
		f.seedDevMedia(dir, now)
	}
	return f
}

func (f *Fake) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pairing {
		select {
		case f.qrCodes <- "chatot-demo-pairing-code":
		default:
		}
		return nil
	}
	f.loggedIn = true
	return nil
}

// NewPairingFake is an empty, logged-out Fake that only produces demo QR
// codes — what AddPairingAccount hands out in the demo build.
func NewPairingFake() *Fake {
	f := NewFake()
	f.chats = nil
	f.messages = make(map[string][]Message)
	f.loggedIn = false
	f.pairing = true
	return f
}

func (f *Fake) QRCodes() <-chan string { return f.qrCodes }

func (f *Fake) Paired() bool { return true }

func (f *Fake) LoggedIn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loggedIn
}

func (f *Fake) Logout(ctx context.Context) error {
	f.mu.Lock()
	f.loggedIn = false
	f.mu.Unlock()
	// The real client reports the logout as an event; the window relies on
	// it to fall back to the pairing screen.
	f.events.Publish(Event{Kind: EventLoggedOut})
	return nil
}

func (f *Fake) PairPhone(ctx context.Context, phone string) (string, error) {
	if _, ok := normalizePairingPhone(phone); !ok {
		return "", fmt.Errorf("chatot/client: invalid phone number %q", phone)
	}
	return "ABCD-1234", nil
}

func (f *Fake) Events() <-chan Event { return f.events.Subscribe() }

// PushEvent lets tests/UI-dev inject an event onto the Events() stream,
// broadcasting to every current subscriber.
func (f *Fake) PushEvent(e Event) { f.events.Publish(e) }

func (f *Fake) Chats(limit int) ([]Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Chat, len(f.chats))
	copy(out, f.chats)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *Fake) Messages(jid string, limit int) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.messages[jid]
	out := make([]Message, len(msgs))
	copy(out, msgs)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// MessagesBefore returns up to limit messages older than beforeMsgID (oldest
// first). An unknown beforeMsgID yields no messages, matching the store.
func (f *Fake) MessagesBefore(jid, beforeMsgID string, limit int) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.messages[jid]
	end := -1
	for i := range msgs {
		if msgs[i].ID == beforeMsgID {
			end = i
			break
		}
	}
	if end <= 0 {
		return nil, nil
	}
	start := 0
	if limit > 0 && end-limit > 0 {
		start = end - limit
	}
	out := make([]Message, end-start)
	copy(out, msgs[start:end])
	return out, nil
}

// RequestMoreHistory simulates the phone's async history-sync reply: the
// first time it's called for a chat, it synthesizes a handful of messages
// older than oldestMsgID and publishes EventHistorySync so the conversation
// view retries MessagesBefore and finds them. Later calls (chat already
// synced, or an unknown oldestMsgID) are no-ops, matching "no more history".
func (f *Fake) RequestMoreHistory(ctx context.Context, chatJID, oldestMsgID string, count int) error {
	f.mu.Lock()
	if f.historySynced[chatJID] {
		f.mu.Unlock()
		return nil
	}
	msgs := f.messages[chatJID]
	idx := -1
	for i := range msgs {
		if msgs[i].ID == oldestMsgID {
			idx = i
			break
		}
	}
	if idx < 0 {
		f.mu.Unlock()
		return fmt.Errorf("chatot/client: request more history: message %q not found in %q", oldestMsgID, chatJID)
	}
	f.historySynced[chatJID] = true

	n := 5
	if count > 0 && count < n {
		n = count
	}
	base := msgs[idx].TS
	older := make([]Message, n)
	for i := 0; i < n; i++ {
		older[n-1-i] = Message{
			ID:      fmt.Sprintf("fake-history-%s-%d", chatJID, i),
			ChatJID: chatJID,
			FromJID: msgs[idx].FromJID,
			FromMe:  false,
			Text:    fmt.Sprintf("(synced older message %d)", i+1),
			TS:      base - int64(i+1)*60,
		}
	}
	f.messages[chatJID] = append(older, msgs...)
	f.mu.Unlock()

	f.events.Publish(Event{Kind: EventHistorySync, HistorySync: &HistorySync{ChatJIDs: []string{chatJID}, Type: "ondemand", Progress: -1}})
	return nil
}

func (f *Fake) Search(query string, limit int) ([]SearchHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	query = strings.ToLower(query)
	var hits []SearchHit
	for jid, msgs := range f.messages {
		for _, m := range msgs {
			if strings.Contains(strings.ToLower(m.Text), query) {
				hits = append(hits, SearchHit{ChatJID: jid, MsgID: m.ID, ChatName: f.chatName(jid), Snippet: m.Text, TS: m.TS})
				if limit > 0 && len(hits) >= limit {
					return hits, nil
				}
			}
		}
	}
	return hits, nil
}

func (f *Fake) SearchInChat(chatJID, query string, limit int) ([]SearchHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	var hits []SearchHit
	for _, m := range f.messages[chatJID] {
		if strings.Contains(strings.ToLower(m.Text), query) {
			hits = append(hits, SearchHit{ChatJID: chatJID, MsgID: m.ID, ChatName: f.chatName(chatJID), Snippet: m.Text, TS: m.TS})
			if limit > 0 && len(hits) >= limit {
				break
			}
		}
	}
	return hits, nil
}

// chatName looks up jid's display name among the seeded/fake chats, falling
// back to the JID itself.
func (f *Fake) chatName(jid string) string {
	for _, c := range f.chats {
		if c.JID == jid {
			return c.Name
		}
	}
	return jid
}

func (f *Fake) nextMsgID() string {
	f.nextID++
	return fmt.Sprintf("fake-%d", f.nextID)
}

func (f *Fake) appendOutbound(jid string, msg Message) {
	f.messages[jid] = append(f.messages[jid], msg)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = msg.Text
			f.chats[i].LastMessageTS = msg.TS
			break
		}
	}
}

// Receive delivers an inbound text from sender into jid as if the peer had
// just sent it: the thread grows, the row's badge counts it and the message
// event goes out. A dev/screenshot aid for the "arrived while away" states.
func (f *Fake) Receive(jid, sender, text string) {
	f.mu.Lock()
	id := f.nextMsgID()
	msg := Message{ID: id, ChatJID: jid, FromJID: sender, Text: text, TS: time.Now().Unix()}
	f.messages[jid] = append(f.messages[jid], msg)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = text
			f.chats[i].LastMessageTS = msg.TS
			f.chats[i].UnreadCount++
			break
		}
	}
	f.mu.Unlock()
	f.PushEvent(Event{Kind: EventMessage, Message: &msg})
}

func (f *Fake) SendText(ctx context.Context, jid, text string, replyTo *MsgRef) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	f.appendOutbound(jid, Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, Text: text, TS: time.Now().Unix(), ReplyTo: replyTo})
	return id, nil
}

func (f *Fake) SendMedia(ctx context.Context, jid string, m Attachment, replyTo *MsgRef) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	f.appendOutbound(jid, Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, Text: m.Caption, TS: time.Now().Unix(), ReplyTo: replyTo, Attachment: &m})
	return id, nil
}

func (f *Fake) SendLocation(ctx context.Context, jid string, loc Location, replyTo *MsgRef) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	l := loc
	l.IsLive = false
	msg := Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, TS: time.Now().Unix(), ReplyTo: replyTo, Location: &l}
	f.messages[jid] = append(f.messages[jid], msg)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = "📍 Location"
			f.chats[i].LastMessageTS = msg.TS
			break
		}
	}
	return id, nil
}

func (f *Fake) SendLiveLocation(ctx context.Context, jid string, lat, lon float64, durationSecs int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	now := time.Now().Unix()
	loc := Location{Latitude: lat, Longitude: lon, IsLive: true, LiveUntil: now + int64(durationSecs)}
	msg := Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, TS: now, Location: &loc}
	f.messages[jid] = append(f.messages[jid], msg)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = "📍 Live location"
			f.chats[i].LastMessageTS = msg.TS
			break
		}
	}
	return id, nil
}

func (f *Fake) SendContact(ctx context.Context, jid string, contact Contact, replyTo *MsgRef) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	c := contact
	msg := Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, TS: time.Now().Unix(), ReplyTo: replyTo, Contact: &c}
	f.messages[jid] = append(f.messages[jid], msg)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = "👤 " + contact.DisplayName
			f.chats[i].LastMessageTS = msg.TS
			break
		}
	}
	return id, nil
}

// ForwardMessage appends a copy of msg's content to toJID, marked Forwarded.
// Content is prioritized attachment > location > contact > poll > text,
// mirroring the single body a real message ever carries.
func (f *Fake) ForwardMessage(ctx context.Context, msg Message, toJID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	out := Message{ID: id, ChatJID: toJID, FromJID: "me", FromMe: true, TS: time.Now().Unix(), Text: msg.Text, Forwarded: true}
	preview := msg.Text
	switch {
	case msg.Attachment != nil:
		a := *msg.Attachment
		out.Attachment = &a
		preview = "📎 " + a.Kind
	case msg.Location != nil:
		l := *msg.Location
		out.Location = &l
		preview = "📍 Location"
	case msg.Contact != nil:
		c := *msg.Contact
		out.Contact = &c
		preview = "👤 " + c.DisplayName
	case msg.Poll != nil:
		p := *msg.Poll
		out.Poll = &p
		preview = "📊 " + p.Name
	}
	f.messages[toJID] = append(f.messages[toJID], out)
	for i := range f.chats {
		if f.chats[i].JID == toJID {
			f.chats[i].Preview = preview
			f.chats[i].LastMessageTS = out.TS
			break
		}
	}
	return id, nil
}

// ClearChat drops jid's in-memory messages and blanks its chat-list preview;
// alsoMedia is accepted for interface parity but there are no cached files
// to remove in the fake.
func (f *Fake) ClearChat(ctx context.Context, jid string, alsoMedia bool) error {
	f.mu.Lock()
	delete(f.messages, jid)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = ""
			f.chats[i].UnreadCount = 0
			break
		}
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	att := Attachment{Kind: "audio", MimeType: "audio/ogg", Data: oggOpus}
	f.appendOutbound(jid, Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, TS: time.Now().Unix(), Attachment: &att})
	return id, nil
}

// SendSticker appends an outbound sticker attachment message; path is only
// recorded as LocalPath so the sender's own bubble renders inline without a
// download round-trip, mirroring the real Whatsmeow client's optimistic echo.
func (f *Fake) SendSticker(ctx context.Context, jid, path string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	msg := Message{
		ID: id, ChatJID: jid, FromJID: "me", FromMe: true, TS: time.Now().Unix(),
		Attachment: &Attachment{Kind: "sticker", MimeType: "image/webp", LocalPath: path},
	}
	f.messages[jid] = append(f.messages[jid], msg)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = "🙂 Sticker"
			f.chats[i].LastMessageTS = msg.TS
			break
		}
	}
	return id, nil
}

func (f *Fake) CreatePoll(ctx context.Context, jid, name string, options []string, selectable int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	opts := make([]PollOption, len(options))
	for i, o := range options {
		opts[i] = PollOption{Name: o}
	}
	msg := Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, TS: time.Now().Unix(),
		Poll: &Poll{Name: name, Options: opts, SelectableCount: selectable}}
	f.messages[jid] = append(f.messages[jid], msg)
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].Preview = "📊 " + name
			f.chats[i].LastMessageTS = msg.TS
			break
		}
	}
	return id, nil
}

func (f *Fake) VotePoll(ctx context.Context, chatJID, pollMsgID string, options []string) error {
	if err := f.votePollLocked(chatJID, pollMsgID, options); err != nil {
		return err
	}
	// The real client emits the tally update it recorded; so does the fake,
	// so the open chat refreshes the bubble the same way.
	f.PushEvent(Event{Kind: EventPollVote, PollVote: &PollVote{ChatJID: chatJID, PollMsgID: pollMsgID}})
	return nil
}

func (f *Fake) votePollLocked(chatJID, pollMsgID string, options []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	chosen := make(map[string]bool, len(options))
	for _, o := range options {
		chosen[o] = true
	}
	for _, msgs := range f.messages {
		for i := range msgs {
			if msgs[i].ID != pollMsgID || msgs[i].Poll == nil {
				continue
			}
			// Copy on write: snapshots handed out by Messages share the
			// old poll pointer and must keep the old tally, or the view
			// sees no change to redraw.
			poll := *msgs[i].Poll
			poll.Options = append([]PollOption(nil), poll.Options...)
			msgs[i].Poll = &poll
			for j := range poll.Options {
				opt := &poll.Options[j]
				want := chosen[opt.Name]
				if want && !opt.Voted {
					opt.Count++
				} else if !want && opt.Voted {
					opt.Count--
				}
				opt.Voted = want
			}
			return nil
		}
	}
	return fmt.Errorf("chatot/client: poll %q not found in chat %q", pollMsgID, chatJID)
}

func (f *Fake) EditMessage(ctx context.Context, chatJID, msgID, newText string) error {
	f.mu.Lock()
	msgs := f.messages[chatJID]
	for i := range msgs {
		if msgs[i].ID == msgID {
			msgs[i].Text = newText
			msgs[i].Edited = true
			echo := msgs[i]
			f.mu.Unlock()
			f.events.Publish(Event{Kind: EventMessage, Message: &echo})
			return nil
		}
	}
	f.mu.Unlock()
	return fmt.Errorf("chatot/client: message %q not found in chat %q", msgID, chatJID)
}

func (f *Fake) DeleteMessage(ctx context.Context, chatJID, msgID string) error {
	f.mu.Lock()
	msgs := f.messages[chatJID]
	for i := range msgs {
		if msgs[i].ID == msgID {
			msgs[i].Deleted = true
			echo := msgs[i]
			f.mu.Unlock()
			f.events.Publish(Event{Kind: EventRevoke, Revoke: &Revoke{ChatJID: chatJID, MsgID: msgID, TS: echo.TS}})
			return nil
		}
	}
	f.mu.Unlock()
	return fmt.Errorf("chatot/client: message %q not found in chat %q", msgID, chatJID)
}

func (f *Fake) React(ctx context.Context, jid, msgID, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.messages[jid]
	for i := range msgs {
		if msgs[i].ID == msgID {
			// One reaction per person: picking a new emoji replaces the old
			// one, and "" just removes it.
			msgs[i].Reactions = withReaction(msgs[i].Reactions, fakeOwnJID, emoji)
			return nil
		}
	}
	return fmt.Errorf("chatot/client: message %q not found in chat %q", msgID, jid)
}

func (f *Fake) DeleteMessageForMe(ctx context.Context, chatJID, msgID string) error {
	f.mu.Lock()
	msgs := f.messages[chatJID]
	for i := range msgs {
		if msgs[i].ID == msgID {
			f.messages[chatJID] = append(msgs[:i:i], msgs[i+1:]...)
			f.mu.Unlock()
			f.events.Publish(Event{Kind: EventRevoke, Revoke: &Revoke{ChatJID: chatJID, MsgID: msgID, TS: time.Now().Unix()}})
			f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: chatJID}})
			return nil
		}
	}
	f.mu.Unlock()
	return fmt.Errorf("chatot/client: message %q not found in chat %q", msgID, chatJID)
}

// MarkReadCall records one Fake.MarkRead, for tests of the read path.
type MarkReadCall struct {
	JID          string
	MsgIDs       []string
	NotifySender bool
}

func (f *Fake) MarkRead(ctx context.Context, jid string, msgIDs []string, notifySender bool) error {
	f.mu.Lock()
	f.markReads = append(f.markReads, MarkReadCall{JID: jid, MsgIDs: append([]string(nil), msgIDs...), NotifySender: notifySender})
	f.mu.Unlock()
	return f.ClearUnread(jid)
}

// MarkReadCalls lists every MarkRead so far, oldest first.
func (f *Fake) MarkReadCalls() []MarkReadCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]MarkReadCall(nil), f.markReads...)
}

func (f *Fake) StopLiveLocation(ctx context.Context, chatJID, msgID string) error {
	f.mu.Lock()
	msgs := f.messages[chatJID]
	for i := range msgs {
		if msgs[i].ID == msgID && msgs[i].Location != nil {
			loc := *msgs[i].Location
			loc.LiveUntil = time.Now().Unix()
			msgs[i].Location = &loc
		}
	}
	f.mu.Unlock()
	f.PushEvent(Event{Kind: EventReaction, Reaction: &Reaction{ChatJID: chatJID, MsgID: msgID}})
	return nil
}

func (f *Fake) ClearUnread(jid string) error {
	f.mu.Lock()
	changed := false
	for i := range f.chats {
		if f.chats[i].JID == jid {
			changed = f.chats[i].UnreadCount != 0
			f.chats[i].UnreadCount = 0
			break
		}
	}
	f.mu.Unlock()
	if changed {
		f.PushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	}
	return nil
}

func (f *Fake) PinChat(ctx context.Context, jid string, pin bool) error {
	f.mu.Lock()
	found := f.updateChat(jid, func(c *Chat) { c.Pinned = pin })
	f.mu.Unlock()
	if !found {
		return fmt.Errorf("chatot/client: chat %q not found", jid)
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) MuteChat(ctx context.Context, jid string, mute bool) error {
	f.mu.Lock()
	found := f.updateChat(jid, func(c *Chat) { c.Muted = mute })
	f.mu.Unlock()
	if !found {
		return fmt.Errorf("chatot/client: chat %q not found", jid)
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// MuteChatFor is MuteChat(true): the fake keeps no timer, so a timed mute
// simply mutes.
func (f *Fake) MuteChatFor(ctx context.Context, jid string, d time.Duration) error {
	return f.MuteChat(ctx, jid, true)
}

func (f *Fake) ArchiveChat(ctx context.Context, jid string, archive bool) error {
	f.mu.Lock()
	found := f.updateChat(jid, func(c *Chat) { c.Archived = archive })
	f.mu.Unlock()
	if !found {
		return fmt.Errorf("chatot/client: chat %q not found", jid)
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) MarkChatUnread(ctx context.Context, jid string, unread bool) error {
	f.mu.Lock()
	found := f.updateChat(jid, func(c *Chat) {
		if unread {
			if c.UnreadCount < 1 {
				c.UnreadCount = 1
			}
		} else {
			c.UnreadCount = 0
		}
	})
	f.mu.Unlock()
	if !found {
		return fmt.Errorf("chatot/client: chat %q not found", jid)
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// updateChat applies fn to the chat matching jid; callers hold f.mu.
func (f *Fake) updateChat(jid string, fn func(*Chat)) bool {
	for i := range f.chats {
		if f.chats[i].JID == jid {
			fn(&f.chats[i])
			return true
		}
	}
	return false
}

// StarMessage sets msgID's starred flag and publishes the same
// reaction-style reload the real client uses to refresh an open thread.
func (f *Fake) StarMessage(ctx context.Context, chatJID, msgID string, starred bool) error {
	f.mu.Lock()
	msgs := f.messages[chatJID]
	for i := range msgs {
		if msgs[i].ID == msgID {
			msgs[i].Starred = starred
			f.mu.Unlock()
			f.events.Publish(Event{Kind: EventReaction, Reaction: &Reaction{ChatJID: chatJID, MsgID: msgID}})
			return nil
		}
	}
	f.mu.Unlock()
	return fmt.Errorf("chatot/client: message %q not found in chat %q", msgID, chatJID)
}

// StarredMessages scans every chat's messages for starred ones, newest first.
func (f *Fake) StarredMessages(limit int) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Message
	for _, msgs := range f.messages {
		for _, m := range msgs {
			if m.Starred {
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// statusBroadcastJID is the special chat every status ("story") message
// belongs to (mirrors types.StatusBroadcastJID.String()).
const statusBroadcastJID = "status@broadcast"

// Statuses returns the seeded/posted status messages, newest first.
func (f *Fake) Statuses(limit int) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.messages[statusBroadcastJID]
	out := make([]Message, 0, len(src))
	for _, m := range src {
		// A deleted status is gone, not a tombstone in the feed.
		if !m.Deleted {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// PostStatus appends a text status to the in-memory broadcast and returns
// nil (the real client would SendMessage to the status broadcast).
func (f *Fake) PostStatus(ctx context.Context, text string) error {
	f.mu.Lock()
	f.nextID++
	msg := Message{
		ID:      fmt.Sprintf("status-%d", f.nextID),
		ChatJID: statusBroadcastJID,
		FromJID: fakeOwnJID,
		FromMe:  true,
		Text:    text,
		TS:      time.Now().Unix(),
	}
	f.messages[statusBroadcastJID] = append(f.messages[statusBroadcastJID], msg)
	f.mu.Unlock()
	return nil
}

// ChatMedia returns jid's seeded image/video attachments, newest first.
func (f *Fake) ChatMedia(jid string) ([]MediaItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []MediaItem
	for _, m := range f.messages[jid] {
		if m.Attachment == nil || (m.Attachment.Kind != "image" && m.Attachment.Kind != "video") || m.Attachment.ViewOnce {
			continue
		}
		out = append(out, MediaItem{
			MsgID: m.ID, Kind: m.Attachment.Kind, MimeType: m.Attachment.MimeType,
			LocalPath: m.Attachment.LocalPath, Thumbnail: m.Attachment.Thumbnail, TS: m.TS,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out, nil
}

// ChatDocs returns jid's seeded document attachments, newest first.
func (f *Fake) ChatDocs(jid string) ([]DocItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []DocItem
	for _, m := range f.messages[jid] {
		if m.Attachment == nil || m.Attachment.Kind != "document" {
			continue
		}
		out = append(out, DocItem{
			MsgID: m.ID, Filename: m.Attachment.Filename, MimeType: m.Attachment.MimeType,
			LocalPath: m.Attachment.LocalPath, TS: m.TS,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out, nil
}

// ChatLinks returns jid's seeded messages containing a URL, newest first.
func (f *Fake) ChatLinks(jid string) ([]LinkItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []LinkItem
	for _, m := range f.messages[jid] {
		urls := store.ExtractURLs(m.Text)
		if len(urls) == 0 {
			continue
		}
		out = append(out, LinkItem{MsgID: m.ID, URL: urls[0], Host: store.URLHost(urls[0]), Title: m.Text, TS: m.TS})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out, nil
}

// Newsletters returns the seeded channels, sorted by name for deterministic
// output.
func (f *Fake) Newsletters(ctx context.Context) ([]Newsletter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Newsletter, 0, len(f.newsletters))
	for _, n := range f.newsletters {
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// NewsletterMessages returns up to count posts for channel jid, newest first.
func (f *Fake) NewsletterMessages(ctx context.Context, jid string, count int) ([]NewsletterMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.newsletterMsgs[jid]
	out := make([]NewsletterMessage, len(src))
	copy(out, src)
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	if count > 0 && len(out) > count {
		out = out[:count]
	}
	for i := range out {
		out[i].MyReaction = f.newsletterMine[jid+"/"+out[i].ID]
	}
	return out, nil
}

// FollowNewsletter adds a channel to the in-memory set (seeding a placeholder
// if unknown) and pushes a refresh event.
func (f *Fake) FollowNewsletter(ctx context.Context, jid string) error {
	f.mu.Lock()
	if _, ok := f.newsletters[jid]; !ok {
		if d, ok := f.directory[jid]; ok {
			n := *d
			n.Following = true
			f.newsletters[jid] = &n
		} else {
			f.newsletters[jid] = &Newsletter{ID: jid, Name: jid, Description: "", Following: true}
		}
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// UnfollowNewsletter removes a channel from the in-memory set.
func (f *Fake) UnfollowNewsletter(ctx context.Context, jid string) error {
	f.mu.Lock()
	delete(f.newsletters, jid)
	delete(f.newsletterMsgs, jid)
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// NewsletterSetMuted flips a channel's muted flag.
func (f *Fake) NewsletterSetMuted(ctx context.Context, jid string, mute bool) error {
	f.mu.Lock()
	if n, ok := f.newsletters[jid]; ok {
		n.Muted = mute
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// NewsletterReact sets our reaction on the named post the way the server
// does: one per post, so a new emoji replaces the previous one's count and
// "" withdraws it (a no-op if the channel/post is unknown).
func (f *Fake) NewsletterReact(ctx context.Context, jid, messageID string, serverID int64, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.newsletterMine == nil {
		f.newsletterMine = make(map[string]string)
	}
	key := jid + "/" + messageID
	msgs := f.newsletterMsgs[jid]
	for i := range msgs {
		if msgs[i].ID != messageID {
			continue
		}
		if msgs[i].Reactions == nil {
			msgs[i].Reactions = make(map[string]int)
		}
		if prev := f.newsletterMine[key]; prev != "" {
			if msgs[i].Reactions[prev] <= 1 {
				delete(msgs[i].Reactions, prev)
			} else {
				msgs[i].Reactions[prev]--
			}
		}
		if emoji != "" {
			msgs[i].Reactions[emoji]++
			f.newsletterMine[key] = emoji
		} else {
			delete(f.newsletterMine, key)
		}
		return nil
	}
	return nil
}

// FollowNewsletterByLink parses the invite key, follows a synthetic channel
// derived from it, and returns its JID.
func (f *Fake) FollowNewsletterByLink(ctx context.Context, link string) (string, error) {
	key := parseChannelInvite(link)
	if key == "" {
		return "", fmt.Errorf("chatot/client: empty channel invite key")
	}
	jid := key + "@newsletter"
	f.mu.Lock()
	if _, ok := f.newsletters[jid]; !ok {
		f.newsletters[jid] = &Newsletter{ID: jid, Name: "Channel " + key, Description: "Followed via link", Following: true, InviteCode: key}
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return jid, nil
}

// Blocklist returns the blocked JIDs, sorted for deterministic output.
func (f *Fake) Blocklist(ctx context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.blocked))
	for jid := range f.blocked {
		out = append(out, jid)
	}
	sort.Strings(out)
	return out, nil
}

func (f *Fake) SetBlocked(ctx context.Context, jid string, blocked bool) error {
	f.mu.Lock()
	if blocked {
		f.blocked[jid] = true
	} else {
		delete(f.blocked, jid)
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) IsBlocked(jid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocked[jid]
}

// PrivacySettings returns a canned settings map; the fake has no real
// account to read privacy settings from.
func (f *Fake) PrivacySettings(ctx context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.privacyLocked()))
	for k, v := range f.privacyLocked() {
		out[k] = v
	}
	return out, nil
}

// privacyLocked is the mutable privacy map, seeded on first use. Callers
// hold f.mu.
func (f *Fake) privacyLocked() map[string]string {
	if f.privacy == nil {
		f.privacy = map[string]string{
			"Group Add":     "all",
			"Last Seen":     "contacts",
			"Status":        "contacts",
			"Profile Photo": "all",
			"Read Receipts": "all",
			"Calls":         "all",
			"Online":        "match_last_seen",
			"Messages":      "all",
			"Defense Mode":  "off",
			"Stickers":      "contacts",
		}
	}
	return f.privacy
}

// SetPrivacySetting validates the pair like the real client and stores it.
func (f *Fake) SetPrivacySetting(ctx context.Context, name, value string) error {
	if _, _, err := privacySettingType(name, value); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.privacyLocked()[name] = value
	return nil
}

// GroupInfo returns the stored group for jid, seeding a canned one on first
// access; non-group jids error. Callers hold no lock.
func (f *Fake) GroupInfo(ctx context.Context, jid string) (*GroupInfo, error) {
	if !strings.HasSuffix(jid, "@g.us") {
		return nil, fmt.Errorf("chatot/client: %s is not a group", jid)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneGroupInfo(f.ensureGroupLocked(jid)), nil
}

// ensureGroupLocked returns the stored group for jid, seeding the canned
// "Weekend Trip" group on first access. Callers hold f.mu.
func (f *Fake) ensureGroupLocked(jid string) *GroupInfo {
	if g, ok := f.groups[jid]; ok {
		return g
	}
	g := &GroupInfo{
		JID:      jid,
		Name:     "Weekend Trip",
		Topic:    "Planning for the cabin trip",
		OwnerJID: fakeOwnJID,
		Participants: []GroupParticipant{
			{JID: fakeOwnJID, IsAdmin: true, IsSuperAdmin: true},
			{JID: "1112223333@s.whatsapp.net"},
		},
	}
	f.groups[jid] = g
	return g
}

// cloneGroupInfo returns a deep copy so callers can't mutate stored state.
func cloneGroupInfo(g *GroupInfo) *GroupInfo {
	out := *g
	out.Participants = make([]GroupParticipant, len(g.Participants))
	copy(out.Participants, g.Participants)
	return &out
}

// OwnJID returns the Fake's own user JID.
func (f *Fake) OwnJID() string { return fakeOwnJID }

// ContactName resolves a fixture person: the chat list's names plus the
// group participants the fixture threads mention.
func (f *Fake) ContactName(jid string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.chats {
		if c.JID == jid && !c.IsGroup {
			return c.Name
		}
	}
	return fakeParticipantNames[jid]
}

// fakeParticipantNames names people who appear in fixture groups without
// being chats of their own.
var fakeParticipantNames = map[string]string{
	"4445556666@s.whatsapp.net": "Linus Torvalds",
	"1998887777@s.whatsapp.net": "Ken Thompson",
	"7778889999@s.whatsapp.net": "Dennis Ritchie",
}

// OwnName is the fixture's profile name.
func (f *Fake) OwnName() string { return "Sezar" }

func (f *Fake) CreateGroup(ctx context.Context, name string, participantJIDs []string) (string, error) {
	if !validGroupName(name) {
		return "", fmt.Errorf("chatot/client: invalid group name %q (1-25 chars)", name)
	}
	f.mu.Lock()
	f.nextGroupN++
	jid := fmt.Sprintf("fake-group-%d@g.us", f.nextGroupN)
	parts := []GroupParticipant{{JID: fakeOwnJID, IsAdmin: true, IsSuperAdmin: true}}
	for _, p := range participantJIDs {
		parts = append(parts, GroupParticipant{JID: p})
	}
	f.groups[jid] = &GroupInfo{JID: jid, Name: name, OwnerJID: fakeOwnJID, Participants: parts}
	f.chats = append(f.chats, Chat{JID: jid, Name: name, IsGroup: true, LastMessageTS: time.Now().Unix()})
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return jid, nil
}

func (f *Fake) LeaveGroup(ctx context.Context, jid string) error {
	f.mu.Lock()
	delete(f.groups, jid)
	kept := f.chats[:0]
	for _, c := range f.chats {
		if c.JID != jid {
			kept = append(kept, c)
		}
	}
	f.chats = kept
	keptComms := f.communities[:0]
	for _, c := range f.communities {
		if c.JID != jid {
			keptComms = append(keptComms, c)
		}
	}
	f.communities = keptComms
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) UpdateGroupParticipants(ctx context.Context, jid string, participantJIDs []string, action string) error {
	if _, ok := mapParticipantAction(action); !ok {
		return fmt.Errorf("chatot/client: unknown participant action %q", action)
	}
	f.mu.Lock()
	g := f.ensureGroupLocked(jid)
	for _, pj := range participantJIDs {
		applyParticipantChange(g, pj, action)
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// applyParticipantChange mutates g's membership for one participant per the
// action. Callers hold f.mu.
func applyParticipantChange(g *GroupInfo, jid, action string) {
	idx := -1
	for i := range g.Participants {
		if g.Participants[i].JID == jid {
			idx = i
			break
		}
	}
	switch action {
	case "add":
		if idx == -1 {
			g.Participants = append(g.Participants, GroupParticipant{JID: jid})
		}
	case "remove":
		if idx != -1 {
			g.Participants = append(g.Participants[:idx], g.Participants[idx+1:]...)
		}
	case "promote":
		if idx != -1 {
			g.Participants[idx].IsAdmin = true
		}
	case "demote":
		if idx != -1 {
			g.Participants[idx].IsAdmin = false
			g.Participants[idx].IsSuperAdmin = false
		}
	}
}

func (f *Fake) SetGroupName(ctx context.Context, jid, name string) error {
	f.mu.Lock()
	g := f.ensureGroupLocked(jid)
	g.Name = name
	f.updateChat(jid, func(c *Chat) { c.Name = name })
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) SetGroupTopic(ctx context.Context, jid, topic string) error {
	f.mu.Lock()
	f.ensureGroupLocked(jid).Topic = topic
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) SetGroupAnnounce(ctx context.Context, jid string, announce bool) error {
	f.mu.Lock()
	f.ensureGroupLocked(jid).Announce = announce
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) SetGroupLocked(ctx context.Context, jid string, locked bool) error {
	f.mu.Lock()
	f.ensureGroupLocked(jid).Locked = locked
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// PinMessage only validates in the demo: a pin has no rendering of its own
// yet, so there is nothing to mutate.
func (f *Fake) PinMessage(ctx context.Context, chatJID, msgID string, pin bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages[chatJID] {
		if m.ID == msgID {
			return nil
		}
	}
	return fmt.Errorf("chatot/client: message %s not found in chat %s", msgID, chatJID)
}

// SetGroupPhoto accepts any non-empty image for the demo group.
func (f *Fake) SetGroupPhoto(ctx context.Context, jid string, jpeg []byte) error {
	if len(jpeg) == 0 {
		return fmt.Errorf("chatot/client: empty group photo")
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) SetGroupDisappearingTimer(ctx context.Context, jid string, seconds int64) error {
	f.mu.Lock()
	f.ensureGroupLocked(jid).DisappearingTimer = uint32(seconds)
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

func (f *Fake) GroupInviteLink(ctx context.Context, jid string, reset bool) (string, error) {
	code := "FAKEINVITECODE01234"
	if reset {
		code = "RESETINVITECODE5678"
	}
	return "https://chat.whatsapp.com/" + code, nil
}

func (f *Fake) JoinGroupWithLink(ctx context.Context, code string) (string, error) {
	c := parseInviteCode(code)
	// Community invites ("…/comm…") land in Communities, not just Chats.
	if strings.HasPrefix(strings.ToLower(c), "comm") {
		f.mu.Lock()
		f.nextGroupN++
		cjid := fmt.Sprintf("fake-community-%d@g.us", f.nextGroupN)
		f.addCommunityLocked(cjid, "Invited community", "You joined this community from an invite link. Its groups appear here as admins link them.", "1112223333@s.whatsapp.net", false)
		f.mu.Unlock()
		f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: cjid}})
		return cjid, nil
	}
	jid := "joined-" + c + "@g.us"
	f.mu.Lock()
	if _, ok := f.groups[jid]; !ok {
		f.groups[jid] = &GroupInfo{
			JID:      jid,
			Name:     "Joined Group",
			OwnerJID: "1112223333@s.whatsapp.net",
			Participants: []GroupParticipant{
				{JID: "1112223333@s.whatsapp.net", IsAdmin: true, IsSuperAdmin: true},
				{JID: fakeOwnJID},
			},
		}
		f.chats = append(f.chats, Chat{JID: jid, Name: "Joined Group", IsGroup: true, LastMessageTS: time.Now().Unix()})
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return jid, nil
}

func (f *Fake) CreateCommunity(ctx context.Context, name, description string) (string, error) {
	if !validGroupName(name) {
		return "", fmt.Errorf("chatot/client: invalid community name %q (1-25 chars)", name)
	}
	f.mu.Lock()
	f.nextGroupN++
	jid := fmt.Sprintf("fake-community-%d@g.us", f.nextGroupN)
	f.addCommunityLocked(jid, name, description, fakeOwnJID, true)
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return jid, nil
}

func (f *Fake) LinkGroupToCommunity(ctx context.Context, community, group string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.groups[community]; !ok {
		return fmt.Errorf("chatot/client: unknown community %q", community)
	}
	g, ok := f.groups[group]
	if !ok {
		return fmt.Errorf("chatot/client: unknown group %q", group)
	}
	for i := range f.communities {
		if f.communities[i].JID != community {
			continue
		}
		for _, cg := range f.communities[i].Groups {
			if cg.JID == group {
				return nil
			}
		}
		f.communities[i].Groups = append(f.communities[i].Groups, CommunityGroup{
			JID: group, Name: g.Name, Joined: true, MemberCount: len(g.Participants), Preview: "Linked just now",
		})
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: community}})
	return nil
}

// GroupJoinRequests returns jid's pending join requests, empty if none.
func (f *Fake) GroupJoinRequests(ctx context.Context, jid string) ([]JoinRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reqs := f.joinRequests[jid]
	out := make([]JoinRequest, len(reqs))
	copy(out, reqs)
	return out, nil
}

// ResolveGroupJoinRequest removes participantJID from jid's pending
// requests; approve only affects whether the participant is also added to
// the group's membership.
func (f *Fake) ResolveGroupJoinRequest(ctx context.Context, groupJID, participantJID string, approve bool) error {
	f.mu.Lock()
	reqs := f.joinRequests[groupJID]
	kept := reqs[:0]
	found := false
	for _, r := range reqs {
		if r.JID == participantJID {
			found = true
			continue
		}
		kept = append(kept, r)
	}
	f.joinRequests[groupJID] = kept
	if found && approve {
		g := f.ensureGroupLocked(groupJID)
		g.Participants = append(g.Participants, GroupParticipant{JID: participantJID})
	}
	f.mu.Unlock()
	if !found {
		return fmt.Errorf("chatot/client: no pending join request from %q in %q", participantJID, groupJID)
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: groupJID}})
	return nil
}

// Labels returns the non-deleted labels.
func (f *Fake) Labels() ([]Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Label, len(f.labels))
	copy(out, f.labels)
	return out, nil
}

func (f *Fake) CreateLabel(ctx context.Context, name string, color int) (string, error) {
	f.mu.Lock()
	ids := make([]string, len(f.labels))
	for i, l := range f.labels {
		ids[i] = l.ID
	}
	id := nextLabelID(ids)
	f.labels = append(f.labels, Label{ID: id, Name: name, Color: color})
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventLabelUpdate})
	return id, nil
}

func (f *Fake) EditLabel(ctx context.Context, id, name string, color int) error {
	f.mu.Lock()
	for i := range f.labels {
		if f.labels[i].ID == id {
			f.labels[i].Name = name
			f.labels[i].Color = color
		}
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventLabelUpdate})
	return nil
}

func (f *Fake) DeleteLabel(ctx context.Context, id string) error {
	f.mu.Lock()
	kept := f.labels[:0]
	for _, l := range f.labels {
		if l.ID != id {
			kept = append(kept, l)
		}
	}
	f.labels = kept
	delete(f.labelChats, id)
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventLabelUpdate})
	return nil
}

func (f *Fake) SetChatLabeled(ctx context.Context, labelID, chatJID string, labeled bool) error {
	f.mu.Lock()
	if f.labelChats[labelID] == nil {
		f.labelChats[labelID] = make(map[string]bool)
	}
	if labeled {
		f.labelChats[labelID][chatJID] = true
	} else {
		delete(f.labelChats[labelID], chatJID)
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventLabelUpdate})
	return nil
}

func (f *Fake) LabelsForChat(chatJID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, l := range f.labels {
		if f.labelChats[l.ID][chatJID] {
			out = append(out, l.ID)
		}
	}
	return out, nil
}

// CheckOnWhatsApp treats any string of 7-15 digits (optionally "+"-prefixed)
// as registered, deriving a synthetic jid from its digits; anything else is
// reported as not on WhatsApp.
func (f *Fake) CheckOnWhatsApp(ctx context.Context, phone string) (string, bool, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if len(digits) < 7 || len(digits) > 15 {
		return "", false, nil
	}
	return digits + "@s.whatsapp.net", true, nil
}

func (f *Fake) SendPresence(available bool) error { return nil }

func (f *Fake) SendTyping(jid string, typing bool) error { return nil }

func (f *Fake) SendRecording(jid string, recording bool) error { return nil }

func (f *Fake) RejectCall(ctx context.Context, callJID, callID string) error { return nil }

// DownloadMedia simulates a successful download by writing an empty temp
// file and pointing the message's Attachment.LocalPath at it, so UI-level
// tests/dev builds can exercise the tap-to-load -> inline swap without a
// real WhatsApp link.
func (f *Fake) DownloadMedia(ctx context.Context, msgID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, msgs := range f.messages {
		for i := range msgs {
			if msgs[i].ID != msgID || msgs[i].Attachment == nil {
				continue
			}
			tmp, err := os.CreateTemp("", "chatot-fake-media-*")
			if err != nil {
				return "", err
			}
			_ = tmp.Close()
			msgs[i].Attachment.LocalPath = tmp.Name()
			return tmp.Name(), nil
		}
	}
	return "", fmt.Errorf("chatot/client: message %q not found for download", msgID)
}

// MarkViewOnceOpened marks msgID's attachment as viewed, mirroring the store
// tombstone the real client persists.
func (f *Fake) MarkViewOnceOpened(ctx context.Context, chatJID, msgID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.messages[chatJID]
	for i := range msgs {
		if msgs[i].ID == msgID && msgs[i].Attachment != nil {
			msgs[i].Attachment.Viewed = true
			return nil
		}
	}
	return fmt.Errorf("chatot/client: message %q not found in chat %q", msgID, chatJID)
}

// Avatar reports no profile picture for every jid — the mockup has no real
// avatar images to serve.
func (f *Fake) Avatar(ctx context.Context, jid string) (string, error) {
	return "", nil
}

// fakeMapThumbnail draws a stand-in for the map preview a real WhatsApp
// location message embeds: a pale tile with a couple of "roads" and a strip of
// water, so the location bubble's map rendering can be exercised without a
// live message. Rendered at 2x the bubble's 270×86 tile for HiDPI.
func fakeMapThumbnail() []byte {
	const w, h = 540, 172
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	land := color.RGBA{0xe8, 0xec, 0xe4, 0xff}
	road := color.RGBA{0xff, 0xff, 0xff, 0xff}
	water := color.RGBA{0xbf, 0xd9, 0xea, 0xff}
	block := color.RGBA{0xdc, 0xe2, 0xd6, 0xff}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := land
			switch {
			case y > 120 && y < 172 && x > 300:
				c = water
			case (x/70)%2 == 1 && (y/50)%2 == 0:
				c = block
			}
			// Two horizontal and three vertical roads.
			if (y > 40 && y < 48) || (y > 100 && y < 106) || (x > 120 && x < 128) || (x > 260 && x < 268) || (x > 420 && x < 426) {
				c = road
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// withReaction returns reactions with reactor's reaction set to emoji (or
// removed when emoji is ""), dropping whatever reactor had before. Emojis
// left with no reactors disappear, and a nil map stays nil when nothing was
// added.
func withReaction(reactions map[string][]string, reactor, emoji string) map[string][]string {
	for e, who := range reactions {
		kept := who[:0:0]
		for _, j := range who {
			if j != reactor {
				kept = append(kept, j)
			}
		}
		if len(kept) == 0 {
			delete(reactions, e)
		} else {
			reactions[e] = kept
		}
	}
	if emoji == "" {
		return reactions
	}
	if reactions == nil {
		reactions = make(map[string][]string)
	}
	reactions[emoji] = append(reactions[emoji], reactor)
	return reactions
}

// Stickers is the Fake's in-memory sticker library, most recent first.
func (f *Fake) Stickers() ([]Sticker, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Sticker(nil), f.stickers...), nil
}

// AddSticker records path as a library entry keyed by the path itself.
func (f *Fake) AddSticker(path string) (Sticker, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := Sticker{Key: "file:" + path, Path: path}
	for i, s := range f.stickers {
		if s.Key == st.Key {
			f.stickers = append(f.stickers[:i], f.stickers[i+1:]...)
			break
		}
	}
	f.stickers = append([]Sticker{st}, f.stickers...)
	return st, nil
}

// RemoveSticker drops the entry with key.
func (f *Fake) RemoveSticker(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, s := range f.stickers {
		if s.Key == key {
			f.stickers = append(f.stickers[:i], f.stickers[i+1:]...)
			return nil
		}
	}
	return nil
}
