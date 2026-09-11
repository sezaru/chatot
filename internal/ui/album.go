package ui

import (
	"time"

	"chatot/internal/client"
)

// An album is WhatsApp's grouping of the pictures and clips one sender
// posted in one go: the thread shows them as a single bubble with a grid of
// tiles rather than a column of photo bubbles. Nothing in the store knows
// about albums. The grouping is read off the loaded thread whenever a row is
// filled (albumSpan), so the list model stays one item per message and
// paging, receipts, revokes, search and the viewer carry on per message.
// The run's first message carries the bubble; the rows of the others
// collapse to nothing (threadRow.renderCollapsed).

// albumGap is the most seconds that may pass between two consecutive
// members of a run.
const albumGap = 60

// albumMaxTiles is how many tiles the grid shows; the last one carries a
// "+N" for the pictures past it.
const albumMaxTiles = 4

// albumMember reports whether m can sit in an album: a photo or clip with
// nothing of its own to say. A caption, a quote, a GIF, a view-once and a
// tombstone each keep their bubble, as does a send that failed (its Retry
// is per message).
func albumMember(m client.Message) bool {
	a := m.Attachment
	if a == nil || m.Deleted || m.ReplyTo != nil || m.ID == typingSentinelID {
		return false
	}
	if a.Kind != "image" && a.Kind != "video" {
		return false
	}
	if a.ViewOnce || a.IsGIF || captionText(*a) != "" {
		return false
	}
	return !(m.FromMe && m.Status == client.MessageStatusFailed)
}

// albumJoins reports whether m carries on the run prev is in: the same
// sender, within albumGap of it, on the same day, forwarded or not alike.
func albumJoins(prev, m client.Message, loc *time.Location) bool {
	if !albumMember(prev) || !albumMember(m) {
		return false
	}
	if prev.FromMe != m.FromMe || prev.FromJID != m.FromJID || prev.Forwarded != m.Forwarded {
		return false
	}
	if m.TS < prev.TS || m.TS-prev.TS > albumGap {
		return false
	}
	return sameDay(prev.TS, m.TS, loc)
}

// albumSpan is the run of album members around pos in msgs, as an inclusive
// range; start == end for a message that stands on its own. breakAt, when
// set, names a message that starts a run of its own whatever precedes it
// (the one under the unread pill).
func albumSpan(msgs []client.Message, pos int, loc *time.Location, breakAt func(client.Message) bool) (start, end int) {
	start, end = pos, pos
	if pos < 0 || pos >= len(msgs) || !albumMember(msgs[pos]) {
		return start, end
	}
	breaks := func(m client.Message) bool { return breakAt != nil && breakAt(m) }
	for start > 0 && albumJoins(msgs[start-1], msgs[start], loc) && !breaks(msgs[start]) {
		start--
	}
	for end+1 < len(msgs) && albumJoins(msgs[end], msgs[end+1], loc) && !breaks(msgs[end+1]) {
		end++
	}
	return start, end
}

// albumRefillSpan is the range of rows to refill after the message at pos
// changed: its own run, and the runs on either side of it, since a change
// can take a message into or out of an album with its neighbours and the
// album's bubble sits on the run's first row. The row that changed is
// always in the range.
func albumRefillSpan(msgs []client.Message, pos int, loc *time.Location, breakAt func(client.Message) bool) (lo, hi int) {
	lo, hi = albumSpan(msgs, pos, loc, breakAt)
	if pos > 0 && albumMember(msgs[pos-1]) {
		if s, _ := albumSpan(msgs, pos-1, loc, breakAt); s < lo {
			lo = s
		}
	}
	if pos+1 < len(msgs) && albumMember(msgs[pos+1]) {
		if _, e := albumSpan(msgs, pos+1, loc, breakAt); e > hi {
			hi = e
		}
	}
	return lo, hi
}

// albumBreak is the thread's run breaker: the unread pill's message starts
// a run of its own; nil while there is no pill.
func (cv *ConversationView) albumBreak() func(client.Message) bool {
	anchor := cv.unreadAnchor
	if anchor == "" {
		return nil
	}
	return func(m client.Message) bool { return m.ID == anchor }
}

// albumSpan is albumSpan over the loaded thread.
func (cv *ConversationView) albumSpan(pos int) (start, end int) {
	return albumSpan(cv.msgs, pos, time.Local, cv.albumBreak())
}

// albumView is the album bubble's content: the run's messages, head first,
// and a tile for each of the first albumMaxTiles of them.
type albumView struct {
	Msgs  []client.Message
	Tiles []mediaView
	// More is the count the last tile's "+N" stands for: the pictures past
	// the tiles, 0 when every one has a tile.
	More int
}

// albumVM derives the album view for run (at least two members, head first).
func albumVM(run []client.Message) *albumView {
	v := &albumView{Msgs: append([]client.Message(nil), run...)}
	shown := len(run)
	if shown > albumMaxTiles {
		shown = albumMaxTiles
		v.More = len(run) - albumMaxTiles
	}
	v.Tiles = make([]mediaView, 0, shown)
	for _, m := range run[:shown] {
		v.Tiles = append(v.Tiles, mediaVM(m))
	}
	return v
}

// applyAlbum folds run into v, the view of its head: the pills under the
// bubble show the reactions on any of its pictures, and the tick is the
// least advanced of its sends.
func applyAlbum(v *bubbleView, run []client.Message) {
	v.Album = albumVM(run)
	merged := map[string][]string{}
	for _, m := range run {
		for emoji, who := range m.Reactions {
			merged[emoji] = append(merged[emoji], who...)
		}
	}
	v.Reactions = reactionViews(merged)
	if !v.FromMe {
		return
	}
	status := run[0].Status
	for _, m := range run[1:] {
		if m.Status < status {
			status = m.Status
		}
	}
	v.TickText, v.TickRead = tickVM(status)
	v.Pending = status == client.MessageStatusPending
}

// albumHas reports whether id is one of run's messages.
func albumHas(run []client.Message, id string) bool {
	for _, m := range run {
		if m.ID == id {
			return true
		}
	}
	return false
}

// albumTile is one tile's footprint in the grid.
type albumTile struct{ W, H int }

// The grid is as wide as a photo bubble, its tiles a hair apart.
const (
	albumWidth   = inlinePhotoSide
	albumTileGap = 2
)

// albumRows lays n tiles out in rows, the way WhatsApp does: two side by
// side, or one across the top and two under it, or two by two.
func albumRows(n int) [][]albumTile {
	half := (albumWidth - albumTileGap) / 2
	square := albumTile{half, half}
	switch {
	case n <= 1:
		return [][]albumTile{{{albumWidth, inlinePhotoMinH * 2}}}
	case n == 2:
		tall := albumTile{half, half + 30}
		return [][]albumTile{{tall, tall}}
	case n == 3:
		return [][]albumTile{{{albumWidth, half + 20}}, {square, square}}
	default:
		return [][]albumTile{{square, square}, {square, square}}
	}
}
