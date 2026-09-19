package ui

import (
	"fmt"
	"testing"
)

// withEmptyVoicePlayers runs f over a fresh player cache and puts the real
// one back after, so one test's players do not leak into the next.
func withEmptyVoicePlayers(t *testing.T, f func()) {
	t.Helper()
	players, speeds, playing := voicePlayers, speedPlayers, nowPlaying
	voicePlayers, speedPlayers, nowPlaying = map[string]*mediaPlayer{}, map[*mediaPlayer]bool{}, nil
	defer func() { voicePlayers, speedPlayers, nowPlaying = players, speeds, playing }()
	f()
}

// A row's disc, track and time label all watch one player object. Dropping
// that object from the cache handed the row's next press a second player,
// which then played with none of the row's widgets following it: the note
// looked stuck — no pause, no moving knob, a dead seek.
func TestSharedVoicePlayer_KeepsThePlayerARowIsWatching(t *testing.T) {
	withEmptyVoicePlayers(t, func() {
		const watchedPath = "/tmp/chatot-test-watched.ogg"
		watched := sharedVoicePlayer(watchedPath, "audio/ogg", 3)
		unwatch := watched.Watch(func() {})
		idle := sharedVoicePlayer("/tmp/chatot-test-idle.ogg", "audio/ogg", 3)

		// Fill the cache past its cap, which sweeps the idle players.
		for i := 0; i < voicePlayersCap+2; i++ {
			sharedVoicePlayer(fmt.Sprintf("/tmp/chatot-test-%d.ogg", i), "audio/ogg", 3)
		}
		if got := sharedVoicePlayer(watchedPath, "audio/ogg", 3); got != watched {
			t.Error("the player a row is watching must survive the sweep")
		}
		if voicePlayers["/tmp/chatot-test-idle.ogg"] == idle {
			t.Error("an idle player nothing is showing is still swept")
		}
		// Once the row goes, so does the reason to keep it.
		unwatch()
		if watched.watched() {
			t.Error("dropping the last watcher leaves the player unwatched")
		}
	})
}

// playVoiceWith plays the caller's own player even when the cache has moved
// on, and files it so the next lookup — and the one-audio-at-a-time sweep —
// finds the object the row watches.
func TestPlayVoiceWith_FilesTheCallersPlayer(t *testing.T) {
	withEmptyVoicePlayers(t, func() {
		const path = "/tmp/chatot-test-rebound.ogg"
		row := newPendingPlayer(3)
		stale := sharedVoicePlayer(path, "audio/ogg", 3)
		if stale == row {
			t.Fatal("the cache should have made its own player")
		}
		got := playVoiceWith(row, mediaView{LocalPath: path, DurationSecs: 3}, voiceHooks{})
		if got != row {
			t.Errorf("playVoiceWith played %p, want the caller's %p", got, row)
		}
		if voicePlayers[path] != row {
			t.Error("the caller's player must be the one filed under its file")
		}
		if stale.Playing() {
			t.Error("the player it replaced must have been silenced")
		}
	})
}
