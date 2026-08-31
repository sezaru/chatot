package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestBlockedContactRows(t *testing.T) {
	chats := []client.Chat{
		{JID: "111@s.whatsapp.net", Name: "Zed"},
		{JID: "222@s.whatsapp.net", Name: "Amy"},
	}
	blocked := []string{"111@s.whatsapp.net", "222@s.whatsapp.net", "333@s.whatsapp.net"}

	rows := blockedContactRows(blocked, chats)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	want := []blockedContactRow{
		{JID: "333@s.whatsapp.net", Name: "333@s.whatsapp.net"},
		{JID: "222@s.whatsapp.net", Name: "Amy"},
		{JID: "111@s.whatsapp.net", Name: "Zed"},
	}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], w)
		}
	}
}
