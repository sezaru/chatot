package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func albumPhoto(id, from string, ts int64) client.Message {
	return client.Message{
		ID: id, ChatJID: "g@g.us", FromJID: from, FromMe: from == "me", TS: ts,
		Status:     client.MessageStatusRead,
		Attachment: &client.Attachment{Kind: "image", MimeType: "image/jpeg"},
	}
}

func TestAlbumSpan_GroupsSameSenderWithinGap(t *testing.T) {
	msgs := []client.Message{
		{ID: "t0", TS: 1000, Text: "hi"},
		albumPhoto("p1", "a", 1010),
		albumPhoto("p2", "a", 1015),
		albumPhoto("p3", "a", 1060),
		albumPhoto("p4", "b", 1061), // another sender
		albumPhoto("p5", "a", 1200), // too late to join p3
		{ID: "t1", TS: 1300, Text: "bye"},
	}
	for pos, want := range map[int][2]int{0: {0, 0}, 1: {1, 3}, 2: {1, 3}, 3: {1, 3}, 4: {4, 4}, 5: {5, 5}, 6: {6, 6}} {
		s, e := albumSpan(msgs, pos, time.UTC, nil)
		if s != want[0] || e != want[1] {
			t.Errorf("pos %d: span = %d..%d, want %d..%d", pos, s, e, want[0], want[1])
		}
	}
}

func TestAlbumSpan_BreaksAtAnchor(t *testing.T) {
	msgs := []client.Message{
		albumPhoto("p1", "a", 1000),
		albumPhoto("p2", "a", 1001),
		albumPhoto("p3", "a", 1002),
	}
	breakAt := func(m client.Message) bool { return m.ID == "p2" }
	if s, e := albumSpan(msgs, 0, time.UTC, breakAt); s != 0 || e != 0 {
		t.Errorf("head span = %d..%d, want 0..0", s, e)
	}
	if s, e := albumSpan(msgs, 2, time.UTC, breakAt); s != 1 || e != 2 {
		t.Errorf("tail span = %d..%d, want 1..2", s, e)
	}
}

func TestAlbumMember_KeepsSpecialBubblesApart(t *testing.T) {
	base := albumPhoto("p", "a", 1000)
	cases := map[string]func(m *client.Message){
		"caption":   func(m *client.Message) { m.Attachment.Caption = "look" },
		"reply":     func(m *client.Message) { m.ReplyTo = &client.MsgRef{MsgID: "x"} },
		"gif":       func(m *client.Message) { m.Attachment.IsGIF = true },
		"view once": func(m *client.Message) { m.Attachment.ViewOnce = true },
		"deleted":   func(m *client.Message) { m.Deleted = true },
		"sticker":   func(m *client.Message) { m.Attachment.Kind = "sticker" },
		"document":  func(m *client.Message) { m.Attachment.Kind = "document" },
		"failed":    func(m *client.Message) { m.FromMe = true; m.Status = client.MessageStatusFailed },
		"text":      func(m *client.Message) { m.Attachment = nil },
	}
	if !albumMember(base) {
		t.Fatal("a plain photo is a member")
	}
	for name, mutate := range cases {
		m := base
		a := *base.Attachment
		m.Attachment = &a
		mutate(&m)
		if albumMember(m) {
			t.Errorf("%s: albumMember = true, want false", name)
		}
	}
	clip := base
	clip.Attachment = &client.Attachment{Kind: "video"}
	if !albumMember(clip) {
		t.Error("a captionless clip is a member")
	}
}

func TestAlbumJoins_SameDayOnly(t *testing.T) {
	loc := time.UTC
	late := time.Date(2026, 9, 10, 23, 59, 40, 0, loc).Unix()
	early := time.Date(2026, 9, 11, 0, 0, 10, 0, loc).Unix()
	if albumJoins(albumPhoto("a", "x", late), albumPhoto("b", "x", early), loc) {
		t.Error("a run must not straddle the day separator")
	}
	if !albumJoins(albumPhoto("a", "x", late), albumPhoto("b", "x", late+10), loc) {
		t.Error("same sender ten seconds apart joins")
	}
	fwd := albumPhoto("b", "x", late+10)
	fwd.Forwarded = true
	if albumJoins(albumPhoto("a", "x", late), fwd, loc) {
		t.Error("a forwarded picture does not join an original one")
	}
}

