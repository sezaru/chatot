package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// previewInput is what the chat list's one-line preview is derived from:
// the newest message's kind/text/payload and, for a media message, its
// attachment row.
type previewInput struct {
	FromMe        bool
	Kind          string // messages.kind: "" plain/media, "location", "contact", "poll", "event"
	Text          string
	Payload       string // opaque JSON body of a rich kind
	MediaKind     string
	MediaCaption  string
	MediaFilename string
	MediaSeconds  int
	MediaIsGIF    bool
}

// buildPreview renders the chat list's preview line the way the design's
// preview() does: a glyph naming the message kind followed by what the user
// would want to read (caption, filename, place, question, voice length) —
// never a raw "[image]" placeholder. A from-me message is prefixed "You: ".
func buildPreview(in previewInput) string {
	body := in.Text
	switch in.Kind {
	case "location":
		body = "📍 " + firstNonEmpty(payloadString(in.Payload, "name"), locationNoun(in.Payload))
	case "contact":
		body = "👤 " + firstNonEmpty(payloadString(in.Payload, "name"), "Contact")
	case "poll":
		body = "📊 " + firstNonEmpty(payloadString(in.Payload, "name"), "Poll")
	case "event":
		body = "📅 " + firstNonEmpty(payloadString(in.Payload, "name"), "Event")
	case "call":
		// A call is logged as "Missed voice call", never "You: ..." — the
		// wording already says whose call it was.
		return callPreview(in.Payload)
	}
	if in.MediaKind != "" {
		body = mediaPreview(in)
	}
	if in.FromMe && body != "" {
		return "You: " + body
	}
	return body
}

// mediaPreview is the attachment half of buildPreview.
func mediaPreview(in previewInput) string {
	switch in.MediaKind {
	case "sticker":
		return "🙂 Sticker"
	case "image":
		return "📷 " + firstNonEmpty(in.MediaCaption, "Photo")
	case "video":
		if in.MediaIsGIF {
			return "🎞 " + firstNonEmpty(in.MediaCaption, "GIF")
		}
		return "🎥 " + firstNonEmpty(in.MediaCaption, "Video")
	case "audio":
		if in.MediaSeconds > 0 {
			return "🎤 " + clockText(in.MediaSeconds)
		}
		return "🎤 Voice message"
	case "document":
		return "📄 " + firstNonEmpty(in.MediaCaption, in.MediaFilename, "Document")
	}
	return "📎 " + firstNonEmpty(in.MediaCaption, in.MediaFilename, "Attachment")
}

// callPreview is the chat-list line for a logged call: the glyph for the
// call's medium and WhatsApp's wording for its outcome.
func callPreview(payload string) string {
	var p struct {
		Video   bool   `json:"video"`
		Outcome string `json:"outcome"`
	}
	_ = json.Unmarshal([]byte(payload), &p)
	glyph := "📞"
	if p.Video {
		glyph = "🎥"
	}
	return glyph + " " + CallText(p.Video, p.Outcome)
}

// CallText is WhatsApp's wording for a logged call: "Missed voice call",
// "Video call", "Declined voice call". Shared (through package client) by
// the chat-list preview, the thread bubble and the notification.
func CallText(video bool, outcome string) string {
	medium := "voice call"
	if video {
		medium = "video call"
	}
	switch outcome {
	case "missed":
		return "Missed " + medium
	case "declined":
		return "Declined " + medium
	case "failed":
		return "Failed " + medium
	}
	return strings.ToUpper(medium[:1]) + medium[1:]
}

// locationNoun tells a live share from a static point when the payload
// carries no place name.
func locationNoun(payload string) string {
	var p struct {
		IsLive bool `json:"live"`
	}
	if json.Unmarshal([]byte(payload), &p) == nil && p.IsLive {
		return "Live location"
	}
	return "Location"
}

// payloadString reads one string field out of a rich message's JSON
// payload, "" when absent or malformed.
func payloadString(payload, key string) string {
	if payload == "" {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return ""
	}
	var s string
	if raw, ok := m[key]; ok && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// clockText renders a playback length as "0:06" / "1:04:20".
func clockText(secs int) string {
	if secs < 3600 {
		return fmt.Sprintf("%d:%02d", secs/60, secs%60)
	}
	return fmt.Sprintf("%d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
