package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestListEmptyStateFor(t *testing.T) {
	// A query wins over every other reason, and the action clears it.
	got := listEmptyStateFor("zzz", true, true)
	if got.Text != "No chats match “zzz”" || got.Action != "Clear search" {
		t.Errorf("query = %+v", got)
	}
	// Then a filter chip…
	got = listEmptyStateFor("", true, true)
	if got.Action != "Show all chats" {
		t.Errorf("filtered = %+v", got)
	}
	// …then archived mode…
	got = listEmptyStateFor("", false, true)
	if got.Text != "No archived chats" || got.Action != "Back to chats" {
		t.Errorf("archived = %+v", got)
	}
	// …and finally a genuinely empty account.
	got = listEmptyStateFor("", false, false)
	if got.Text != "No chats yet" || got.Action != "Start a chat" {
		t.Errorf("empty = %+v", got)
	}
}

func TestMergedPreview(t *testing.T) {
	if got, want := mergedPreview("Work", "See you tomorrow"), "Work · See you tomorrow"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
	// A parenthesised qualifier is dropped so the prefix stays short.
	if got, want := mergedPreview("Sezar (personal)", "Hi"), "Sezar · Hi"; got != want {
		t.Errorf("qualified = %q, want %q", got, want)
	}
	// A chat with no preview yet shows just the account, not a dangling "· ".
	if got, want := mergedPreview("Work", ""), "Work"; got != want {
		t.Errorf("no preview = %q, want %q", got, want)
	}
	if got, want := mergedPreview("", "Hi"), "Hi"; got != want {
		t.Errorf("no account = %q, want %q", got, want)
	}
}

func TestMergedHeader(t *testing.T) {
	name, sub := mergedHeader(3)
	if name != "All accounts" || sub != "3 accounts · merged list" {
		t.Errorf("= (%q, %q)", name, sub)
	}
	if _, sub := mergedHeader(1); sub != "1 account · merged list" {
		t.Errorf("singular = %q", sub)
	}
}

func TestMatchContacts(t *testing.T) {
	contacts := []client.Chat{
		{JID: "a@s.whatsapp.net", Name: "Ada Lovelace"},
		{JID: "g@s.whatsapp.net", Name: "Grace Hopper"},
	}
	if got := matchContacts(contacts, ""); len(got) != 2 {
		t.Errorf("empty query returned %d", len(got))
	}
	// Case-insensitive substring, not prefix.
	got := matchContacts(contacts, "hopper")
	if len(got) != 1 || got[0].Name != "Grace Hopper" {
		t.Errorf("= %+v", got)
	}
	if got := matchContacts(contacts, "zzz"); len(got) != 0 {
		t.Errorf("no match returned %d", len(got))
	}
}

func TestParticipantsCaption(t *testing.T) {
	if got, want := participantsCaption(1), "1 PARTICIPANT"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
	if got, want := participantsCaption(4), "4 PARTICIPANTS"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
}

func TestContactInfoSub(t *testing.T) {
	if got, want := contactInfoSub("442079460958@s.whatsapp.net"), "+442079460958"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
	// A device-suffixed JID still resolves to the bare number.
	if got, want := contactInfoSub("442079460958:12@s.whatsapp.net"), "+442079460958"; got != want {
		t.Errorf("device suffix = %q, want %q", got, want)
	}
	// A group (or anything non-numeric) has no number to show, so the card
	// renders no subline rather than a raw JID.
	if got := contactInfoSub("weekendtrip@g.us"); got != "" {
		t.Errorf("group = %q, want empty", got)
	}
	if got := contactInfoSub("notanumber@s.whatsapp.net"); got != "" {
		t.Errorf("non-numeric = %q, want empty", got)
	}
	if got := contactInfoSub(""); got != "" {
		t.Errorf("empty = %q, want empty", got)
	}
}

func TestPrefPagesMatchMockup(t *testing.T) {
	want := []string{"Appearance", "Notifications", "Privacy", "Network", "Shortcuts", "Advanced"}
	got := prefPagesLabels()
	if len(got) != len(want) {
		t.Fatalf("prefPages has %d pages, want %d", len(got), len(want))
	}
	for i, label := range want {
		if got[i] != label {
			t.Errorf("page %d = %q, want %q", i, got[i], label)
		}
	}
}

func TestAboutVersionLine(t *testing.T) {
	if got, want := aboutVersionLine(), aboutVersion+" · GTK4 · libadwaita"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
}

func TestMergedChatsSpanAccountsAndResolvePerAccountClients(t *testing.T) {
	// The merged list must (a) contain every account's chats and (b) hand back
	// the owning account id, because every per-row lookup has to go through
	// that account's own client rather than the manager's active-account proxy.
	m := client.NewAccountManager()
	m.AddAccount("personal", "Sezar (personal)", client.NewFake())
	m.AddAccount("work", "Work", client.NewFake())

	rows := m.MergedChats(0)
	if len(rows) == 0 {
		t.Fatal("MergedChats returned nothing")
	}

	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.AccountID] = true
		if r.AccountName == "" {
			t.Errorf("row %q has no account name", r.JID)
		}
		if c := m.ClientFor(r.AccountID); c == nil {
			t.Errorf("no client for account %q", r.AccountID)
		}
	}
	if !seen["personal"] || !seen["work"] {
		t.Errorf("merged list covers only %v, want both accounts", seen)
	}

	// Pinned chats sort ahead of the rest, across accounts.
	pinnedEnded := false
	for _, r := range rows {
		if !r.Pinned {
			pinnedEnded = true
			continue
		}
		if pinnedEnded {
			t.Error("a pinned chat sorted after an unpinned one")
			break
		}
	}

	if m.ClientFor("nosuchaccount") != nil {
		t.Error("ClientFor(unknown) returned a client")
	}
}
