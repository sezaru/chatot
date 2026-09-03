package store

import "testing"

func TestContactNameResolvesAndFallsBack(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertContact(ContactRow{JID: "111@s.whatsapp.net", PushName: "Grace"}))
	must(t, s.UpsertContact(ContactRow{JID: "999@lid", PNJID: "222@s.whatsapp.net"}))
	must(t, s.UpsertContact(ContactRow{JID: "222@s.whatsapp.net", FullName: "Ken Thompson"}))
	must(t, s.UpsertContact(ContactRow{JID: "777@lid", PNJID: "444@s.whatsapp.net", PushName: "Linus"}))
	must(t, s.UpsertContact(ContactRow{JID: "555@s.whatsapp.net", PushName: "+555"}))
	must(t, s.UpsertContact(ContactRow{JID: "666@s.whatsapp.net", PushName: "+55 48 9901-0873"}))
	cases := map[string]string{
		"111@s.whatsapp.net": "Grace",
		"333@s.whatsapp.net": "",             // a bare number is not a name
		"999@lid":            "Ken Thompson", // named on the phone-number twin
		"444@s.whatsapp.net": "Linus",        // named on the LID twin
		"555@s.whatsapp.net": "",             // a "+number" push name is no name
		"666@s.whatsapp.net": "",             // nor is a formatted one
		"unknown@lid":        "",
	}
	for jid, want := range cases {
		got, err := s.ContactName(jid)
		if err != nil {
			t.Fatalf("ContactName(%s): %v", jid, err)
		}
		if got != want {
			t.Errorf("ContactName(%s) = %q, want %q", jid, got, want)
		}
	}
}

func TestContactNameIgnoresDeviceSuffix(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertContact(ContactRow{JID: "999@lid", FullName: "Ken Thompson", PNJID: "111@s.whatsapp.net"}))
	name, err := s.ContactName("999:27@lid")
	must(t, err)
	if name != "Ken Thompson" {
		t.Errorf("device-addressed lookup = %q, want Ken Thompson", name)
	}
	name, err = s.ContactName("111:3@s.whatsapp.net")
	must(t, err)
	if name != "Ken Thompson" {
		t.Errorf("device-addressed twin lookup = %q, want Ken Thompson", name)
	}
}
