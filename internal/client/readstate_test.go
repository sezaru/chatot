package client

import "testing"

func TestGroupReadStatus(t *testing.T) {
	canon := func(jid string) string {
		switch jid {
		case "9001@lid":
			return "111"
		case "111@s.whatsapp.net":
			return "111"
		case "222@s.whatsapp.net":
			return "222"
		case "555@s.whatsapp.net", "9555@lid":
			return "555"
		}
		return jid
	}
	members := []string{"555@s.whatsapp.net", "111@s.whatsapp.net", "222@s.whatsapp.net"}
	own := []string{"555@s.whatsapp.net"}
	cases := []struct {
		name         string
		readers      []string
		participants []string
		want         int
	}{
		{"nobody read", nil, members, MessageStatusDelivered},
		{"one of two others", []string{"111@s.whatsapp.net"}, members, MessageStatusDelivered},
		{"everyone but self", []string{"111@s.whatsapp.net", "222@s.whatsapp.net"}, members, MessageStatusRead},
		{"reader under a LID", []string{"9001@lid", "222@s.whatsapp.net"}, members, MessageStatusRead},
		{"a reader who left does not count", []string{"111@s.whatsapp.net", "333@s.whatsapp.net"}, members, MessageStatusDelivered},
		{"self reading is not a reader", []string{"9555@lid", "111@s.whatsapp.net"}, members, MessageStatusDelivered},
		{"membership unknown", []string{"111@s.whatsapp.net"}, nil, MessageStatusDelivered},
	}
	for _, c := range cases {
		if got := groupReadStatus(c.readers, c.participants, own, canon); got != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestReadCoversChat(t *testing.T) {
	cases := []struct {
		rangeTS, lastTS int64
		want            bool
	}{
		{0, 100, true},   // no range: the whole chat
		{100, 100, true}, // read up to the newest message
		{100, 90, true},
		{100, 101, false}, // a message arrived after the read
	}
	for _, c := range cases {
		if got := readCoversChat(c.rangeTS, c.lastTS); got != c.want {
			t.Errorf("readCoversChat(%d, %d) = %v, want %v", c.rangeTS, c.lastTS, got, c.want)
		}
	}
}

func TestJIDUserIn(t *testing.T) {
	own := []string{"554888073648", "64081113427987"}
	if !jidUserIn("554888073648:48@s.whatsapp.net", own) || !jidUserIn("64081113427987@lid", own) {
		t.Fatal("own device not recognised")
	}
	if jidUserIn("999@lid", own) || jidUserIn("", own) {
		t.Fatal("stranger recognised as own")
	}
}
