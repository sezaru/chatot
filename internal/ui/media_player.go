package ui

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/media"
)

// mediaPlayer drives one gtk.MediaFile and tells its widgets when to
// repaint: the voice-note row, the viewer's transport bar and the video
// stage all share it, so play/seek/mute behave the same everywhere. It is
// created idle (no autoplay) and only ever used on the GTK main loop.
type mediaPlayer struct {
	stream  *gtk.MediaFile
	path    string
	seconds int // length the message claims, for before the stream is prepared
	ended   bool
	// failed is set when the file could not be made playable (a transcode
	// that did not go through); widgets bound later show the fallback.
	failed error
	// watchers repaint on every timestamp/playing/ended change, keyed so a
	// widget can drop its own when it goes away.
	watchers map[int]func()
	nextKey  int
	// wantPlay records a Toggle made before the stream was prepared;
	// wantSeek (when >= 0) a SeekTo made before it was seekable.
	wantPlay bool
	wantSeek float64
	// pending is set while path is known but its stream not yet built.
	pending bool
	// muted and loop are the settings a stream gets when built.
	muted, loop bool
}

// newMediaPlayer prepares path for playback without starting it; the
// stream exists at once (a stage needs it to paint).
func newMediaPlayer(path string, seconds int) *mediaPlayer {
	p := &mediaPlayer{path: path, seconds: seconds, wantSeek: -1}
	p.attach(gtk.NewMediaFileForFilename(path))
	return p
}

// newPendingPlayer is a player whose file is not ready yet (a transcode in
// flight); SetFile arms it.
func newPendingPlayer(seconds int) *mediaPlayer {
	return &mediaPlayer{seconds: seconds, wantSeek: -1}
}

// SetFile (re)points the player at path. The stream itself is built on the
// first play or seek: a GStreamer pipeline costs tens of milliseconds
// (hundreds for the first), and a chat page can hold dozens of voice notes
// that are never played.
func (p *mediaPlayer) SetFile(path string) {
	p.path = path
	if p.stream != nil {
		// A stream already bound is swapped at once; the old one is
		// silenced and its notifications are ignored from here on.
		p.stream.Pause()
		p.attach(gtk.NewMediaFileForFilename(path))
	} else {
		p.pending = true
	}
	p.notify()
}

// ensureStream builds the stream for a pending file.
func (p *mediaPlayer) ensureStream() {
	if p.pending {
		p.pending = false
		p.attach(gtk.NewMediaFileForFilename(p.path))
	}
}

func (p *mediaPlayer) attach(stream *gtk.MediaFile) {
	p.stream = stream
	p.pending = false
	stream.SetMuted(p.muted)
	stream.SetLoop(p.loop)
	// A stream replaced by SetFile keeps its handlers; they do nothing.
	current := func() bool { return p.stream == stream }
	for _, prop := range []string{"timestamp", "playing", "ended", "prepared", "duration"} {
		stream.NotifyProperty(prop, func() {
			if current() {
				p.notify()
			}
		})
	}
	stream.NotifyProperty("ended", func() {
		if current() && stream.Ended() {
			p.ended = true
		}
	})
	// GTK refuses Play and Seek on a stream that is not prepared yet; a
	// Toggle or SeekTo that came early is applied once it is.
	stream.NotifyProperty("prepared", func() {
		if !current() || !stream.IsPrepared() {
			return
		}
		if p.wantSeek >= 0 && stream.IsSeekable() {
			stream.Seek(int64(p.wantSeek * p.Duration() * 1e6))
			p.wantSeek = -1
		}
		if p.wantPlay {
			p.wantPlay = false
			stream.Play()
		}
	})
	stream.NotifyProperty("error", func() {
		if err := stream.Error(); err != nil && current() {
			log.Printf("chatot: media %s: %v", p.path, err)
		}
	})
}

// Ready reports whether there is a file to play.
func (p *mediaPlayer) Ready() bool { return p.stream != nil || p.pending }

// Watch registers f to run after every playback state change and returns
// the call that unregisters it (wired to the widget's destroy, so a recycled
// list row doesn't leave its closures behind).
func (p *mediaPlayer) Watch(f func()) (unwatch func()) {
	if p.watchers == nil {
		p.watchers = map[int]func(){}
	}
	p.nextKey++
	key := p.nextKey
	p.watchers[key] = f
	return func() { delete(p.watchers, key) }
}

func (p *mediaPlayer) notify() {
	for _, f := range p.watchers {
		f()
	}
}

