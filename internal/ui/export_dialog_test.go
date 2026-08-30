package ui

import (
	"strings"
	"testing"

	"chatot/internal/client"
)

func TestFormatChatExportOrderingAndSenderLabels(t *testing.T) {
	msgs := []client.Message{
		{ID: "m1", FromJID: "them", Text: "hi there", TS: 1700000000},
		{ID: "m2", FromMe: true, Text: "hey!", TS: 1700000060},
	}
	out := formatChatExport(msgs, "Ada Lovelace")

	hiIdx := strings.Index(out, "hi there")
	heyIdx := strings.Index(out, "hey!")
	if hiIdx == -1 || heyIdx == -1 || hiIdx > heyIdx {
		t.Fatalf("expected messages in input order, got:\n%s", out)
	}
	if !strings.Contains(out, "Ada Lovelace: hi there") {
		t.Errorf("expected contact name as sender for inbound message, got:\n%s", out)
	}
	if !strings.Contains(out, "You: hey!") {
		t.Errorf("expected \"You\" as sender for FromMe message, got:\n%s", out)
	}
}

func TestFormatChatExportHeaderNamesTheChat(t *testing.T) {
	out := formatChatExport(nil, "Grace Hopper")
	if !strings.Contains(out, "Grace Hopper") {
		t.Errorf("expected header to name the chat, got:\n%s", out)
	}
}

func TestExportBodyMarkers(t *testing.T) {
	cases := []struct {
		name string
		msg  client.Message
		want string
	}{
		{"text", client.Message{Text: "plain text"}, "plain text"},
		{"deleted", client.Message{Deleted: true, Text: "gone"}, "[deleted]"},
		{"photo", client.Message{Attachment: &client.Attachment{Kind: "image"}}, "[Photo]"},
		{"video", client.Message{Attachment: &client.Attachment{Kind: "video"}}, "[Video]"},
		{"gif", client.Message{Attachment: &client.Attachment{Kind: "video", IsGIF: true}}, "[GIF]"},
		{"voice", client.Message{Attachment: &client.Attachment{Kind: "audio"}}, "[Voice message]"},
		{"document", client.Message{Attachment: &client.Attachment{Kind: "document", Filename: "invoice.pdf"}}, "[Document: invoice.pdf]"},
		{"document no name", client.Message{Attachment: &client.Attachment{Kind: "document"}}, "[Document]"},
		{"sticker", client.Message{Attachment: &client.Attachment{Kind: "sticker"}}, "[Sticker]"},
		{"location named", client.Message{Location: &client.Location{Name: "Home"}}, "[Location: Home]"},
		{"location unnamed", client.Message{Location: &client.Location{}}, "[Location]"},
		{"contact", client.Message{Contact: &client.Contact{DisplayName: "Bob"}}, "[Contact: Bob]"},
		{"poll", client.Message{Poll: &client.Poll{Name: "Lunch?"}}, "[Poll: Lunch?]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := exportBody(c.msg, "Contact")
			if got != c.want {
				t.Errorf("exportBody() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestExportBodyDeletedTakesPriorityOverAttachment(t *testing.T) {
	msg := client.Message{Deleted: true, Attachment: &client.Attachment{Kind: "image"}}
	if got := exportBody(msg, "Contact"); got != "[deleted]" {
		t.Errorf("exportBody() = %q, want [deleted] to take priority", got)
	}
}

func TestSlugForFilename(t *testing.T) {
	cases := map[string]string{
		"Ada Lovelace":  "ada-lovelace",
		"Grace  Hopper": "grace-hopper",
		"日本語":           "chat",
		"":              "chat",
		"O'Brien!!":     "o-brien",
	}
	for in, want := range cases {
		if got := slugForFilename(in); got != want {
			t.Errorf("slugForFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIncludeMediaSubtitle(t *testing.T) {
	if got := includeMediaSubtitle(0, 0, true); got != "No media in this chat" {
		t.Errorf("got %q for zero files", got)
	}
	if got := includeMediaSubtitle(1, 1024, true); got != "1 file · about 1 KB" {
		t.Errorf("got %q, want singular \"file\"", got)
	}
	if got := includeMediaSubtitle(48, 96*1<<20, true); got != "48 files · about 96 MB" {
		t.Errorf("got %q, want the mockup's 48 files/96 MB shape", got)
	}
	if got := includeMediaSubtitle(3, 0, false); got != "3 files" {
		t.Errorf("got %q, want count-only when size is unknown", got)
	}
}
