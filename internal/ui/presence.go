package ui

import (
	"fmt"
	"time"
)

// PresenceState is a contact or chat's live presence, as tracked by the UI
// from client.Events() (EventPresence / EventChatPresence). Kept GTK-free
// so presenceSubtitle can be unit-tested without a display.
type PresenceState struct {
	Online    bool
	LastSeen  time.Time // zero if never seen
	Typing    bool
	Recording bool // true while the peer is recording a voice note
}

// presenceSubtitle renders p's display text for the conversation header, in
// priority order: recording beats typing beats online beats last-seen beats
// nothing. now is injected so the last-seen relative text is deterministic
// in tests.
func presenceSubtitle(p PresenceState, now time.Time) string {
	switch {
	case p.Recording:
		return "recording audio…"
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

// chatPresenceTypingRecording derives the typing/recording flags from a raw
// client.ChatPresence's State/Media strings: composing+audio media is a
// voice-note recording, any other composing is plain typing, anything else
// (paused) is neither.
func chatPresenceTypingRecording(state, media string) (typing, recording bool) {
	if state != "composing" {
		return false, false
	}
	if media == "audio" {
		return false, true
	}
	return true, false
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
