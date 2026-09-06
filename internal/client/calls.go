package client

import (
	"strings"
	"time"

	"chatot/internal/store"
)

// CallText is WhatsApp's wording for a logged call: "Missed voice call",
// "Video call", "Declined voice call". Shared by the thread bubble, the
// chat-list preview and the notification so they never disagree.
func CallText(video bool, outcome string) string { return store.CallText(video, outcome) }

// ReactionText is the chat-list and notification line for someone reacting
// to one of our messages, WhatsApp's way: `Reacted 👍 to "hello"`. reactor
// prefixes the line in a group, where the chat's name alone doesn't say who
// reacted; "" in a DM.
func ReactionText(reactor, emoji, target string) string {
	body := "Reacted " + emoji + " to " + strings.TrimSpace(target)
	if target != "" && !strings.HasPrefix(target, `"`) {
		body = "Reacted " + emoji + ` to "` + strings.TrimSpace(target) + `"`
	}
	if reactor != "" {
		return reactor + ": " + body
	}
	return body
}

// staleCallAge is how old a call signal must be to count as replayed
// rather than live. An offer rings for well under a minute, so one stamped
// older than this was queued while this device was away (the machine
// asleep when the phone rang) and delivered on reconnect; it is logged in
// the thread but must not ring now.
const staleCallAge = 60 * time.Second

// callIsStale reports whether a call signal stamped ts is catch-up traffic
// rather than a live ring. A signal without a timestamp falls back to the
// post-connect sync window, like a message does.
func callIsStale(ts int64, now time.Time, syncing bool) bool {
	if ts == 0 {
		return syncing
	}
	return now.Sub(time.Unix(ts, 0)) > staleCallAge
}

// callMsgID is the thread row a live call is logged under. The phone's own
// record of the same call (a history-sync stub or a CallLogMessage) arrives
// under a message id of its own.
func callMsgID(callID string) string { return "call:" + callID }

// personName is the display name chatot knows for jid, falling back to the
// phone number for an unknown contact and to the bare user part for a LID
// that no number is known for.
func (w *Whatsmeow) personName(jid string) string {
	if name := w.ContactName(jid); name != "" {
		return name
	}
	user := jidUser(jid)
	if strings.HasSuffix(jid, "@lid") || user == "" {
		return user
	}
	return "+" + user
}