// watchUntilDestroyed is Watch for a widget's lifetime.
func (p *mediaPlayer) watchUntilDestroyed(w gtk.Widgetter, f func()) {
	unwatch := p.Watch(f)
	gtk.BaseWidget(w).ConnectDestroy(unwatch)
}

// voicePlayers keeps one player per downloaded audio file: GtkListView
// rebuilds a bubble whenever its row is recycled, and a fresh player per
// rebuild would leave the old one playing with nothing to stop it. Keyed by
// the cache path, which is unique per attachment. Main loop only.
var voicePlayers = map[string]*mediaPlayer{}

// sharedVoicePlayer returns the player for path, creating (and, for MP3,
// transcoding) it on first use.
func sharedVoicePlayer(path, mime string, seconds int) *mediaPlayer {
	if p, ok := voicePlayers[path]; ok {
		return p
	}
	// Bounded: once the cap is reached every idle player is dropped (a
	// rebuilt bubble simply makes a fresh one), so a long session doesn't
	// hoard streams.
	if len(voicePlayers) >= voicePlayersCap {
		for k, old := range voicePlayers {
			if !old.Playing() {
				delete(voicePlayers, k)
			}
		}
	}
	p := newPendingPlayer(seconds)
	voicePlayers[path] = p
	preparePlayable(p, path, mime, func(err error) {
		p.failed = err
		p.notify()
	})
	return p
}

// voicePlayersCap is how many idle in-chat players are kept around.
const voicePlayersCap = 24

// pauseVoicePlayers stops every in-chat player (a chat switch).
func pauseVoicePlayers() {
	for _, p := range voicePlayers {
		p.Pause()
	}
}

// Playing reports whether the stream is running.
func (p *mediaPlayer) Playing() bool {
	return p.stream != nil && (p.stream.Playing() || p.wantPlay)
}

// Toggle plays or pauses; a stream at its end starts over.
func (p *mediaPlayer) Toggle() {
	p.ensureStream()
	if p.stream == nil {
		return
	}
	if p.stream.Playing() {
		p.stream.Pause()
		return
	}
	if !p.stream.IsPrepared() {
		p.wantPlay = !p.wantPlay
		p.notify()
		return
	}
	if p.ended || (p.stream.Ended()) {
		p.ended = false
		p.stream.Seek(0)
	}
	p.stream.Play()
}

// Pause stops playback (a bubble scrolled away, a pane closed).
func (p *mediaPlayer) Pause() {
	p.wantPlay = false
	if p.stream != nil && p.stream.Playing() {
		p.stream.Pause()
	}
}

// Duration is the length in seconds: the stream's once prepared, else what
// the message said.
func (p *mediaPlayer) Duration() float64 {
	if p.stream != nil && p.stream.IsPrepared() && p.stream.Duration() > 0 {
		return float64(p.stream.Duration()) / 1e6
	}
	return float64(p.seconds)
}

// Elapsed is the playhead in seconds.
func (p *mediaPlayer) Elapsed() float64 {
	if p.stream == nil {
		return 0
	}
	return float64(p.stream.Timestamp()) / 1e6
}

// Progress is Elapsed over Duration, clamped to 0..1.
func (p *mediaPlayer) Progress() float64 {
	d := p.Duration()
	if d <= 0 {
		return 0
	}
	f := p.Elapsed() / d
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// SeekTo moves the playhead to fraction (0..1) of the length.
func (p *mediaPlayer) SeekTo(fraction float64) {
	p.ensureStream()
	if p.stream == nil {
		return
	}
	fraction = clampF(fraction, 0, 1)
	if !p.stream.IsSeekable() {
		p.wantSeek = fraction
		p.notify()
		return
	}
	p.ended = false
	p.stream.Seek(int64(fraction * p.Duration() * 1e6))
	p.notify()
}

// SetMuted toggles audio output.
func (p *mediaPlayer) SetMuted(muted bool) {
	p.muted = muted
	if p.stream != nil {
		p.stream.SetMuted(muted)
	}
}

// Muted reports the audio state.
func (p *mediaPlayer) Muted() bool { return p.muted }

// SetLoop makes playback wrap (GIF-style clips).
func (p *mediaPlayer) SetLoop(loop bool) {
	p.loop = loop
	if p.stream != nil {
		p.stream.SetLoop(loop)
	}
}

// playableCacheDir is where MP3s GTK cannot play are transcoded to.
func playableCacheDir() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(dir, "chatot", "playable")
}

