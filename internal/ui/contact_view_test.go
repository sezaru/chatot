package ui

import (
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
