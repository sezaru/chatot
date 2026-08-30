package store

import (
	"sort"
	"testing"
)

func TestUpsertLabelAndLabelsExcludesDeleted(t *testing.T) {
	s := newTestStore(t)

	must(t, s.UpsertLabel("1", "Work", 3, false))
	must(t, s.UpsertLabel("2", "Family", 5, false))
	must(t, s.UpsertLabel("3", "Temp", 0, true)) // created already deleted

	labels, err := s.Labels()
	must(t, err)
	if len(labels) != 2 {
		t.Fatalf("Labels() len = %d, want 2 (%v)", len(labels), labels)
	}
	// Ordered by numeric id.
	if labels[0].ID != "1" || labels[0].Name != "Work" || labels[0].Color != 3 {
		t.Fatalf("labels[0] = %+v", labels[0])
	}
	if labels[1].ID != "2" || labels[1].Name != "Family" {
		t.Fatalf("labels[1] = %+v", labels[1])
	}

	// Rename + delete via upsert.
	must(t, s.UpsertLabel("1", "Job", 4, false))
	must(t, s.UpsertLabel("2", "Family", 5, true))
	labels, err = s.Labels()
	must(t, err)
	if len(labels) != 1 || labels[0].ID != "1" || labels[0].Name != "Job" || labels[0].Color != 4 {
		t.Fatalf("after edit/delete labels = %+v", labels)
	}
}

func TestChatLabelRoundTrip(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertLabel("1", "Work", 0, false))
	must(t, s.UpsertLabel("2", "Family", 0, false))

	a := "a@s.whatsapp.net"
	b := "b@s.whatsapp.net"

	must(t, s.SetChatLabel("1", a, true))
	must(t, s.SetChatLabel("1", b, true))
	must(t, s.SetChatLabel("2", a, true))
	must(t, s.SetChatLabel("1", a, true)) // INSERT OR IGNORE: idempotent

	ids, err := s.LabelsForChat(a)
	must(t, err)
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "1" || ids[1] != "2" {
		t.Fatalf("LabelsForChat(a) = %v, want [1 2]", ids)
	}

	chats, err := s.ChatsForLabel("1")
	must(t, err)
	sort.Strings(chats)
	if len(chats) != 2 || chats[0] != a || chats[1] != b {
		t.Fatalf("ChatsForLabel(1) = %v, want [%s %s]", chats, a, b)
	}

	// Remove one association.
	must(t, s.SetChatLabel("1", a, false))
	ids, err = s.LabelsForChat(a)
	must(t, err)
	if len(ids) != 1 || ids[0] != "2" {
		t.Fatalf("after remove LabelsForChat(a) = %v, want [2]", ids)
	}
	chats, err = s.ChatsForLabel("1")
	must(t, err)
	if len(chats) != 1 || chats[0] != b {
		t.Fatalf("after remove ChatsForLabel(1) = %v, want [%s]", chats, b)
	}
}