// preparePlayable hands the player a GTK-safe copy of the audio at path:
// the file itself when it plays as is, else a transcode made off the main
// loop (the player stays pending, so its widgets show the disabled state
// until then). onFail runs on the main loop if the transcode fails.
func preparePlayable(p *mediaPlayer, path, mime string, onFail func(error)) {
	if !media.NeedsTranscode(path, mime) {
		p.SetFile(path)
		return
	}
	go func() {
		out, err := media.PlayableAudio(context.Background(), playableCacheDir(), path, mime)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: transcode %s: %v", path, err)
				if onFail != nil {
					onFail(err)
				}
				return
			}
			p.SetFile(out)
		})
	}()
}

// ---- voice note row (the mockup's isVoice ready state) -------------------

// voiceTrackH is the track widget's height: the 4px bar plus room for the
// 10px knob above and below its centre.
const voiceTrackH = 12

// newVoiceRow builds the mockup's voice-note row: a 28px round play disc, a
// 4px track with a 10px knob and the mono length, 260px wide. onGreen picks
// the outgoing-bubble colours. onOpen, when set, is the mono length's
// click-to-open (the viewer); the track itself seeks.
func newVoiceRow(p *mediaPlayer, onGreen bool, onOpen func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-voice")

	glyph := gtk.NewDrawingArea()
	glyph.SetSizeRequest(11, 11)
	glyph.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawPausePlay(cr, float64(w), float64(h), !p.Playing())
	})
	play := newRoundButton(glyph, 28)
	play.RemoveCSSClass("chatot-round-btn")
	play.AddCSSClass("chatot-voice-play")
	play.SetFocusOnClick(false)
	play.SetTooltipText("Play")
	play.ConnectClicked(p.Toggle)
	play.SetSensitive(p.Ready())
	row.Append(play)

	track := gtk.NewDrawingArea()
	track.SetHExpand(true)
	track.SetVAlign(gtk.AlignCenter)
	track.SetSizeRequest(120, voiceTrackH)
	track.SetCursorFromName("pointer")
	track.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawVoiceTrack(cr, float64(w), float64(h), p.Progress(), onGreen, isDark())
	})
	seek := gtk.NewGestureClick()
	seek.ConnectReleased(func(_ int, x, _ float64) {
		w := float64(track.AllocatedWidth())
		if w > 0 {
			p.SeekTo(x / w)
		}
	})
	track.AddController(seek)
	row.Append(track)

	timeLabel := gtk.NewLabel(humanDuration(int(p.Duration() + 0.5)))
	timeLabel.AddCSSClass("chatot-voice-time")
	timeLabel.SetVAlign(gtk.AlignCenter)
	if onOpen != nil {
		timeLabel.SetTooltipText("Open in the viewer")
		row.Append(openOnClick(timeLabel, "", func(string) { onOpen() }))
	} else {
		row.Append(timeLabel)
	}

	p.watchUntilDestroyed(row, func() {
		play.SetSensitive(p.Ready())
		glyph.QueueDraw()
		track.QueueDraw()
		// The length until playback has moved; then the playhead.
		secs := p.Duration()
		if p.Elapsed() > 0.5 {
			secs = p.Elapsed()
		}
		timeLabel.SetLabel(humanClock(secs))
		if p.Playing() {
			play.SetTooltipText("Pause")
		} else {
			play.SetTooltipText("Play")
		}
	})
	return row
}

// drawVoiceTrack paints the 4px track, its played part and the 10px knob.
func drawVoiceTrack(cr *cairo.Context, w, h, progress float64, onGreen, dark bool) {
	cy := h / 2
	// Track background: white at 30% on green, grey at 28% elsewhere.
	if onGreen {
		cr.SetSourceRGBA(1, 1, 1, 0.3)
	} else {
		cr.SetSourceRGBA(0.5, 0.5, 0.5, 0.28)
	}
	roundedRectPath(cr, 0, cy-2, w, 4, 2)
	cr.Fill()
	// Played part and knob: white on green, the accent elsewhere.
	if onGreen {
		cr.SetSourceRGB(1, 1, 1)
	} else {
		cr.SetSourceRGB(0x1b/255.0, 0x8c/255.0, 0x72/255.0)
	}
	x := progress * w
	if x > 0 {
		roundedRectPath(cr, 0, cy-2, x, 4, 2)
		cr.Fill()
	}
	cr.Arc(clampF(x, 5, w-5), cy, 5, 0, 6.2832)
	cr.Fill()
}

