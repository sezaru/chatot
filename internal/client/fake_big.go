package client

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
)

// seedBig (CHATOT_FAKE_BIG=1) pads the fake account out to the size of a
// real one so scroll performance can be measured: a few hundred chats and a
// long thread in the first one, and a group of nothing but pictures.
// Nothing else about the fixtures changes.
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
	f.seedBigMedia(now)
}

// bigMediaJID is the CHATOT_FAKE_BIG chat that holds nothing but pictures
// and clips, as many as a long-lived group gathers, for the media page.
const bigMediaJID = "120363000000000099@g.us"

func (f *Fake) seedBigMedia(now int64) {
	const n = 1500
	f.chats = append(f.chats, Chat{JID: bigMediaJID, Name: "Load Test Media", Preview: "Photo", LastMessageTS: now - 60, IsGroup: true})
	msgs := make([]Message, 0, n)
	for i := 0; i < n; i++ {
		m := Message{
			ID:      fmt.Sprintf("big-media-%d", i),
			ChatJID: bigMediaJID,
			FromJID: fakeOwnJID,
			FromMe:  true,
			Status:  MessageStatusRead,
			// Newest a minute ago, about forty a day before that.
			TS:         now - 60 - int64(n-1-i)*2100,
			Attachment: &Attachment{Kind: "image", MimeType: "image/jpeg"},
		}
		if i%10 == 5 {
			m.Attachment = &Attachment{Kind: "video", MimeType: "video/mp4", DurationSecs: 5 + i%90}
		}
		// Some carry no thumbnail at all: those tiles keep their glyph.
		if i%7 != 0 {
			m.Attachment.Thumbnail = bigThumbnail(i)
		}
		msgs = append(msgs, m)
	}
	f.messages[bigMediaJID] = msgs
}

// bigThumbnail is a small JPEG stand-in for a picture's embedded preview:
// a two-colour gradient that differs with i, landscape or portrait.
func bigThumbnail(i int) []byte {
	w, h := 96, 72
	if i%3 == 0 {
		w, h = 72, 96
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	a := color.RGBA{uint8(40 + i*37%180), uint8(60 + i*53%160), uint8(90 + i*29%150), 0xff}
	b := color.RGBA{uint8(200 - i*13%120), uint8(120 + i*7%100), uint8(60 + i*17%120), 0xff}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x+y) / float64(w+h)
			img.Set(x, y, color.RGBA{
				uint8(float64(a.R)*(1-t) + float64(b.R)*t),
				uint8(float64(a.G)*(1-t) + float64(b.G)*t),
				uint8(float64(a.B)*(1-t) + float64(b.B)*t),
				0xff,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil
	}
	return buf.Bytes()
}
