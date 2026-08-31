package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestEventVM_Named(t *testing.T) {
	m := client.Message{EventInvite: &client.EventInvite{
		Name: "Team offsite", Location: "Bletchley Park", StartTS: 1735689600,
	}}
	v := eventVM(m)
	if !v.IsEvent {
		t.Fatal("expected IsEvent=true")
	}
	if v.Title != "Team offsite" {
		t.Errorf("Title = %q, want Team offsite", v.Title)
	}
	if v.Location != "Bletchley Park" {
		t.Errorf("Location = %q", v.Location)
	}
	if v.When != "Wed, 01 Jan 2025 00:00" {
		t.Errorf("When = %q, want Wed, 01 Jan 2025 00:00", v.When)
	}
}

func TestEventVM_UnnamedFallback(t *testing.T) {
	v := eventVM(client.Message{EventInvite: &client.EventInvite{StartTS: 1735689600}})
	if v.Title != "Event" {
		t.Errorf("Title = %q, want Event", v.Title)
	}
}

func TestEventVM_NoEvent(t *testing.T) {
	if eventVM(client.Message{Text: "hi"}).IsEvent {
		t.Error("expected IsEvent=false for a non-event message")
	}
}

func TestEventVM_Canceled(t *testing.T) {
	v := eventVM(client.Message{EventInvite: &client.EventInvite{Name: "Standup", Canceled: true}})
	if !v.Canceled {
		t.Error("expected Canceled=true")
	}
}

func TestEventWhenText(t *testing.T) {
	if got := eventWhenText(0, 0); got != "" {
		t.Errorf("eventWhenText(0,0) = %q, want empty", got)
	}
	start := int64(1735689600) // 2025-01-01 00:00:00 UTC
	if got := eventWhenText(start, 0); got != "Wed, 01 Jan 2025 00:00" {
		t.Errorf("eventWhenText(start,0) = %q", got)
	}
	sameDayEnd := start + 3600
	if got := eventWhenText(start, sameDayEnd); got != "Wed, 01 Jan 2025 00:00 - 01:00" {
		t.Errorf("eventWhenText(start,sameDayEnd) = %q", got)
	}
	nextDayEnd := start + 86400
	if got := eventWhenText(start, nextDayEnd); got != "Wed, 01 Jan 2025 00:00 - Thu, 02 Jan 2025 00:00" {
		t.Errorf("eventWhenText(start,nextDayEnd) = %q", got)
	}
}

func TestBubbleVM_EventMessage(t *testing.T) {
	m := client.Message{ID: "1", EventInvite: &client.EventInvite{Name: "Team offsite", StartTS: 1735689600}}
	out := bubbleVM(m, nil, nil, mustParse(t, "2026-08-30 12:00:00"))
	if !out.IsEvent {
		t.Fatal("expected IsEvent=true")
	}
	if out.IsMedia {
		t.Error("expected IsMedia=false for an event message")
	}
	if out.Event.Title != "Team offsite" {
		t.Errorf("Event.Title = %q, want Team offsite", out.Event.Title)
	}
}
