package client

import (
	"os"
	"path/filepath"
)

// seedGroupThread gives the fixture group a short thread with two senders
// and an @mention, so group bubbles (author lines, mention rendering, the
// unread separator) have something to show without a live account.
func (f *Fake) seedGroupThread(now int64) {
	const g = "weekendtrip@g.us"
	f.messages[g] = []Message{
		{ID: "g1", ChatJID: g, FromJID: "4445556666@s.whatsapp.net", TS: now - 7600, Text: "I can drive on Friday"},
		{ID: "g2", ChatJID: g, FromJID: "me", FromMe: true, TS: now - 7500, Status: MessageStatusRead,
			Text: "@4445556666 can you pick me up on the way?"},
		{ID: "g3", ChatJID: g, FromJID: "1112223333@s.whatsapp.net", TS: now - 7400,
			Text: "@1234567890 bring the board games!", ReplyTo: &MsgRef{ChatJID: g, MsgID: "g2"}},
		{ID: "g4", ChatJID: g, FromJID: "1112223333@s.whatsapp.net", TS: now - 7200, Text: "See everyone Friday!"},
	}
}

// seedDevMedia adds downloaded attachments to the second fixture chat from
// the files in dir (photo.jpg, clip.mp4, voice.ogg, demo.mp3, sample.pdf,
// notes.ods — any that exist), so the inline players, tiles and the
// attachment viewer can be exercised with real bytes. Dev/screenshot only:
// CHATOT_FAKE_MEDIA=<dir>.
func (f *Fake) seedDevMedia(dir string, now int64) {
	const jid = "1112223333@s.whatsapp.net"
	kinds := []struct {
		file, kind, mime, caption string
		secs                      int
		fromMe                    bool
	}{
		{"photo.jpg", "image", "image/jpeg", "Balcony faces the river", 0, false},
		{"clip.mp4", "video", "video/mp4", "", 6, false},
		{"voice.ogg", "audio", "audio/ogg; codecs=opus", "", 3, false},
		{"demo.mp3", "audio", "audio/mpeg", "", 24, true},
		{"sample.pdf", "document", "application/pdf", "", 0, false},
		{"notes.ods", "document", "application/vnd.oasis.opendocument.spreadsheet", "", 0, true},
	}
	ts := now - 1800
	for i, k := range kinds {
		path := filepath.Join(dir, k.file)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		ts += 60
		from := jid
		if k.fromMe {
			from = "me"
		}
		f.messages[jid] = append(f.messages[jid], Message{
			ID: "dm" + itoa(i), ChatJID: jid, FromJID: from, FromMe: k.fromMe, TS: ts, Status: MessageStatusRead,
			Attachment: &Attachment{
				Kind: k.kind, MimeType: k.mime, Filename: k.file, Caption: k.caption,
				LocalPath: path, Size: info.Size(), DurationSecs: k.secs,
			},
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
