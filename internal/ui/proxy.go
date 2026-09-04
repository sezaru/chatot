package ui

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

// proxyTypes are the schemes the Network page offers, in display order.
// "" is a direct connection.
var proxyTypes = []struct{ Scheme, Label string }{
	{"", "None"},
	{"socks5", "SOCKS5"},
	{"http", "HTTP"},
}

// proxyTypeLabel is the display name for a scheme, "None" for anything
// unknown.
func proxyTypeLabel(scheme string) string {
	for _, t := range proxyTypes {
		if t.Scheme == scheme {
			return t.Label
		}
	}
	return "None"
}

// proxyParts splits a proxy URL into the pieces the page edits. A URL the
// app cannot parse reads as a direct connection.
func proxyParts(raw string) (scheme, host, port string) {
	if raw == "" {
		return "", "", ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", ""
	}
	return strings.ToLower(u.Scheme), u.Hostname(), u.Port()
}

// proxyURL joins the page's pieces back into the URL the client reads.
// No scheme or no host means a direct connection ("").
func proxyURL(scheme, host, port string) string {
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if scheme == "" || host == "" {
		return ""
	}
	addr := host
	if port != "" {
		addr = net.JoinHostPort(host, port)
	}
	return scheme + "://" + addr
}

// testProxyTarget is the endpoint a direct-connection test reaches for.
const testProxyTarget = "web.whatsapp.com:443"

// testProxy measures a TCP connect to the proxy (or, with no proxy, to
// WhatsApp's web endpoint) and reports how long it took.
func testProxy(ctx context.Context, raw string) (time.Duration, error) {
	scheme, host, port := proxyParts(raw)
	addr := testProxyTarget
	if scheme != "" {
		if port == "" {
			return 0, errors.New("the proxy needs a port")
		}
		addr = net.JoinHostPort(host, port)
	}
	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}
