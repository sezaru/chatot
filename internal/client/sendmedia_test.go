package client

import (
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestDetectAttachmentKind(t *testing.T) {
	cases := []struct {
		name     string
		mime     string
		filename string
		wantKind string
		wantType whatsmeow.MediaType
	}{
		{"png mime", "image/png", "photo.png", "image", whatsmeow.MediaImage},
		{"jpeg mime", "image/jpeg", "photo.jpg", "image", whatsmeow.MediaImage},
		{"mp4 mime", "video/mp4", "clip.mp4", "video", whatsmeow.MediaVideo},
		{"ogg mime", "audio/ogg", "note.ogg", "audio", whatsmeow.MediaAudio},
		{"pdf mime", "application/pdf", "report.pdf", "document", whatsmeow.MediaDocument},
		{"empty mime, pdf extension", "", "report.pdf", "document", whatsmeow.MediaDocument},
		{"empty mime, png extension", "", "photo.png", "image", whatsmeow.MediaImage},
		{"octet-stream, png extension", "application/octet-stream", "photo.png", "image", whatsmeow.MediaImage},
		{"empty mime, no filename", "", "", "document", whatsmeow.MediaDocument},
		{"mime with charset param", "image/png; charset=binary", "photo.png", "image", whatsmeow.MediaImage},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, mt := detectAttachmentKind(tc.mime, tc.filename)
			if kind != tc.wantKind {
				t.Errorf("detectAttachmentKind(%q, %q) kind = %q, want %q", tc.mime, tc.filename, kind, tc.wantKind)
			}
			if mt != tc.wantType {
				t.Errorf("detectAttachmentKind(%q, %q) mediaType = %q, want %q", tc.mime, tc.filename, mt, tc.wantType)
			}
		})
	}
}

func TestAttachmentFilename(t *testing.T) {
	cases := []struct {
		name string
		att  Attachment
		want string
	}{
		{"explicit filename wins", Attachment{Filename: "invoice.pdf", LocalPath: "/tmp/abc123"}, "invoice.pdf"},
		{"falls back to basename", Attachment{LocalPath: "/home/user/Downloads/report.pdf"}, "report.pdf"},
		{"generic fallback", Attachment{}, "file"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := attachmentFilename(tc.att); got != tc.want {
				t.Errorf("attachmentFilename(%+v) = %q, want %q", tc.att, got, tc.want)
			}
		})
	}
}
