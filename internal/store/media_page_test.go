package store

import (
	"reflect"
	"strconv"
	"testing"
)

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"no url", "just some plain text", nil},
		{"single https", "check this out https://example.com/foo", []string{"https://example.com/foo"}},
		{"bare www", "see www.example.com for details.", []string{"www.example.com"}},
		{"multiple", "https://a.example.com and https://b.example.com/path", []string{"https://a.example.com", "https://b.example.com/path"}},
		{"trailing punctuation stripped", "link: https://example.com/cabin/4412).", []string{"https://example.com/cabin/4412"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractURLs(tt.text)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractURLs(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestURLHost(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://stay.example.com/cabin/4412", "stay.example.com"},
		{"www.example.com/path", "www.example.com"},
		{"http://example.com:8080/x", "example.com:8080"},
	}
	for _, tt := range tests {
		if got := URLHost(tt.url); got != tt.want {
			t.Errorf("URLHost(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestChatMediaDocsLinks(t *testing.T) {
	s := newTestStore(t)
	const jid = "chat@s.whatsapp.net"

	if err := s.UpsertMessage(MessageRow{ChatJID: jid, MsgID: "m1", TS: 100, Text: "hi"}); err != nil {
		t.Fatalf("UpsertMessage m1: %v", err)
	}
	if err := s.UpsertMedia(MediaRow{ChatJID: jid, MsgID: "m1", Kind: "image", MimeType: "image/jpeg"}); err != nil {
		t.Fatalf("UpsertMedia m1: %v", err)
	}

	if err := s.UpsertMessage(MessageRow{ChatJID: jid, MsgID: "m2", TS: 200, Text: ""}); err != nil {
		t.Fatalf("UpsertMessage m2: %v", err)
	}
	if err := s.UpsertMedia(MediaRow{ChatJID: jid, MsgID: "m2", Kind: "document", Filename: "lease.pdf", MimeType: "application/pdf"}); err != nil {
		t.Fatalf("UpsertMedia m2: %v", err)
	}

	if err := s.UpsertMessage(MessageRow{ChatJID: jid, MsgID: "m3", TS: 300, Text: "listing: https://stay.example.com/cabin/4412"}); err != nil {
		t.Fatalf("UpsertMessage m3: %v", err)
	}

	if err := s.UpsertMessage(MessageRow{ChatJID: jid, MsgID: "m4", TS: 400, Text: "no link here"}); err != nil {
		t.Fatalf("UpsertMessage m4: %v", err)
	}

	media, err := s.ChatMedia(jid)
	if err != nil {
		t.Fatalf("ChatMedia: %v", err)
	}
	if len(media) != 1 || media[0].MsgID != "m1" || media[0].Kind != "image" {
		t.Errorf("ChatMedia = %+v, want one image item m1", media)
	}

	docs, err := s.ChatDocs(jid)
	if err != nil {
		t.Fatalf("ChatDocs: %v", err)
	}
	if len(docs) != 1 || docs[0].MsgID != "m2" || docs[0].Filename != "lease.pdf" {
		t.Errorf("ChatDocs = %+v, want one doc item m2", docs)
	}

	links, err := s.ChatLinks(jid)
	if err != nil {
		t.Fatalf("ChatLinks: %v", err)
	}
	if len(links) != 1 || links[0].MsgID != "m3" || links[0].URL != "https://stay.example.com/cabin/4412" || links[0].Host != "stay.example.com" {
		t.Errorf("ChatLinks = %+v, want one link item m3", links)
	}
}

func TestSetMediaThumbnail(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net", Name: "A"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Kind: "video", Thumbnail: []byte{1}}))
	must(t, s.SetMediaThumbnail("a@s.whatsapp.net", "m1", []byte{2, 3}))
	msgs, err := s.Messages("a@s.whatsapp.net", 10)
	must(t, err)
	if len(msgs) != 1 || msgs[0].Attachment == nil || string(msgs[0].Attachment.Thumbnail) != "\x02\x03" {
		t.Fatalf("got %+v", msgs)
	}
}

func TestChatAttachments(t *testing.T) {
	s := newTestStore(t)
	const jid = "chat@s.whatsapp.net"
	add := func(id string, ts int64, kind string, viewOnce bool) {
		t.Helper()
		must(t, s.UpsertMessage(MessageRow{ChatJID: jid, MsgID: id, TS: ts}))
		if kind != "" {
			must(t, s.UpsertMedia(MediaRow{ChatJID: jid, MsgID: id, Kind: kind, MimeType: "x/y", Caption: "cap " + id, ViewOnce: viewOnce}))
		}
	}
	add("p1", 100, "image", false)
	add("v1", 200, "video", false)
	add("d1", 300, "document", false)
	add("once", 400, "image", true)
	add("gone", 500, "image", false)
	add("text", 600, "", false)
	add("p2", 700, "image", false)
	must(t, s.MarkMessageDeleted(jid, "gone", 500))
	must(t, s.UpsertReaction(ReactionRow{ChatJID: jid, MsgID: "p2", ReactorJID: "x@s.whatsapp.net", Emoji: "👍", TS: 701}))

	got, err := s.ChatAttachments(jid, "image", "video")
	if err != nil {
		t.Fatalf("ChatAttachments: %v", err)
	}
	var ids []string
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	// Oldest first; no document, view-once, deleted or plain text message.
	if want := []string{"p1", "v1", "p2"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ChatAttachments ids = %v, want %v", ids, want)
	}
	if got[0].Attachment == nil || got[0].Attachment.Caption != "cap p1" {
		t.Errorf("p1 attachment = %+v, want its caption", got[0].Attachment)
	}
	if len(got[2].Reactions["👍"]) != 1 {
		t.Errorf("p2 reactions = %v, want one 👍", got[2].Reactions)
	}

	docs, err := s.ChatAttachments(jid, "document")
	if err != nil || len(docs) != 1 || docs[0].ID != "d1" {
		t.Errorf("ChatAttachments(document) = %v, %v; want d1", docs, err)
	}
}

func TestReactionsForManyIDs(t *testing.T) {
	s := newTestStore(t)
	const jid = "chat@s.whatsapp.net"
	ids := make([]string, 2*reactionBatch+7)
	for i := range ids {
		ids[i] = "m" + strconv.Itoa(i)
	}
	must(t, s.UpsertReaction(ReactionRow{ChatJID: jid, MsgID: ids[len(ids)-1], ReactorJID: "x@s.whatsapp.net", Emoji: "🎉", TS: 1}))
	got, err := s.reactionsFor(jid, ids)
	if err != nil {
		t.Fatalf("reactionsFor: %v", err)
	}
	if len(got[ids[len(ids)-1]]["🎉"]) != 1 {
		t.Errorf("reactions in the last batch = %v, want one 🎉", got)
	}
}
