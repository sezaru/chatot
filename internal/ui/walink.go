package ui

import (
	"net/url"
	"strings"
)

// ParseWhatsAppLink reads the chat a WhatsApp link points at: the
// whatsapp://send?phone=…&text=… URI the "open in app" button of
// api.whatsapp.com launches, and the wa.me / api.whatsapp.com /
// web.whatsapp.com https forms of the same link. phone comes back E.164
// ("+5548…"), text as typed; ok is false for anything else, a link with no
// usable number included.
func ParseWhatsAppLink(raw string) (phone, text string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", false
	}
	q := u.Query()
	number := q.Get("phone")
	switch strings.ToLower(u.Scheme) {
	case "whatsapp":
		// whatsapp://send?… parses with host "send", whatsapp:send?… with
		// an opaque part; the number sits in the query either way.
	case "https", "http":
		host := strings.ToLower(u.Host)
		switch host {
		case "wa.me", "www.wa.me":
			number = strings.Trim(u.Path, "/")
		case "api.whatsapp.com", "web.whatsapp.com", "www.whatsapp.com", "whatsapp.com":
		default:
			return "", "", false
		}
	default:
		return "", "", false
	}
	phone, ok = normalizePhone("+" + strings.TrimPrefix(strings.TrimSpace(number), "+"))
	if !ok {
		return "", "", false
	}
	return phone, q.Get("text"), true
}
