package ui

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// nowPlayingNote is the voice note or audio last started in a chat. It
// keeps playing when the reader moves to another chat, the way WhatsApp
// Web keeps an audio going, and the sidebar's mini-player holds it then.
// Main loop only.
type nowPlayingNote struct {
	p  *mediaPlayer
	mv mediaView
	// held marks a pause made from the mini-player itself, which keeps
	// the strip (a resume is one click away); a note paused in its own
	// row is let go of when the reader leaves its chat.
	held bool
}

// nowPlaying is the note the mini-player would show; nil when none.
var nowPlaying *nowPlayingNote

// nowPlayingWatchers run whenever nowPlaying is set, replaced or cleared.
var nowPlayingWatchers []func()

// setNowPlaying makes p (playing mv) the note the sidebar keeps at hand
// and silences every other in-chat player: one audio at a time.
func setNowPlaying(p *mediaPlayer, mv mediaView) {
	for _, other := range voicePlayers {
		if other != p {
			other.Pause()
		}
	}
	if nowPlaying != nil && nowPlaying.p == p {
		nowPlaying.mv = mv
		nowPlaying.held = false
		return
	}
	nowPlaying = &nowPlayingNote{p: p, mv: mv}
	notifyNowPlaying()
}

// clearNowPlaying forgets p when it is the note at hand (it ended, or the
// mini-player's × stopped it).
func clearNowPlaying(p *mediaPlayer) {
	if nowPlaying == nil || nowPlaying.p != p {
		return
	}
	nowPlaying = nil
	notifyNowPlaying()
}

func notifyNowPlaying() {
	for _, f := range nowPlayingWatchers {
		f()
	}
}

// nowPlayingFollows reports whether np is still worth holding once chat
// jid is the one showing: it is in that chat (its own row shows it), still
// playing, or paused from the mini-player. A note paused in its own row
// and then left behind is not.
func nowPlayingFollows(np *nowPlayingNote, jid string) bool {
	if np == nil {
		return false
	}
	return np.mv.ChatJID == jid || np.held || np.p.Playing()
}

// miniPlayerShows reports whether the strip shows for np while chat
// current is open: only away from the note's own chat, where its row is
// the player.
func miniPlayerShows(np *nowPlayingNote, current string) bool {
	return np != nil && np.mv.ChatJID != current
}

// miniPlayer is the strip under the chat list that keeps a voice note or
// audio at hand once the reader has left its chat: play/pause, the track
// to seek, × to stop, and the name takes the reader back to the note in
// its chat.
type miniPlayer struct {
	*gtk.Box
	c client.Client
	// onOpen opens the note's chat at the note (main wires openChatAt).
	onOpen  func(chatJID, msgID string)
	current string // the chat open in the content pane
	bound   *nowPlayingNote
	unwatch func()
	glyph   *gtk.DrawingArea
	play    *gtk.Button
	title   *gtk.Label
	track   *gtk.DrawingArea
	timeLbl *gtk.Label
}

func newMiniPlayer(c client.Client) *miniPlayer {
	root := gtk.NewBox(gtk.OrientationHorizontal, 10)
	root.AddCSSClass("chatot-miniplayer")
	root.SetVisible(false)
	m := &miniPlayer{Box: root, c: c}

	m.glyph = gtk.NewDrawingArea()
	m.glyph.SetSizeRequest(11, 11)
	m.glyph.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		paused := m.bound == nil || !m.bound.p.Playing()
		drawPausePlay(cr, float64(w), float64(h), paused)
	})
	m.play = newRoundButton(m.glyph, 28)
	m.play.RemoveCSSClass("chatot-round-btn")
	m.play.AddCSSClass("chatot-voice-play")
	m.play.SetFocusOnClick(false)
	m.play.SetTooltipText("Pause")
	m.play.ConnectClicked(m.toggle)
	root.Append(m.play)

	col := gtk.NewBox(gtk.OrientationVertical, 3)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	m.title = gtk.NewLabel("")
	m.title.SetXAlign(0)
	m.title.SetEllipsize(pango.EllipsizeEnd)
	m.title.SetMaxWidthChars(1)
	m.title.SetHExpand(true)
	m.title.AddCSSClass("chatot-miniplayer-title")
	m.title.SetTooltipText("Go to the message")
	m.title.SetCursorFromName("pointer")
	open := gtk.NewGestureClick()
	open.ConnectReleased(func(int, float64, float64) { m.open() })
	m.title.AddController(open)
	col.Append(m.title)

	line := gtk.NewBox(gtk.OrientationHorizontal, 8)
	m.track = gtk.NewDrawingArea()
	m.track.SetHExpand(true)
	m.track.SetVAlign(gtk.AlignCenter)
	m.track.SetSizeRequest(60, voiceTrackH)
	m.track.SetCursorFromName("pointer")
	m.track.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		var progress float64
		if m.bound != nil {
			progress = m.bound.p.Progress()
		}
		drawVoiceTrack(cr, float64(w), float64(h), progress, false, false, isDark(), bubbleColorsOf(m.track))
	})
	seek := gtk.NewGestureClick()
	seek.ConnectReleased(func(_ int, x, _ float64) {
		if w := float64(m.track.AllocatedWidth()); w > 0 && m.bound != nil {
			m.bound.p.SeekTo(x / w)
		}
	})
	m.track.AddController(seek)
	line.Append(m.track)
	m.timeLbl = gtk.NewLabel("")
	m.timeLbl.AddCSSClass("chatot-voice-time")
	m.timeLbl.SetVAlign(gtk.AlignCenter)
	line.Append(m.timeLbl)
	col.Append(line)
	root.Append(col)

	closeBtn := gtk.NewButtonWithLabel("✕")
	closeBtn.RemoveCSSClass("text-button")
	closeBtn.AddCSSClass("flat")
	closeBtn.AddCSSClass("chatot-miniplayer-close")
	closeBtn.SetVAlign(gtk.AlignCenter)
	closeBtn.SetFocusOnClick(false)
	closeBtn.SetTooltipText("Stop")
	closeBtn.ConnectClicked(m.close)
	root.Append(closeBtn)

	nowPlayingWatchers = append(nowPlayingWatchers, m.refresh)
	return m
}

