package ui

import "testing"

func TestProxyPartsAndURL(t *testing.T) {
	cases := []struct{ raw, scheme, host, port string }{
		{"", "", "", ""},
		{"socks5://127.0.0.1:9050", "socks5", "127.0.0.1", "9050"},
		{"http://proxy.example:3128", "http", "proxy.example", "3128"},
		{"HTTP://[::1]:8080", "http", "::1", "8080"},
		{"garbage", "", "", ""},
	}
	for _, c := range cases {
		s, h, p := proxyParts(c.raw)
		if s != c.scheme || h != c.host || p != c.port {
			t.Errorf("proxyParts(%q) = %q %q %q, want %q %q %q", c.raw, s, h, p, c.scheme, c.host, c.port)
		}
	}
	if got := proxyURL("socks5", "127.0.0.1", "9050"); got != "socks5://127.0.0.1:9050" {
		t.Errorf("proxyURL = %q", got)
	}
	if got := proxyURL("http", "::1", "8080"); got != "http://[::1]:8080" {
		t.Errorf("proxyURL v6 = %q", got)
	}
	if got := proxyURL("socks5", "", "9050"); got != "" {
		t.Errorf("proxyURL without host = %q, want direct", got)
	}
	if got := proxyURL("", "host", "1"); got != "" {
		t.Errorf("proxyURL without scheme = %q, want direct", got)
	}
}
