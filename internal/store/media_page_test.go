package store

import (
	"reflect"
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
