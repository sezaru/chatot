package ui

import (
	"strings"
	"testing"

	"chatot/internal/client"
)

func TestContactVM_Named(t *testing.T) {
	m := client.Message{Contact: &client.Contact{DisplayName: "Alan Turing", Phones: []string{"+44 7900 000000"}}}
	v := contactVM(m)
	if !v.IsContact {
		t.Fatal("expected IsContact=true")
	}
	if v.Name != "Alan Turing" {
		t.Errorf("Name = %q, want Alan Turing", v.Name)
	}
	if len(v.Phones) != 1 || v.Phones[0] != "+44 7900 000000" {
		t.Errorf("Phones = %v", v.Phones)
	}
}

func TestContactVM_EmptyNameFallback(t *testing.T) {
	v := contactVM(client.Message{Contact: &client.Contact{Phones: []string{"+1"}}})
	if v.Name != "Contact" {
		t.Errorf("Name = %q, want Contact", v.Name)
	}
}

func TestContactVM_MultiplePhones(t *testing.T) {
	v := contactVM(client.Message{Contact: &client.Contact{DisplayName: "Ada", Phones: []string{"+1", "+2"}}})
	if len(v.Phones) != 2 {
		t.Errorf("Phones = %v, want 2 entries", v.Phones)
	}
}

func TestContactVM_NoContact(t *testing.T) {
	if contactVM(client.Message{Text: "hi"}).IsContact {
		t.Error("expected IsContact=false for a non-contact message")
	}
}

func TestBubbleVM_ContactMessage(t *testing.T) {
	m := client.Message{ID: "1", Contact: &client.Contact{DisplayName: "Alan Turing", Phones: []string{"+44 7900 000000"}}}
	out := bubbleVM(m, nil, nil, mustParse(t, "2026-08-30 12:00:00"))
	if !out.IsContact {
		t.Fatal("expected IsContact=true")
	}
	if out.IsLocation || out.IsMedia {
		t.Error("expected IsLocation=false and IsMedia=false for a contact message")
	}
	if out.Contact.Name != "Alan Turing" {
		t.Errorf("Contact.Name = %q, want Alan Turing", out.Contact.Name)
	}
}

func TestContactChatJID(t *testing.T) {
	if got, ok := contactChatJID([]string{"+44 20 7946 0958"}); !ok || got != "442079460958@s.whatsapp.net" {
		t.Errorf("contactChatJID = (%q, %v)", got, ok)
	}
	// The first *dialable* number wins, not simply the first entry.
	if got, ok := contactChatJID([]string{"ext. 4471", "+351 912 345 678"}); !ok || got != "351912345678@s.whatsapp.net" {
		t.Errorf("contactChatJID skip = (%q, %v)", got, ok)
	}
	// No usable number: the caller renders the action inert rather than
	// opening a chat with a malformed JID.
	if _, ok := contactChatJID(nil); ok {
		t.Error("contactChatJID(nil) reported ok")
	}
	if _, ok := contactChatJID([]string{"call the office"}); ok {
		t.Error("contactChatJID(non-number) reported ok")
	}
	// A card written without a waid carries its number as bare digits, no
	// "+" in front. The same contact shared twice showed up both ways, and
	// only the copy with the "+" could be messaged.
	for _, bare := range []string{"554899010873", "595983590139", "55 48 9901-0873"} {
		got, ok := contactChatJID([]string{bare})
		if !ok {
			t.Errorf("contactChatJID(%q) reported not ok", bare)
			continue
		}
		if want := strings.ReplaceAll(strings.ReplaceAll(bare, " ", ""), "-", "") + "@s.whatsapp.net"; got != want {
			t.Errorf("contactChatJID(%q) = %q, want %q", bare, got, want)
		}
	}
	// Too short to be carrying a country code: a national number must not
	// be guessed into one.
	for _, national := range []string{"99010873", "4471", "2079460958"} {
		if _, ok := contactChatJID([]string{national}); ok {
			t.Errorf("contactChatJID(%q) reported ok; too short to be international", national)
		}
	}
}