func clampF(v, lo, hi float64) float64 {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---- video stage --------------------------------------------------------

// videoStage is a picture that paints the player's frames, with the poster
// frame shown until the first decoded frame lands. Unlike GtkVideo it has
// no controls of its own, so the transport bar below it is the only one.
type videoStage struct {
	*gtk.Overlay
	pic    *gtk.Picture
	poster *gtk.Picture
	bound  *gtk.MediaFile // the stream the picture currently paints
	// unwatch drops the stage's player watcher; also wired to destroy.
	unwatch func()
	// shared marks a stage painting another view's player: that view's
	// surface holds the stream's realization (realizing it on a second
	// surface while it prepares stalls the pipeline), and playback carries
	// on when this stage goes away.
	shared bool
}

// newVideoStage paints p's frames; poster (JPEG bytes, may be nil) covers
// the stage until playback starts.
func newVideoStage(p *mediaPlayer, poster []byte) *videoStage {
	overlay := gtk.NewOverlay()
	pic := gtk.NewPicture()
	pic.SetCanShrink(true)
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetHExpand(true)
	pic.SetVExpand(true)
	overlay.SetChild(pic)
	s := &videoStage{Overlay: overlay, pic: pic}

	if len(poster) > 0 {
		if texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(poster)); err == nil {
			s.poster = gtk.NewPictureForPaintable(texture)
			s.poster.SetCanShrink(true)
			s.poster.SetContentFit(gtk.ContentFitContain)
			s.poster.SetCanTarget(false)
			overlay.AddOverlay(s.poster)
		}
	}
	// A click on the picture toggles playback; the stage has no other
	// control of its own.
	click := gtk.NewGestureClick()
	click.SetButton(gdk.BUTTON_PRIMARY)
	click.ConnectReleased(func(int, float64, float64) { p.Toggle() })
	overlay.AddController(click)

	s.bind(p)
	return s
}

// bind points the stage at p's stream: the media file is the picture's
// paintable, and it needs the surface realized to decode onto it.
func (s *videoStage) bind(p *mediaPlayer) {
	hook := func() {
		if p.stream == nil {
			return
		}
		s.bound = p.stream
		s.pic.SetPaintable(p.stream)
		if s.pic.Realized() {
			p.stream.Realize(s.pic.Native().Surface())
		}
	}
	s.pic.ConnectRealize(func() {
		if p.stream != nil && !s.shared {
			p.stream.Realize(s.pic.Native().Surface())
		}
	})
	s.pic.ConnectUnrealize(func() {
		if p.stream != nil && !s.shared {
			p.Pause()
			p.stream.Unrealize(s.pic.Native().Surface())
		}
	})
	hook()
	s.unwatch = p.Watch(func() {
		if p.stream != nil && s.bound != p.stream {
			hook()
		}
		if s.poster != nil && (p.Playing() || p.Elapsed() > 0) {
			s.poster.SetVisible(false)
		}
	})
	s.pic.ConnectDestroy(s.unwatch)
}

// ---- transport bar (the mockup's viewHasTransport row) ------------------

// newTransportBar is the viewer's play/elapsed/track/length/mute row (plus
// a fullscreen button when onFullscreen is set): a 32px accent play disc,
// mono times, a 5px track with a 13px knob.
func newTransportBar(p *mediaPlayer, onFullscreen func()) gtk.Widgetter {
	bar, unwatch := newTransportBarWatched(p, onFullscreen)
	bar.ConnectDestroy(unwatch)
	return bar
}

