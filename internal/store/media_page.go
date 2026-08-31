package store

import (
	"net/url"
	"regexp"
	"strings"
)

// MediaItem is one image/video attachment in a chat, for the media/links/docs
// page's Media tab.
type MediaItem struct {
	MsgID     string
	Kind      string // "image" or "video"
	MimeType  string
	LocalPath string
	Thumbnail []byte
	TS        int64
}

// DocItem is one document attachment in a chat, for the Docs tab.
type DocItem struct {
	MsgID     string
	Filename  string
	MimeType  string
	LocalPath string
	TS        int64
}

// LinkItem is a message in a chat whose text contains a URL, for the Links
// tab: URL is the first URL found in Title (the message's own text).
type LinkItem struct {
	MsgID string
	URL   string
	Host  string
	Title string
	TS    int64
}

// ChatMedia returns jid's image/video attachments, newest first.
func (s *Store) ChatMedia(jid string) ([]MediaItem, error) {
	rows, err := s.db.Query(`
		SELECT md.msg_id, md.kind, COALESCE(md.mime_type, ''), COALESCE(md.local_path, ''), md.thumbnail, m.ts
		FROM media md JOIN messages m ON m.chat_jid = md.chat_jid AND m.msg_id = md.msg_id
		WHERE md.chat_jid = ? AND md.kind IN ('image', 'video') AND m.deleted = 0 AND md.view_once = 0
		ORDER BY m.ts DESC, m.rowid DESC
	`, jid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MediaItem
	for rows.Next() {
		var it MediaItem
		if err := rows.Scan(&it.MsgID, &it.Kind, &it.MimeType, &it.LocalPath, &it.Thumbnail, &it.TS); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ChatDocs returns jid's document attachments, newest first.
func (s *Store) ChatDocs(jid string) ([]DocItem, error) {
	rows, err := s.db.Query(`
		SELECT md.msg_id, COALESCE(md.filename, ''), COALESCE(md.mime_type, ''), COALESCE(md.local_path, ''), m.ts
		FROM media md JOIN messages m ON m.chat_jid = md.chat_jid AND m.msg_id = md.msg_id
		WHERE md.chat_jid = ? AND md.kind = 'document' AND m.deleted = 0
		ORDER BY m.ts DESC, m.rowid DESC
	`, jid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DocItem
	for rows.Next() {
		var it DocItem
		if err := rows.Scan(&it.MsgID, &it.Filename, &it.MimeType, &it.LocalPath, &it.TS); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ChatLinks returns jid's messages whose text contains a URL, newest first,
// one link per message (the first URL found in its text).
func (s *Store) ChatLinks(jid string) ([]LinkItem, error) {
	rows, err := s.db.Query(`
		SELECT msg_id, text, ts FROM messages
		WHERE chat_jid = ? AND deleted = 0 AND text IS NOT NULL AND text != ''
		ORDER BY ts DESC, rowid DESC
	`, jid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LinkItem
	for rows.Next() {
		var msgID, text string
		var ts int64
		if err := rows.Scan(&msgID, &text, &ts); err != nil {
			return nil, err
		}
		urls := ExtractURLs(text)
		if len(urls) == 0 {
			continue
		}
		out = append(out, LinkItem{MsgID: msgID, URL: urls[0], Host: URLHost(urls[0]), Title: text, TS: ts})
	}
	return out, rows.Err()
}

// urlPattern matches http(s):// and bare www. URLs, the two forms WhatsApp
// text messages commonly carry.
var urlPattern = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)

// ExtractURLs returns the URLs found in text, in the order they appear.
// Trailing punctuation a sentence would attach to a URL (".", ",", ")", …)
// is stripped so it doesn't leak into the result.
func ExtractURLs(text string) []string {
	matches := urlPattern.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimRight(m, ".,;:!?)]}\"'")
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

// URLHost returns rawURL's host (e.g. "stay.example.com"), assuming an
// https scheme for a bare "www."-prefixed URL so url.Parse can find it.
// Returns rawURL unchanged if it can't be parsed into a host.
func URLHost(rawURL string) string {
	parseable := rawURL
	if !strings.Contains(rawURL, "://") {
		parseable = "https://" + rawURL
	}
	u, err := url.Parse(parseable)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}
