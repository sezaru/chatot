package ui

import "testing"

// A note follows the reader out of its chat only while it plays, or when
// the mini-player itself paused it; one paused in its row stays behind.
func TestNowPlayingFollowsChatSwitch(t *testing.T) {
	idle := &nowPlayingNote{p: newPendingPlayer(10), mv: mediaView{ChatJID: "a@s.whatsapp.net"}}
	if nowPlayingFollows(nil, "b@s.whatsapp.net") {
		t.Fatal("nil note follows")
	}
	if !nowPlayingFollows(idle, "a@s.whatsapp.net") {
		t.Error("a note is kept while its own chat shows")
	}
	if nowPlayingFollows(idle, "b@s.whatsapp.net") {
		t.Error("a note paused in its row followed the reader out")
	}
	idle.held = true
	if !nowPlayingFollows(idle, "b@s.whatsapp.net") {
		t.Error("a note the mini-player paused was let go of")
	}
	playing := &nowPlayingNote{p: newPendingPlayer(10), mv: mediaView{ChatJID: "a@s.whatsapp.net"}}
	playing.p.wantPlay = true // a press waiting for its file counts as playing
	if !nowPlayingFollows(playing, "b@s.whatsapp.net") {
		t.Error("a playing note was let go of")
	}
}

func TestMiniPlayerShowsAwayFromTheNotesChat(t *testing.T) {
	np := &nowPlayingNote{p: newPendingPlayer(10), mv: mediaView{ChatJID: "a@s.whatsapp.net"}}
	if miniPlayerShows(nil, "b@s.whatsapp.net") {
		t.Error("shows with nothing playing")
	}
	if miniPlayerShows(np, "a@s.whatsapp.net") {
		t.Error("shows in the note's own chat, where its row is the player")
	}
	if !miniPlayerShows(np, "b@s.whatsapp.net") {
		t.Error("hidden in another chat")
	}
	if !miniPlayerShows(np, "") {
		t.Error("hidden with no chat open")
	}
}
