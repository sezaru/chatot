package client

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
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
	nextID   int
}

// NewFake returns a Fake seeded with canned chats and messages, already
// logged in (no QR codes will be emitted).
func NewFake() *Fake {
	now := time.Now().Unix()
	f := &Fake{
		messages: make(map[string][]Message),
		events:   newEventBus(nil),
		qrCodes:  make(chan string, 1),
		loggedIn: true,
	}

	f.chats = []Chat{
		{JID: "1234567890@s.whatsapp.net", Name: "Ada Lovelace", Preview: "See you tomorrow!", UnreadCount: 2, LastMessageTS: now - 60, Pinned: true},
		{JID: "1112223333@s.whatsapp.net", Name: "Grace Hopper", Preview: "Bug found in the relay", UnreadCount: 0, LastMessageTS: now - 3600},
	}

	f.messages["1234567890@s.whatsapp.net"] = []Message{
		{ID: "m1", ChatJID: "1234567890@s.whatsapp.net", FromJID: "1234567890@s.whatsapp.net", FromMe: false, Text: "Hey, are we still on for tomorrow?", TS: now - 120},
		{ID: "m2", ChatJID: "1234567890@s.whatsapp.net", FromJID: "me", FromMe: true, Text: "Yep!", TS: now - 90},
		{ID: "m3", ChatJID: "1234567890@s.whatsapp.net", FromJID: "1234567890@s.whatsapp.net", FromMe: false, Text: "See you tomorrow!", TS: now - 60},
	}
	f.messages["1112223333@s.whatsapp.net"] = []Message{
		{ID: "m4", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, Text: "Bug found in the relay", TS: now - 3600},
		{ID: "m5", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 3000,
			Location: &Location{Name: "Bletchley Park", Address: "Sherwood Dr, Bletchley, Milton Keynes", Latitude: 51.9976, Longitude: -0.7406}},
		{ID: "m6", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2900,
			Contact: &Contact{DisplayName: "Alan Turing", Phones: []string{"+44 20 7946 0958"}}},
		{ID: "m7", ChatJID: "1112223333@s.whatsapp.net", FromJID: "1112223333@s.whatsapp.net", FromMe: false, TS: now - 2800,
			Poll: &Poll{Name: "Lunch tomorrow?", SelectableCount: 1, Options: []PollOption{
				{Name: "Pizza", Count: 1},
				{Name: "Sushi"},
				{Name: "Salad"},
			}}},
	}

	return f
}

func (f *Fake) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loggedIn = true
	return nil
}

func (f *Fake) QRCodes() <-chan string { return f.qrCodes }

func (f *Fake) LoggedIn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loggedIn
}

func (f *Fake) Logout(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loggedIn = false
	return nil
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

func (f *Fake) SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextMsgID()
	att := Attachment{Kind: "audio", MimeType: "audio/ogg", Data: oggOpus}
	f.appendOutbound(jid, Message{ID: id, ChatJID: jid, FromJID: "me", FromMe: true, TS: time.Now().Unix(), Attachment: &att})
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
			f.chats[i].Preview = "📊 Poll"
			f.chats[i].LastMessageTS = msg.TS
			break
		}
	}
	return id, nil
}

func (f *Fake) VotePoll(ctx context.Context, chatJID, pollMsgID string, options []string) error {
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
			for j := range msgs[i].Poll.Options {
				opt := &msgs[i].Poll.Options[j]
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

func (f *Fake) React(ctx context.Context, jid, msgID, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.messages[jid]
	for i := range msgs {
		if msgs[i].ID == msgID {
			if emoji == "" {
				for k, v := range msgs[i].Reactions {
					if v == "me" {
						delete(msgs[i].Reactions, k)
					}
				}
				return nil
			}
			if msgs[i].Reactions == nil {
				msgs[i].Reactions = make(map[string]string)
			}
			msgs[i].Reactions[emoji] = "me"
			return nil
		}
	}
	return fmt.Errorf("chatot/client: message %q not found in chat %q", msgID, jid)
}

func (f *Fake) MarkRead(ctx context.Context, jid string, msgIDs []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].UnreadCount = 0
			break
		}
	}
	return nil
}

func (f *Fake) SendPresence(available bool) error { return nil }

func (f *Fake) SendTyping(jid string, typing bool) error { return nil }

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
