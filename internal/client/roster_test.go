package client

import (
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Work":            "work",
		"  Bakery  ":      "bakery",
		"My Phone #2":     "my-phone-2",
		"Éü!!":            "account",
		"":                "account",
		"a--b":            "a-b",
		"UPPER lower 123": "upper-lower-123",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRosterRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	want := roster{Accounts: []rosterEntry{
		{ID: "work", Label: "Work", Proxy: "socks5://localhost:1080"},
		{ID: "bakery", Label: "Bakery (business)"},
	}}
	if err := saveRoster(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadRoster(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accounts) != len(want.Accounts) {
		t.Fatalf("round-trip length = %d, want %d", len(got.Accounts), len(want.Accounts))
	}
	for i, e := range want.Accounts {
		if got.Accounts[i] != e {
			t.Errorf("entry %d = %+v, want %+v", i, got.Accounts[i], e)
		}
	}
}

func TestLoadRosterMissingFile(t *testing.T) {
	got, err := loadRoster(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("missing roster should not error, got %v", err)
	}
	if len(got.Accounts) != 0 {
		t.Fatalf("missing roster should be empty, got %d accounts", len(got.Accounts))
	}
}

func TestAddPairingAccountFakeModeAddsDemoAccount(t *testing.T) {
	// Without a state dir there is no WhatsApp to pair with; in the demo build
	// (a Fake active client) the manager hands out a logged-out pairing Fake
	// that emits a demo QR, so the add-account card has something to render.
	m := NewAccountManager()
	m.AddAccount("default", "Default", NewFake())
	a, err := m.AddPairingAccount("Work")
	if err != nil {
		t.Fatalf("AddPairingAccount in fake mode: %v", err)
	}
	if a.LoggedIn() {
		t.Error("a pairing account should start logged out")
	}
	select {
	case code := <-a.QRCodes():
		if code == "" {
			t.Error("empty demo QR")
		}
	default:
		t.Error("pairing fake emitted no QR on start")
	}
	if got := m.Count(); got != 2 {
		t.Errorf("Count() = %d, want 2", got)
	}
	// It is the demo's only escape hatch: a manager whose active client is not
	// a Fake still refuses without a base dir.
	if err := m.RenameAccount(a.ID, "Renamed"); err != nil {
		t.Errorf("RenameAccount: %v", err)
	}
	if err := m.RenameAccount(a.ID, "  "); err == nil {
		t.Error("RenameAccount accepted a blank label")
	}
	for _, meta := range m.Accounts() {
		if meta.ID == a.ID && meta.Name != "Renamed" {
			t.Errorf("renamed account shows %q", meta.Name)
		}
	}
}

func TestRemoveAccountGuards(t *testing.T) {
	m := NewAccountManager()
	m.AddAccount("a", "A", NewFake())

	if err := m.RemoveAccount("a"); err == nil {
		t.Fatal("removing the last account should error, got nil")
	}

	m.AddAccount("b", "B", NewFake())
	if err := m.RemoveAccount("missing"); err == nil {
		t.Fatal("removing an unknown account should error, got nil")
	}

	// Removing the active account auto-switches to the other, then drops it.
	if m.ActiveID() != "a" {
		t.Fatalf("first-added should be active, got %q", m.ActiveID())
	}
	if err := m.RemoveAccount("a"); err != nil {
		t.Fatalf("removing active (with another present) should succeed, got %v", err)
	}
	if m.ActiveID() != "b" {
		t.Errorf("after removing active, active should switch to b, got %q", m.ActiveID())
	}
	if metas := m.Accounts(); len(metas) != 1 || metas[0].ID != "b" {
		t.Errorf("roster after remove = %+v, want single [b]", metas)
	}
}

func TestUniqueID(t *testing.T) {
	m := NewAccountManager()
	m.AddAccount("work", "Work", NewFake())
	if got := m.uniqueID("Work"); got != "work-2" {
		t.Errorf("uniqueID collision = %q, want %q", got, "work-2")
	}
	if got := m.uniqueID("Other"); got != "other" {
		t.Errorf("uniqueID no-collision = %q, want %q", got, "other")
	}
}
