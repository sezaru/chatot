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
	only := NewFake()
	m.AddAccount("a", "A", only)

	// The last account can't be dropped (chatot always has one), so removing
	// it means signing it out: it stays, logged out, as the account to pair.
	if err := m.RemoveAccount("a"); err != nil {
		t.Fatalf("removing the last account should sign it out, got %v", err)
	}
	if only.LoggedIn() {
		t.Error("last account was not signed out")
	}
	if got := m.Count(); got != 1 {
		t.Fatalf("Count() after removing the last account = %d, want 1", got)
	}
	// Pretend it got linked again, so the next removal has a live session to
	// sign out rather than reusing the one just retired.
	only.SetLinked()

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
	// The removed account was signed out of WhatsApp, not just forgotten.
	if only.LoggedIn() {
		t.Error("removed account is still logged in")
	}
}

func TestRemoveDefaultAccountIsRemembered(t *testing.T) {
	m := NewAccountManager()
	m.AddAccount(defaultAccountID, "", NewFake())
	m.AddAccount("work", "Work", NewFake())
	if err := m.RemoveAccount(defaultAccountID); err != nil {
		t.Fatalf("RemoveAccount(default): %v", err)
	}
	if !m.defaultRemoved {
		t.Error("removing the default account must be recorded for the next launch")
	}

	// Next launch: main registers default first again; the roster drops it.
	m2 := NewAccountManager()
	m2.AddAccount(defaultAccountID, "", NewFake())
	m2.AddAccount("work", "Work", NewFake())
	m2.dropDefault("work")
	if got := m2.ActiveID(); got != "work" {
		t.Errorf("active after dropping default = %q, want work", got)
	}
	if metas := m2.Accounts(); len(metas) != 1 || metas[0].ID != "work" {
		t.Errorf("accounts after dropping default = %+v, want [work]", metas)
	}
	if !m2.defaultRemoved {
		t.Error("dropDefault must keep the removal flagged so the roster persists it")
	}
}

func TestRosterRoundTripsDefaultRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), rosterFile)
	if err := saveRoster(path, roster{DefaultRemoved: true, Accounts: []rosterEntry{{ID: "work", Label: "Work"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := loadRoster(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DefaultRemoved {
		t.Error("DefaultRemoved did not survive the round trip")
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
