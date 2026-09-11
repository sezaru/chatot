package ui

import "testing"

func TestParseWhatsAppLink(t *testing.T) {
	cases := []struct {
		in          string
		phone, text string
		ok          bool
	}{
		{"whatsapp://send/?phone=595992931997&text=ATLAS&type=phone_number&app_absent=0", "+595992931997", "ATLAS", true},
		{"whatsapp://send?phone=%2B55%2048%2099807-3648&text=Ol%C3%A1%20a%C3%AD", "+5548998073648", "Olá aí", true},
		{"whatsapp:send?phone=15551234567", "+15551234567", "", true},
		{"https://wa.me/15551234567?text=hi", "+15551234567", "hi", true},
		{"https://api.whatsapp.com/send/?phone=15551234567&text=hi", "+15551234567", "hi", true},
		{"https://web.whatsapp.com/send?phone=15551234567", "+15551234567", "", true},
		{"whatsapp://send?text=no%20number", "", "", false},
		{"https://example.com/send?phone=15551234567", "", "", false},
		{"mailto:someone@example.com", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		phone, text, ok := ParseWhatsAppLink(c.in)
		if phone != c.phone || text != c.text || ok != c.ok {
			t.Errorf("ParseWhatsAppLink(%q) = %q, %q, %v; want %q, %q, %v", c.in, phone, text, ok, c.phone, c.text, c.ok)
		}
	}
}

func TestJIDFallbackName(t *testing.T) {
	if got := JIDFallbackName("15559876543@s.whatsapp.net"); got != formatPhoneDisplay("+15559876543") {
		t.Errorf("phone JID = %q", got)
	}
	if got := JIDFallbackName("60795983540278@lid"); got != "60795983540278@lid" {
		t.Errorf("lid JID = %q", got)
	}
}