// SetCurrentChat is the chat switch: the strip shows or hides for jid,
// and a note that was paused in its own row is let go of.
func (m *miniPlayer) SetCurrentChat(jid string) {
	if m.current == jid {
		return
	}
	m.current = jid
	if np := nowPlaying; np != nil && !nowPlayingFollows(np, jid) {
		clearNowPlaying(np.p) // refreshes through the watcher
		return
	}
	m.refresh()
}

// refresh binds the strip to nowPlaying and shows or hides it.
func (m *miniPlayer) refresh() {
	np := nowPlaying
	if !miniPlayerShows(np, m.current) {
		m.bind(nil)
		m.SetVisible(false)
		return
	}
	m.bind(np)
	m.title.SetLabel(m.titleFor(np.mv))
	m.SetVisible(true)
	m.paint()
}

func (m *miniPlayer) bind(np *nowPlayingNote) {
	if m.bound == np {
		return
	}
	if m.unwatch != nil {
		m.unwatch()
		m.unwatch = nil
	}
	m.bound = np
	if np != nil {
		m.unwatch = np.p.Watch(m.paint)
	}
}

// paint repaints the disc, the track and the clock for the bound player.
func (m *miniPlayer) paint() {
	m.glyph.QueueDraw()
	m.track.QueueDraw()
	if m.bound == nil {
		return
	}
	p := m.bound.p
	m.play.SetSensitive(p.Ready())
	// The length until playback has moved; then the playhead.
	secs := p.Duration()
	if p.Elapsed() > 0.5 {
		secs = p.Elapsed()
	}
	m.timeLbl.SetLabel(humanClock(secs))
	if p.Playing() {
		m.play.SetTooltipText("Pause")
	} else {
		m.play.SetTooltipText("Play")
	}
}

func (m *miniPlayer) toggle() {
	np := m.bound
	if np == nil {
		return
	}
	if np.p.Playing() {
		np.held = true
		np.p.Pause()
		return
	}
	setNowPlaying(np.p, np.mv) // silences anything started meanwhile
	np.p.Toggle()
}

func (m *miniPlayer) close() {
	np := m.bound
	if np == nil {
		return
	}
	np.p.Pause()
	clearNowPlaying(np.p)
}

func (m *miniPlayer) open() {
	np := m.bound
	if np == nil || m.onOpen == nil {
		return
	}
	m.onOpen(np.mv.ChatJID, np.mv.MsgID)
}

// titleFor names the note: its chat, and in a group who sent it.
func (m *miniPlayer) titleFor(mv mediaView) string {
	chat := chatByJID(m.c, mv.ChatJID).Name
	if chat == "" {
		chat = JIDFallbackName(mv.ChatJID)
	}
	if !strings.HasSuffix(mv.ChatJID, "@g.us") {
		return chat
	}
	who := "You"
	if !mv.FromMe {
		if who = m.c.ContactName(mv.FromJID); who == "" {
			who = JIDFallbackName(mv.FromJID)
		}
	}
	return who + " · " + chat
}
