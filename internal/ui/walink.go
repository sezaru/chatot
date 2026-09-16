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

// ParseWhatsAppInvite reads the group invite code a WhatsApp link carries:
// the whatsapp://chat?code=… URI the "Join chat" button of a
// chat.whatsapp.com page launches (with or without a slash before the
// query), and the https://chat.whatsapp.com/<code> link itself. ok is
// false for anything else, a link with no code included.
func ParseWhatsAppInvite(raw string) (code string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "whatsapp":
		// whatsapp://chat?… parses with host "chat", whatsapp:chat?… with
		// an opaque part.
		if strings.ToLower(u.Host) != "chat" && strings.ToLower(strings.TrimSuffix(u.Opaque, "/")) != "chat" {
			return "", false
		}
		code = u.Query().Get("code")
	case "https", "http":
		if strings.ToLower(u.Host) != "chat.whatsapp.com" {
			return "", false
		}
		code = strings.Trim(u.Path, "/")
	default:
		return "", false
	}
	code = strings.TrimSpace(code)
	if code == "" || strings.ContainsAny(code, " \t/") {
		return "", false
	}
	return code, true
}
