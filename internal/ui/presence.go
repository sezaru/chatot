package ui

import (
	"fmt"
	"sort"
	"strings"
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
	// Composers names who is typing or recording, in a group: the
	// subtitle says "Ana is typing…" there. Empty in a direct chat, where
	// the peer is the only one who can be.
	Composers []string
}

// presenceSubtitle renders p's display text for the conversation header, in
// priority order: recording beats typing beats online beats last-seen beats
// nothing. now is injected so the last-seen relative text is deterministic
// in tests.
func presenceSubtitle(p PresenceState, now time.Time) string {
	switch {
	case p.Recording:
		return composingText("recording", p.Composers)
	case p.Typing:
		return composingText("typing", p.Composers)
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

// composingText is the typing notice for kind ("recording" for a voice
// note, anything else plain typing) by the named composers: a direct chat
// has none and reads "typing…"; a group names who, WhatsApp's way: "Ana is
// typing…", "Ana and Bia are typing…", "Ana, Bia and Cid are typing…".
func composingText(kind string, names []string) string {
	verb := "typing…"
	if kind == "recording" {
		verb = "recording audio…"
	}
	switch len(names) {
	case 0:
		return verb
	case 1:
		return names[0] + " is " + verb
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " are " + verb
	}
}

// composers is who is composing in which chat: chat JID → sender JID →
// "typing" or "recording". A direct chat has at most its peer; a group can
// have several people at once, each ending their own burst (a paused
// notice, a stale timer, their message) without touching the others'.
type composers map[string]map[string]string

// set records sender's kind in chat; an empty kind ends their burst.
func (c composers) set(chat, sender, kind string) {
	if kind == "" {
		delete(c[chat], sender)
		if len(c[chat]) == 0 {
			delete(c, chat)
		}
		return
	}
	if c[chat] == nil {
		c[chat] = make(map[string]string)
	}
	c[chat][sender] = kind
}

// clear ends every burst in chat (a message arrived, or the chat closed).
func (c composers) clear(chat string) { delete(c, chat) }

// kind is chat's notice: "" when no one is composing, "recording" when
// everyone who is is recording, else "typing".
func (c composers) kind(chat string) string {
	kind := ""
	for _, k := range c[chat] {
		if k == "typing" {
			return "typing"
		}
		kind = k
	}
	return kind
}

// senders lists who is composing in chat, in a stable order.
func (c composers) senders(chat string) []string {
	out := make([]string, 0, len(c[chat]))
	for s := range c[chat] {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// composingKind is a raw client.ChatPresence's notice: "recording" for a
// voice note in the making, "typing" for any other composing, "" for a
// paused one.
func composingKind(state, media string) string {
	switch typing, recording := chatPresenceTypingRecording(state, media); {
	case recording:
		return "recording"
	case typing:
		return "typing"
	}
	return ""
}

// composingKey is the stale-timer counter's key for sender's burst in chat.
func composingKey(chat, sender string) string { return chat + "\x00" + sender }
