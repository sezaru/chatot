package ui

import "testing"

func TestJoinRequestNaming(t *testing.T) {
	names := map[string]string{"1998887777@s.whatsapp.net": "Tomás Leal"}
	// A known contact: name on top, number underneath.
	if got := joinRequestName("1998887777@s.whatsapp.net", names); got != "Tomás Leal" {
		t.Errorf("name = %q", got)
	}
	if got := joinRequestSub("1998887777@s.whatsapp.net", names); got != "+1998887777" {
		t.Errorf("sub = %q", got)
	}
	// Unknown: the number is the title and there is no second line.
	if got := joinRequestName("5550001111@s.whatsapp.net", names); got != "+5550001111" {
		t.Errorf("unknown name = %q", got)
	}
	if got := joinRequestSub("5550001111@s.whatsapp.net", names); got != "" {
		t.Errorf("unknown sub = %q, want empty", got)
	}
	// Not a phone JID at all: shown raw rather than blank.
	if got := joinRequestName("weird@lid", nil); got != "weird@lid" {
		t.Errorf("raw = %q", got)
	}
}
