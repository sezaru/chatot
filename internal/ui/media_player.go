package ui

import (
	"context"
	"log"
	"path/filepath"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/media"
	"chatot/internal/settings"
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
	// onStop, when set, gets the playhead in seconds each time playback
	// stops short of the end (a pause, a chat switch), so the caller can
	// remember where to resume; onEnded runs when the stream plays through.
	// Both run on the main loop. Rebound by whoever starts the player.
	onStop  func(secs float64)
	onEnded func()
	// src and mime name the audio preparePlayable was handed, for a
	// re-render at another speed; rate is the tempo of the file the stream
	// holds (1 for anything else), and speedGen counts SetSpeed calls so
	// a render that finishes late is dropped.
	src, mime string
	rate      float64
	speedGen  int
	// swapping is set while SetSpeed replaces the stream: the old one's
	// pause is not a stop worth reporting.
	swapping bool
}

// voiceSpeed is the tempo every voice note and audio plays at (1, 1.5 or
// 2): the pill on the playing note or the viewer's bar cycles it, and it
// holds for every note played after that. Kept across launches through
// SaveVoiceSpeed, which main.go points at the settings file.
var voiceSpeed = 1.0

// SaveVoiceSpeed, when set, persists a speed the pill picked.
var SaveVoiceSpeed func(speed float64)

// speedPlayers are the audio players a speed change reaches at once (the
// ones playing re-render on the spot; idle ones catch up on their next
// play). Main loop only.
var speedPlayers = map[*mediaPlayer]bool{}

// VoiceSpeed is the current playback tempo.
func VoiceSpeed() float64 { return voiceSpeed }

// SetVoiceSpeed makes speed the tempo from here on: whatever is playing
// carries on at it from where it is, and every audio started later uses
// it. Does not persist it (the startup call restores the saved value).
func SetVoiceSpeed(speed float64) {
	if speed <= 0 {
		speed = 1
	}
	voiceSpeed = speed
	trace(1, "voice speed %g× (%d players)", speed, len(speedPlayers))
	for p := range speedPlayers {
		if p.Playing() {
			p.SetSpeed(speed)
		} else {
			// The label on an idle row reads the new value at once.
			p.notify()
		}
	}
}

// cycleVoiceSpeed is the pill's click: the next speed, applied and saved.
func cycleVoiceSpeed() {
	SetVoiceSpeed(settings.NextVoiceSpeed(voiceSpeed))
	if SaveVoiceSpeed != nil {
		SaveVoiceSpeed(voiceSpeed)
	}
}

// speedLabel is the pill's text for speed: "1×", "1.5×", "2×".
func speedLabel(speed float64) string {
	return strconv.FormatFloat(speed, 'f', -1, 64) + "×"
}

// forgetSpeedPlayer drops p from the players a speed change reaches.
func forgetSpeedPlayer(p *mediaPlayer) { delete(speedPlayers, p) }

// unspeedable gives up on re-rendering p (ffmpeg could not read its file):
// it plays as it is, at the tempo of the file it holds.
func (p *mediaPlayer) unspeedable() {
	p.src = ""
	forgetSpeedPlayer(p)
}

// Speed is the tempo of the file the player holds.
func (p *mediaPlayer) Speed() float64 {
	if p.rate <= 0 {
		return 1
	}
	return p.rate
}

// speedable reports whether p can be re-rendered at another tempo: it has
// a source audio file (video stages and files handed straight to
// newMediaPlayer have none).
func (p *mediaPlayer) speedable() bool { return p.src != "" }