// newTransportBarWatched is newTransportBar with the watcher's release
// handed back, for a bar whose widget may outlive the window it sat in.
func newTransportBarWatched(p *mediaPlayer, onFullscreen func()) (*gtk.Box, func()) {
	bar := gtk.NewBox(gtk.OrientationHorizontal, 11)
	bar.AddCSSClass("chatot-transport")

	glyph := gtk.NewDrawingArea()
	glyph.SetSizeRequest(12, 12)
	glyph.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawPausePlay(cr, float64(w), float64(h), !p.Playing())
	})
	play := newRoundButton(glyph, 32)
	play.RemoveCSSClass("chatot-round-btn")
	play.AddCSSClass("chatot-transport-play")
	play.SetFocusOnClick(false)
	play.SetTooltipText("Play · Space")
	play.ConnectClicked(p.Toggle)
	play.SetSensitive(p.Ready())
	bar.Append(play)

	elapsed := gtk.NewLabel("0:00")
	elapsed.AddCSSClass("chatot-transport-time")
	bar.Append(elapsed)

	track := gtk.NewDrawingArea()
	track.SetHExpand(true)
	track.SetVAlign(gtk.AlignCenter)
	track.SetSizeRequest(120, 14)
	track.SetCursorFromName("pointer")
	track.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawTransportTrack(cr, float64(w), float64(h), p.Progress(), isDark())
	})
	seek := gtk.NewGestureClick()
	seek.ConnectReleased(func(_ int, x, _ float64) {
		if w := float64(track.AllocatedWidth()); w > 0 {
			p.SeekTo(x / w)
		}
	})
	track.AddController(seek)
	drag := gtk.NewGestureDrag()
	drag.ConnectDragUpdate(func(dx, _ float64) {
		x0, _, _ := drag.StartPoint()
		if w := float64(track.AllocatedWidth()); w > 0 {
			p.SeekTo((x0 + dx) / w)
		}
	})
	track.AddController(drag)
	bar.Append(track)

	total := gtk.NewLabel(humanDuration(int(p.Duration() + 0.5)))
	total.AddCSSClass("chatot-transport-time")
	bar.Append(total)

	mute := gtk.NewButtonWithLabel("🔊")
	mute.AddCSSClass("flat")
	mute.RemoveCSSClass("text-button")
	mute.AddCSSClass("chatot-transport-btn")
	mute.SetTooltipText("Mute")
	mute.SetFocusOnClick(false)
	mute.ConnectClicked(func() {
		p.SetMuted(!p.Muted())
		if p.Muted() {
			mute.SetLabel("🔇")
			mute.SetTooltipText("Unmute")
		} else {
			mute.SetLabel("🔊")
			mute.SetTooltipText("Mute")
		}
	})
	bar.Append(mute)

	if onFullscreen != nil {
		full := gtk.NewButtonWithLabel("⤢")
		full.AddCSSClass("flat")
		full.RemoveCSSClass("text-button")
		full.AddCSSClass("chatot-transport-btn")
		full.SetTooltipText("Fullscreen · F")
		full.SetFocusOnClick(false)
		full.ConnectClicked(onFullscreen)
		bar.Append(full)
	}

	unwatch := p.Watch(func() {
		play.SetSensitive(p.Ready())
		glyph.QueueDraw()
		track.QueueDraw()
		elapsed.SetLabel(humanClock(p.Elapsed()))
		total.SetLabel(humanClock(p.Duration()))
		if p.Playing() {
			play.SetTooltipText("Pause · Space")
		} else {
			play.SetTooltipText("Play · Space")
		}
	})
	return bar, unwatch
}

// humanClock is humanDuration that renders 0 as "0:00" rather than "".
func humanClock(secs float64) string {
	s := int(secs + 0.5)
	if s <= 0 {
		return "0:00"
	}
	return humanDuration(s)
}

// drawTransportTrack paints the viewer's 5px track (chip grey), the accent
// played part and a 13px accent knob ringed in the bar colour.
func drawTransportTrack(cr *cairo.Context, w, h, progress float64, dark bool) {
	cy := h / 2
	if dark {
		cr.SetSourceRGBA(1, 1, 1, 0.12)
	} else {
		cr.SetSourceRGBA(0, 0, 0, 0.08)
	}
	roundedRectPath(cr, 0, cy-2.5, w, 5, 2.5)
	cr.Fill()
	cr.SetSourceRGB(0x1b/255.0, 0x8c/255.0, 0x72/255.0)
	x := progress * w
	if x > 0 {
		roundedRectPath(cr, 0, cy-2.5, x, 5, 2.5)
		cr.Fill()
	}
	kx := clampF(x, 6.5, w-6.5)
	cr.Arc(kx, cy, 6.5, 0, 6.2832)
	cr.Fill()
	if dark {
		cr.SetSourceRGB(0x24/255.0, 0x24/255.0, 0x24/255.0)
	} else {
		cr.SetSourceRGB(1, 1, 1)
	}
	cr.Arc(kx, cy, 4.5, 0, 6.2832)
	cr.Fill()
	cr.SetSourceRGB(0x1b/255.0, 0x8c/255.0, 0x72/255.0)
	cr.Arc(kx, cy, 3, 0, 6.2832)
	cr.Fill()
}
