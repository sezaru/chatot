package ui

import (
	"fmt"
	"time"
)

// PresenceState is a contact or chat's live presence, as tracked by the UI
// from client.Events() (EventPresence / EventChatPresence). Kept GTK-free
// so presenceSubtitle can be unit-tested without a display.
type PresenceState struct {
	Online   bool
	LastSeen time.Time // zero if never seen
	Typing   bool
}

// presenceSubtitle renders p's display text for the conversation header,
// in priority order: typing beats online beats last-seen beats nothing.
// now is injected so the last-seen relative text is deterministic in tests.
func presenceSubtitle(p PresenceState, now time.Time) string {
	switch {
	case p.Typing:
		return "typing…"
	case p.Online:
		return "online"
	case !p.LastSeen.IsZero():
		return "last seen " + relativeTime(p.LastSeen, now)
	default:
		return ""
	}
}

// relativeTime renders t relative to now as "just now" / "Xm ago" / "Xh
// ago" / "Xd ago". t must not be after now (presence timestamps are always
// in the past); a negative delta is clamped to "just now" rather than
// showing a nonsensical negative duration.
func relativeTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