func TestAlbumVM_TilesAndMore(t *testing.T) {
	var run []client.Message
	for i := 0; i < 6; i++ {
		run = append(run, albumPhoto("p"+string(rune('0'+i)), "a", int64(1000+i)))
	}
	v := albumVM(run)
	if len(v.Tiles) != albumMaxTiles || v.More != 2 || len(v.Msgs) != 6 {
		t.Fatalf("tiles=%d more=%d msgs=%d", len(v.Tiles), v.More, len(v.Msgs))
	}
	if v.Tiles[3].MsgID != "p3" {
		t.Errorf("fourth tile is %s, want p3", v.Tiles[3].MsgID)
	}
	if v2 := albumVM(run[:3]); len(v2.Tiles) != 3 || v2.More != 0 {
		t.Errorf("three pictures: tiles=%d more=%d", len(v2.Tiles), v2.More)
	}
}

func TestApplyAlbum_MergesReactionsAndLowestTick(t *testing.T) {
	run := []client.Message{
		albumPhoto("p1", "me", 1000),
		albumPhoto("p2", "me", 1001),
		albumPhoto("p3", "me", 1002),
	}
	run[0].Status = client.MessageStatusRead
	run[1].Status = client.MessageStatusSent
	run[2].Status = client.MessageStatusDelivered
	run[1].Reactions = map[string][]string{"👍": {"a@s.whatsapp.net"}}
	run[2].Reactions = map[string][]string{"👍": {"b@s.whatsapp.net"}, "❤️": {"a@s.whatsapp.net"}}

	v := bubbleVM(run[0], nil, nil, time.Unix(2000, 0).UTC())
	applyAlbum(&v, run)
	if v.Album == nil || len(v.Album.Tiles) != 3 {
		t.Fatal("album not applied")
	}
	if v.TickText != "✓" || v.TickRead {
		t.Errorf("tick = %q read=%v, want the single tick of the least advanced send", v.TickText, v.TickRead)
	}
	if len(v.Reactions) != 2 {
		t.Fatalf("reactions = %+v, want the two emojis across the run", v.Reactions)
	}
	for _, r := range v.Reactions {
		if r.Emoji == "👍" && r.Count != 2 {
			t.Errorf("👍 count = %d, want 2", r.Count)
		}
	}
}

func TestAlbumRows_FootprintsMatchTheBubble(t *testing.T) {
	for n := 2; n <= 4; n++ {
		rows := albumRows(n)
		tiles := 0
		for _, row := range rows {
			width := -albumTileGap
			for _, tile := range row {
				width += tile.W + albumTileGap
				tiles++
			}
			if width != albumWidth {
				t.Errorf("n=%d: row width %d, want %d", n, width, albumWidth)
			}
		}
		if tiles != n {
			t.Errorf("n=%d: %d tiles laid out", n, tiles)
		}
	}
}

func TestAlbumRefillSpan_CoversTheRunsOnEitherSide(t *testing.T) {
	msgs := []client.Message{
		{ID: "t0", TS: 1000, Text: "hi"},
		albumPhoto("p1", "a", 1010),
		albumPhoto("p2", "a", 1012),
		{ID: "t1", TS: 1013, Text: "mid"},
		albumPhoto("p3", "a", 1020),
		albumPhoto("p4", "a", 1022),
		{ID: "t2", TS: 1100, Text: "bye"},
	}
	cases := map[int][2]int{
		0: {0, 2}, // a text next to a run: the whole run is refilled too
		1: {1, 2},
		2: {1, 2},
		3: {1, 5}, // between two runs: both
		5: {4, 5},
		6: {4, 6},
	}
	for pos, want := range cases {
		lo, hi := albumRefillSpan(msgs, pos, time.UTC, nil)
		if lo != want[0] || hi != want[1] {
			t.Errorf("pos %d: refill %d..%d, want %d..%d", pos, lo, hi, want[0], want[1])
		}
		if pos < lo || pos > hi {
			t.Errorf("pos %d: not inside its own refill range %d..%d", pos, lo, hi)
		}
	}

	// A revoked picture leaves its run: refilling it must reach the head
	// of the run it was in, so the grid loses the tile.
	msgs[2].Deleted = true
	if lo, hi := albumRefillSpan(msgs, 2, time.UTC, nil); lo != 1 || hi != 2 {
		t.Errorf("revoked member: refill %d..%d, want 1..2", lo, hi)
	}
}
