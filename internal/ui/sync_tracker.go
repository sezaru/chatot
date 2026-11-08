package ui

import (
	"fmt"
	"time"

	"chatot/internal/client"
)

// syncPhase is where the post-link history backfill stands.
type syncPhase int

const (
	// syncIdle: nothing streaming.
	syncIdle syncPhase = iota
	// syncBlocking: the phone is still sending the chat list and recent
	// messages, so the main view would come up empty or half-filled; the
	// window shows the syncing screen instead.
	syncBlocking
	// syncBackground: recent chats are in and usable; the older history is
	// still arriving in "full" chunks behind a sidebar banner.
	syncBackground
)

const (
	// syncBlockingQuiet: once the chat list and recent messages have landed,
	// this long without a further chunk means the phone is done with the
	// blocking part (the "full" chunks, when they come, follow within
	// seconds and hand over to the banner).
	syncBlockingQuiet = 8 * time.Second
	// syncBlockingMax bounds the syncing screen no matter what the phone
	// sends: a stalled sync must not lock the user out of the cached store.
	syncBlockingMax = 3 * time.Minute
	// syncBackgroundQuiet retires the banner when "full" chunks stop without
	// ever reporting 100%.
	syncBackgroundQuiet = 2 * time.Minute
	// syncCatchUpQuiet retires the catch-up banner when the server's
	// end-of-backlog marker never comes.
	syncCatchUpQuiet = time.Minute
)

// catchUp is the backlog the server replays on connect: what arrived while
// chatot was closed or offline. It runs beside the post-link phases.
type catchUp struct {
	active   bool
	total    int
	messages int
	received int
	last     time.Time
}

// SyncTracker folds pairing, history-sync and connect-time backlog events
// into a phase plus the running counts the syncing screen and banner
// display. Pure: the caller supplies the clock and reacts to the phase it
// reads back.
type SyncTracker struct {
	phase     syncPhase
	chats     int
	messages  int
	percent   int // "full" progress 0-100, -1 while unknown
	gotRecent bool
	started   time.Time
	lastChunk time.Time
	catchUp   catchUp
}

// Pair starts the blocking phase: the account was just linked and the
// backlog is about to stream.
func (t *SyncTracker) Pair(now time.Time) {
	*t = SyncTracker{phase: syncBlocking, percent: -1, started: now, lastChunk: now, catchUp: t.catchUp}
}

// Offline accounts for one connect-time backlog event. A backlog with
// nothing in it shows nothing.
func (t *SyncTracker) Offline(o *client.OfflineSync, now time.Time) {
	if o == nil {
		return
	}
	if o.Done || o.Total == 0 {
		t.catchUp = catchUp{}
		return
	}
	t.catchUp = catchUp{active: true, total: o.Total, messages: o.Messages, received: o.Received, last: now}
}

// Chunk accounts for one history-sync event. Chunks that aren't part of the
// message backlog (push names, status, on-demand pages, synthetic
// refreshes) are ignored.
func (t *SyncTracker) Chunk(h *client.HistorySync, now time.Time) {
	if h == nil {
		return
	}
	switch h.Type {
	case "bootstrap", "recent", "full":
	default:
		return
	}
	if t.phase == syncIdle {
		// A backfill still streaming after a restart mid-sync, or one the
		// phone re-sent: the store is usable, so only the banner shows.
		*t = SyncTracker{phase: syncBackground, percent: -1, started: now, catchUp: t.catchUp}
	}
	t.chats += h.Chats
	t.messages += h.Messages
	t.lastChunk = now
	if h.Type != "full" {
		t.gotRecent = true
		return
	}
	if t.phase == syncBlocking {
		t.phase = syncBackground
	}
	if h.Progress >= 0 {
		t.percent = h.Progress
	}
	if h.Progress >= 100 {
		t.phase = syncIdle
	}
}

// Tick applies the time-based exits; it reports whether the phase changed.
func (t *SyncTracker) Tick(now time.Time) bool {
	changed := false
	if t.catchUp.active && now.Sub(t.catchUp.last) >= syncCatchUpQuiet {
		t.catchUp = catchUp{}
		changed = true
	}
	switch t.phase {
	case syncBlocking:
		quiet := t.gotRecent && now.Sub(t.lastChunk) >= syncBlockingQuiet
		if quiet || now.Sub(t.started) >= syncBlockingMax {
			t.phase = syncIdle
			return true
		}
	case syncBackground:
		if now.Sub(t.lastChunk) >= syncBackgroundQuiet {
			t.phase = syncIdle
			return true
		}
	}
	return changed
}

// Blocking reports whether the syncing screen should cover the main view.
func (t *SyncTracker) Blocking() bool { return t.phase == syncBlocking }

// Background reports whether the sidebar banner should show.
func (t *SyncTracker) Background() bool { return t.phase == syncBackground || t.catchUp.active }

// Counts is the "N chats · M messages" line under the syncing screen's
// spinner; empty until anything has arrived.
func (t *SyncTracker) Counts() string {
	if t.chats == 0 && t.messages == 0 {
		return ""
	}
	return fmt.Sprintf("%s · %s", plural(t.chats, "chat"), plural(t.messages, "message"))
}

// BannerText is the background banner's label. The older history, when it
// streams, outranks the catch-up: it is the longer of the two.
func (t *SyncTracker) BannerText() string {
	if t.phase != syncBackground && t.catchUp.active {
		if c := t.catchUp; c.messages > 0 {
			return fmt.Sprintf("Catching up · %s of %s", groupThousands(min(c.received, c.messages)), plural(c.messages, "message"))
		}
		return "Catching up…"
	}
	if t.percent < 0 {
		return "Syncing older messages…"
	}
	return fmt.Sprintf("Syncing older messages · %d%%", t.percent)
}

// Fraction is the banner's progress-bar fill, -1 for indeterminate.
func (t *SyncTracker) Fraction() float64 {
	if t.phase != syncBackground && t.catchUp.active {
		if c := t.catchUp; c.messages > 0 {
			return float64(min(c.received, c.messages)) / float64(c.messages)
		}
		return -1
	}
	if t.percent < 0 {
		return -1
	}
	return float64(t.percent) / 100
}

// plural renders "1 chat" / "4,213 messages".
func plural(n int, noun string) string {
	s := groupThousands(n)
	if n == 1 {
		return s + " " + noun
	}
	return s + " " + noun + "s"
}
