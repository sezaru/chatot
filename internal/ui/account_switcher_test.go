package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestAccountRowVMActiveCheck(t *testing.T) {
	metas := []client.AccountMeta{
		{ID: "personal", Name: "Sezar (personal)", Status: "Connected"},
		{ID: "work", Name: "Work", Status: "Logged out · scan to relink"},
	}
	got := []bool{
		accountRowVM(metas[0], "work").Active,
		accountRowVM(metas[1], "work").Active,
	}
	want := []bool{false, true}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("account %d active = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAccountRowVMUnreadBadge(t *testing.T) {
	cases := []struct {
		unread     int
		showUnread bool
		text       string
	}{
		{0, false, ""},
		{3, true, "3"},
		{99, true, "99"},
		{150, true, "99+"},
	}
	for _, c := range cases {
		vm := accountRowVM(client.AccountMeta{ID: "x", Name: "X", Unread: c.unread}, "x")
		if vm.ShowUnread != c.showUnread || vm.UnreadText != c.text {
			t.Errorf("unread %d: got show=%v text=%q, want show=%v text=%q",
				c.unread, vm.ShowUnread, vm.UnreadText, c.showUnread, c.text)
		}
	}
}

func TestAccountRowVMFields(t *testing.T) {
	vm := accountRowVM(client.AccountMeta{ID: "work", Name: "Work", Status: "Connected"}, "work")
	if vm.Initial != "W" {
		t.Errorf("Initial = %q, want %q", vm.Initial, "W")
	}
	if vm.Status != "Connected" {
		t.Errorf("Status = %q, want %q", vm.Status, "Connected")
	}
}
