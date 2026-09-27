package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

// audioBlob is a marshaled AudioMessage carrying a mimetype (field 2) and,
// when ptt, the PTT flag (field 6).
func audioBlob(ptt bool) []byte {
	b := protowire.AppendTag(nil, 2, protowire.BytesType)
	b = protowire.AppendString(b, "audio/ogg; codecs=opus")
	if ptt {
		b = protowire.AppendTag(b, 6, protowire.VarintType)
		b = protowire.AppendVarint(b, 1)
	}
	b = protowire.AppendTag(b, 9, protowire.BytesType)
	return protowire.AppendString(b, "/v/t62/enc")
}

func TestAudioBlobIsVoice(t *testing.T) {
	if !audioBlobIsVoice(audioBlob(true)) {
		t.Error("PTT blob read as not a voice note")
	}
	if audioBlobIsVoice(audioBlob(false)) {
		t.Error("blob without PTT read as a voice note")
	}
	if audioBlobIsVoice([]byte{0xff}) {
		t.Error("garbage read as a voice note")
	}
}

func TestMessagesMediaVoiceFlagRoundTrips(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "note", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "song", TS: 2}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "note", Kind: "audio", Voice: true}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "song", Kind: "audio", MimeType: "audio/mpeg"}))

	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	if len(msgs) != 2 || !msgs[0].Attachment.Voice || msgs[1].Attachment.Voice {
		t.Fatalf("got note %+v, song %+v; want only the note flagged Voice", msgs[0].Attachment, msgs[1].Attachment)
	}
}

func TestOpenBackfillsVoiceFromStoredDescriptors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatot.db")
	s, err := Open(path)
	must(t, err)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	for _, id := range []string{"note", "song"} {
		must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: id, TS: 1}))
		must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: id, Kind: "audio", ProtoBlob: audioBlob(id == "note")}))
	}
	must(t, s.Close())

	// A database from before the column: the flag has to come from the blobs.
	raw, err := sql.Open("sqlite3", path)
	must(t, err)
	_, err = raw.Exec(`ALTER TABLE media DROP COLUMN voice`)
	must(t, err)
	must(t, raw.Close())

	s, err = Open(path)
	must(t, err)
	t.Cleanup(func() { _ = s.Close() })
	msgs, err := s.Messages("a@s.whatsapp.net", 50)
	must(t, err)
	voice := map[string]bool{}
	for _, m := range msgs {
		voice[m.ID] = m.Attachment.Voice
	}
	if !voice["note"] || voice["song"] {
		t.Fatalf("voice flags after reopen = %v, want note only", voice)
	}
}
