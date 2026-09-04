package client

import "fmt"

// seedBig (CHATOT_FAKE_BIG=1) pads the fake account out to the size of a
// real one so scroll performance can be measured: a few hundred chats and a
// long thread in the first one. Nothing else about the fixtures changes.
func (f *Fake) seedBig(now int64) {
	const (
		chats = 200
		msgs  = 600
	)
	bodies := []string{
		"ok",
		"Sounds good, see you there.",
		"Can you send me the address again? I lost the message from last week and the map link expired.",
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.",
		"👍",
		"Meeting moved to 15:30",
	}
	for i := 0; i < chats; i++ {
		jid := fmt.Sprintf("5511%08d@s.whatsapp.net", 90000000+i)
		f.chats = append(f.chats, Chat{
			JID:           jid,
			Name:          fmt.Sprintf("Load Test %03d", i),
			Preview:       bodies[i%len(bodies)],
			UnreadCount:   i % 4,
			LastMessageTS: now - int64(i)*3600,
			Muted:         i%7 == 0,
			Pinned:        i < 2,
		})
		f.messages[jid] = []Message{{ID: fmt.Sprintf("big-%d", i), ChatJID: jid, FromJID: jid, Text: bodies[i%len(bodies)], TS: now - int64(i)*3600}}
	}
	const thread = "1234567890@s.whatsapp.net"
	existing := f.messages[thread]
	base := now - 86400*30
	older := make([]Message, 0, msgs)
	for i := 0; i < msgs; i++ {
		m := Message{
			ID:      fmt.Sprintf("big-thread-%d", i),
			ChatJID: thread,
			FromJID: thread,
			FromMe:  i%3 == 0,
			Text:    bodies[i%len(bodies)],
			TS:      base + int64(i)*600,
		}
		if m.FromMe {
			m.FromJID = fakeOwnJID
			m.Status = MessageStatusRead
		}
		if i%11 == 0 {
			m.Reactions = map[string][]string{"❤️": {thread}}
		}
		if i%17 == 0 && i > 0 {
			m.ReplyTo = &MsgRef{ChatJID: thread, MsgID: fmt.Sprintf("big-thread-%d", i-1)}
		}
		older = append(older, m)
	}
	f.messages[thread] = append(older, existing...)
}