// SetSpeed re-renders the player's audio at speed and carries the playhead
// over: the render happens off the main loop, the old file plays on
// meanwhile, and at the swap the same fraction of the note continues, still
// playing if it was. A press waiting for the file (wantPlay) is kept.
func (p *mediaPlayer) SetSpeed(speed float64) {
	if !p.speedable() || speed <= 0 || p.rate == speed {
		return
	}
	p.speedGen++
	gen := p.speedGen
	src, mime := p.src, p.mime
	go func() {
		out, err := media.SpeedAudio(context.Background(), playableCacheDir(), src, mime, speed)
		glib.IdleAdd(func() {
			if gen != p.speedGen || p.src != src {
				return
			}
			if err != nil {
				log.Printf("chatot: render %s at %g×: %v", src, speed, err)
				// The note plays on as it is, and no longer offers the
				// pill; a press that was waiting for the render goes
				// through the plain path now.
				p.unspeedable()
				if p.wantPlay {
					p.wantPlay = false
					p.Toggle()
				}
				return
			}
			fraction := p.Progress()
			if p.wantSeek >= 0 {
				fraction = p.wantSeek
			}
			if p.ended || (p.stream != nil && p.stream.Ended()) {
				fraction = 0
				p.ended = false
			}
			resume := p.Playing()
			trace(1, "voice speed: %s at %g× from %.2f (playing=%v)", filepath.Base(src), speed, fraction, resume)
			p.rate = speed
			if p.stream == nil {
				// Never built: the new file simply takes the old one's
				// place, and the first play builds the stream from it.
				p.path = out
				p.pending = true
				if p.wantPlay {
					p.ensureStream()
				}
				p.notify()
				return
			}
			p.swapping = true
			p.SetFile(out)
			p.swapping = false
			if fraction > 0 {
				p.SeekTo(fraction)
			}
			if resume {
				p.wantPlay = true
			}
			p.notify()
		})
	}()
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
		if p.wantPlay {
			// Somebody pressed play while the file was still being made:
			// build the stream now, and its prepared handler starts it.
			p.ensureStream()
		}
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
			if p.onEnded != nil {
				p.onEnded()
			}
		}
	})
	// A stop before the end is a pause worth resuming from; GTK freezes
	// notifications while ending a stream, so at the end Ended() is
	// already true here and onEnded above covers it.
	stream.NotifyProperty("playing", func() {
		if current() && !p.swapping && !stream.Playing() && !stream.Ended() && p.onStop != nil {
			p.onStop(p.Elapsed())
		}
	})
	// GTK refuses Play and Seek on a stream that is not prepared yet; a
	// Toggle or SeekTo that came early is applied once it is.
	stream.NotifyProperty("prepared", func() {
		if !current() || !stream.IsPrepared() {
			return
		}
		if p.wantSeek >= 0 && stream.IsSeekable() {
			stream.Seek(int64(p.wantSeek * p.streamSeconds() * 1e6))
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
				forgetSpeedPlayer(old)
			}
		}
	}
	p := newPendingPlayer(seconds)
	voicePlayers[path] = p
	prepareSpeedable(p, path, mime, func(err error) {
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

// anyVoicePlaying reports whether a note is playing in the chat right now,
// which is how a run started by itself knows the reader moved on.
func anyVoicePlaying() bool {
	for _, p := range voicePlayers {
		if p.Playing() {
			return true
		}
	}
	return false
}

// Playing reports whether the stream is running, which includes a press
// that is still waiting for the file: the disc shows pause from the click,
// not from the moment the pipeline catches up.
func (p *mediaPlayer) Playing() bool {
	return p.wantPlay || (p.stream != nil && p.stream.Playing())
}

// Toggle plays or pauses; a stream at its end starts over. A note whose
// file is at an older speed is re-rendered first and starts when that
// lands.
func (p *mediaPlayer) Toggle() {
	if p.speedable() && p.rate != voiceSpeed && !p.Playing() {
		p.wantPlay = true
		p.notify()
		p.SetSpeed(voiceSpeed)
		return
	}
	p.ensureStream()
	if p.stream == nil {
		// There is no file yet: an MP3 is still being transcoded. Remember
		// the press so the note starts the moment it lands, rather than
		// swallowing it — which is what a run of notes hit when it fetched
		// the next one and pressed play in the same breath.
		p.wantPlay = !p.wantPlay
		p.notify()
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

// streamSeconds is the length of the file the stream holds: the stream's
// once prepared, else what the message said scaled to the file's tempo.
func (p *mediaPlayer) streamSeconds() float64 {
	if p.stream != nil && p.stream.IsPrepared() && p.stream.Duration() > 0 {
		return float64(p.stream.Duration()) / 1e6
	}
	return float64(p.seconds) / p.Speed()
}

// Duration is the length in seconds on the note's own timeline (a file
// rendered at 2× plays a 1:00 note in 0:30, but it is still a 1:00 note).
func (p *mediaPlayer) Duration() float64 {
	return p.streamSeconds() * p.Speed()
}

// Elapsed is the playhead in seconds, on the note's own timeline.
func (p *mediaPlayer) Elapsed() float64 {
	if p.stream == nil {
		return 0
	}
	return float64(p.stream.Timestamp()) / 1e6 * p.Speed()
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
	fraction = clampF(fraction, 0, 1)
	if p.stream == nil && p.speedable() && p.rate != voiceSpeed {
		// The file is due for a re-render at the current speed (Toggle
		// does it); the seek waits for the stream that render builds.
		p.wantSeek = fraction
		p.notify()
		return
	}
	p.ensureStream()
	if p.stream == nil {
		return
	}
	if !p.stream.IsSeekable() {
		p.wantSeek = fraction
		p.notify()
		return
	}
	p.ended = false
	p.stream.Seek(int64(fraction * p.streamSeconds() * 1e6))
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
	return filepath.Join(cacheDir(), "playable")
}

// preparePlayable hands the player a GTK-safe copy of the audio at path:
// the file itself when it plays as is, else a transcode made off the main
// loop (the player stays pending, so its widgets show the disabled state
// until then). onFail runs on the main loop if the transcode fails.
func preparePlayable(p *mediaPlayer, path, mime string, onFail func(error)) {
	preparePlayableAt(p, path, mime, 1, onFail)
}

// prepareSpeedable is preparePlayable for a note the speed pill governs:
// the file is rendered at the current speed, and the player follows later
// changes (SetVoiceSpeed) until it is forgotten.
func prepareSpeedable(p *mediaPlayer, path, mime string, onFail func(error)) {
	p.src, p.mime = path, mime
	speedPlayers[p] = true
	preparePlayableAt(p, path, mime, voiceSpeed, onFail)
}

func preparePlayableAt(p *mediaPlayer, path, mime string, speed float64, onFail func(error)) {
	p.rate = speed
	if speed == 1 && !media.NeedsTranscode(path, mime) {
		p.SetFile(path)
		return
	}
	go func() {
		out, err := media.SpeedAudio(context.Background(), playableCacheDir(), path, mime, speed)
		glib.IdleAdd(func() {
			if err != nil && speed != 1 && !media.NeedsTranscode(path, mime) {
				// ffmpeg could not read what GTK may still play: the note
				// plays as it is, at 1×, rather than not at all.
				log.Printf("chatot: render %s at %g×: %v", path, speed, err)
				p.unspeedable()
				if p.speedGen == 0 {
					p.rate = 1
					p.SetFile(path)
				}
				return
			}
			if err != nil {
				log.Printf("chatot: transcode %s: %v", path, err)
				if onFail != nil {
					onFail(err)
				}
				return
			}
			if p.speedGen > 0 {
				// A SetSpeed already overtook this render.
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
// the outgoing-bubble colours; played swaps an incoming row's accent for
// the listened-to blue, the way WhatsApp's microphone turns blue. onToggle
// is the disc's click (nil plays/pauses the player directly). onOpen, when
// set, is the mono length's click-to-open (the viewer); the track itself
// seeks.
func newVoiceRow(p *mediaPlayer, onGreen, played bool, onToggle, onOpen func()) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-voice")
	played = played && !onGreen
	if onToggle == nil {
		onToggle = p.Toggle
	}

	glyph := gtk.NewDrawingArea()
	glyph.SetSizeRequest(11, 11)
	glyph.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawPausePlay(cr, float64(w), float64(h), !p.Playing())
	})
	play := newRoundButton(glyph, 28)
	play.RemoveCSSClass("chatot-round-btn")
	play.AddCSSClass("chatot-voice-play")
	if played {
		play.AddCSSClass("chatot-voice-played")
	}
	play.SetFocusOnClick(false)
	play.SetTooltipText("Play")
	play.ConnectClicked(onToggle)
	play.SetSensitive(p.Ready())
	row.Append(play)

	track := gtk.NewDrawingArea()
	track.SetHExpand(true)
	track.SetVAlign(gtk.AlignCenter)
	track.SetSizeRequest(120, voiceTrackH)
	track.SetCursorFromName("pointer")
	track.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawVoiceTrack(cr, float64(w), float64(h), p.Progress(), onGreen, played, isDark(), bubbleColorsOf(track))
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

	// The speed pill exists only while this note is the one playing (the
	// mockup's isPlaying); it cycles the tempo for every note.
	speed := gtk.NewButton()
	speed.AddCSSClass("flat")
	speed.AddCSSClass("chatot-transcribe-btn")
	speed.AddCSSClass("chatot-transcribe-btn-on")
	speed.AddCSSClass("chatot-speed-btn")
	speed.SetChild(gtk.NewLabel(speedLabel(VoiceSpeed())))
	speed.SetVAlign(gtk.AlignCenter)
	speed.SetSizeRequest(-1, transcribeBtnSize)
	speed.SetTooltipText("Playback speed")
	speed.SetFocusOnClick(false)
	speed.SetVisible(p.speedable() && p.Playing())
	speed.ConnectClicked(cycleVoiceSpeed)
	row.Append(speed)

	p.watchUntilDestroyed(row, func() {
		play.SetSensitive(p.Ready())
		glyph.QueueDraw()
		track.QueueDraw()
		speed.Child().(*gtk.Label).SetLabel(speedLabel(VoiceSpeed()))
		speed.SetVisible(p.speedable() && p.Playing())
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

// voicePlayedRGB is the accent of an incoming voice note that has been
// listened to: WhatsApp's microphone blue, in place of the green.
var voicePlayedRGB = [3]float64{0x53 / 255.0, 0xbd / 255.0, 0xeb / 255.0}

// drawVoiceTrack paints the 4px track, its played part and the 10px knob.
func drawVoiceTrack(cr *cairo.Context, w, h, progress float64, onGreen, played, dark bool, c bubbleColors) {
	cy := h / 2
	// Track background: the bubble's text at 30% on the outgoing bubble,
	// grey at 28% elsewhere.
	if onGreen {
		setSourceRGBA(cr, c.onOut, 0.3)
	} else {
		cr.SetSourceRGBA(0.5, 0.5, 0.5, 0.28)
	}
	roundedRectPath(cr, 0, cy-2, w, 4, 2)
	cr.Fill()
	// Played part and knob: the bubble's text on the outgoing bubble, the
	// accent elsewhere (blue once the note has been listened to).
	switch {
	case onGreen:
		setSourceRGBA(cr, c.onOut, 1)
	case played:
		cr.SetSourceRGB(voicePlayedRGB[0], voicePlayedRGB[1], voicePlayedRGB[2])
	default:
		setSourceRGBA(cr, c.accent, 1)
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

	s.SetPoster(poster)
	// A click on the picture toggles playback; the stage has no other
	// control of its own.
	click := gtk.NewGestureClick()
	click.SetButton(gdk.BUTTON_PRIMARY)
	click.ConnectReleased(func(int, float64, float64) { p.Toggle() })
	overlay.AddController(click)

	s.bind(p)
	return s
}

// SetPoster shows jpeg over the stage until playback starts (nothing for
// empty bytes); a later call swaps the picture, unless the poster has
// already gone because the clip is playing.
func (s *videoStage) SetPoster(jpeg []byte) {
	if len(jpeg) == 0 {
		return
	}
	texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(jpeg))
	if err != nil {
		return
	}
	if s.poster != nil {
		if s.poster.Visible() {
			s.poster.SetPaintable(texture)
		}
		return
	}
	s.poster = gtk.NewPictureForPaintable(texture)
	s.poster.SetCanShrink(true)
	s.poster.SetContentFit(gtk.ContentFitContain)
	s.poster.SetCanTarget(false)
	s.Overlay.AddOverlay(s.poster)
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
		drawTransportTrack(cr, float64(w), float64(h), p.Progress(), isDark(), tokenRGBA(track, "chatot_accent", accentRGBA))
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

	// The tempo, for audio the speed pill governs (a clip's picture cannot
	// be re-rendered the way a note is, so a video bar has no pill).
	var speed *gtk.Button
	if p.speedable() {
		speed = gtk.NewButtonWithLabel(speedLabel(VoiceSpeed()))
		speed.RemoveCSSClass("text-button")
		speed.AddCSSClass("flat")
		speed.AddCSSClass("chatot-transport-speed")
		speed.SetVAlign(gtk.AlignCenter)
		speed.SetTooltipText("Playback speed")
		speed.SetFocusOnClick(false)
		speed.ConnectClicked(cycleVoiceSpeed)
		bar.Append(speed)
	}

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
		if speed != nil {
			speed.SetLabel(speedLabel(VoiceSpeed()))
		}
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
func drawTransportTrack(cr *cairo.Context, w, h, progress float64, dark bool, accent [4]float64) {
	cy := h / 2
	if dark {
		cr.SetSourceRGBA(1, 1, 1, 0.12)
	} else {
		cr.SetSourceRGBA(0, 0, 0, 0.08)
	}
	roundedRectPath(cr, 0, cy-2.5, w, 5, 2.5)
	cr.Fill()
	setSourceRGBA(cr, accent, 1)
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
	setSourceRGBA(cr, accent, 1)
	cr.Arc(kx, cy, 3, 0, 6.2832)
	cr.Fill()
}

// bubbleColors are the tokens a track or bar inside a bubble draws with:
// the accent on an incoming bubble, the outgoing bubble's text colour on
// the outgoing one.
type bubbleColors struct{ accent, onOut [4]float64 }

// bubbleColorsOf reads those tokens on w.
func bubbleColorsOf(w gtk.Widgetter) bubbleColors {
	return bubbleColors{tokenRGBA(w, "chatot_accent", accentRGBA), tokenRGBA(w, "chatot_on_bubble_out", whiteRGBA)}
}
