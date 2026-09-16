package ui

import (
	"time"

	"chatot/internal/client"
)

// voiceHooks is what a voice-note row tells its conversation about
// playback, with the note's view (its message and chat): it started (the
// note counts as played), it stopped at a position (remembered for the
// next play), or it ran out (the next voice note in the thread may
// follow). The chat is named because a note keeps playing after the
// reader leaves its chat (see miniPlayer), so the conversation may be
// showing another one by then. Any hook may be nil.
type voiceHooks struct {
	onPlay  func(mv mediaView)
	onStop  func(mv mediaView, ms int)
	onEnded func(mv mediaView)
	// onTranscribe asks for the note's transcript: requested by a click
	// (the text unfolds once it lands) or automatically (it stays folded).
	// onCancelTranscribe takes that back while the run is under way.
	// onToggleTranscript remembers whether the text is unfolded, so a
	// rebuilt row keeps it that way.
	onTranscribe       func(msgID, path string, requested bool)
	onCancelTranscribe func(msgID string)
	onToggleTranscript func(msgID string, open bool)
	// onTranscriptMore remembers whether a long transcript's "Read more"
	// is open.
	onTranscriptMore func(msgID string, open bool)
}

// playVoice starts mv's shared player, resuming where it last stopped when
// it has not moved yet this session, and reports the start to hooks. The
// row's play disc and auto-advance both go through here so a note started
// either way is marked played and remembers its position.
func playVoice(mv mediaView, hooks voiceHooks) *mediaPlayer {
	p := sharedVoicePlayer(mv.LocalPath, mv.MimeType, mv.DurationSecs)
	bindVoiceHooks(p, mv, hooks)
	// The note becomes the one the sidebar keeps at hand, and whatever
	// else was playing stops: one audio at a time, as on WhatsApp.
	setNowPlaying(p, mv)
	if p.Playing() {
		return p
	}
	if resume := resumeFraction(mv.PlayPosMS, p.Duration()); resume > 0 && p.Elapsed() < 0.5 && !p.ended {
		p.SeekTo(resume)
	}
	p.Toggle()
	if hooks.onPlay != nil {
		hooks.onPlay(mv)
	}
	return p
}

// bindVoiceHooks points p's stop/end callbacks at mv's message. The player
// outlives its row (it is shared across rebuilds), so this is redone by
// whoever touches it; the closures only ever name the same message.
func bindVoiceHooks(p *mediaPlayer, mv mediaView, hooks voiceHooks) {
	p.onStop = func(secs float64) {
		if hooks.onStop != nil {
			hooks.onStop(mv, int(secs*1000+0.5))
		}
	}
	p.onEnded = func() {
		if hooks.onStop != nil {
			hooks.onStop(mv, 0)
		}
		if hooks.onEnded != nil {
			hooks.onEnded(mv)
		}
		// Nothing followed (or the next note is still being fetched):
		// the sidebar's mini-player has nothing left to hold.
		if !p.Playing() {
			clearNowPlaying(p)
		}
	}
}

// resumeFraction is where a note stopped last time, as a fraction of its
// length, or 0 when it should start over: nothing saved, no known length,
// or it stopped within a second of the end.
func resumeFraction(posMS int, durationSecs float64) float64 {
	if posMS <= 0 || durationSecs <= 0 {
		return 0
	}
	pos := float64(posMS) / 1000
	if pos >= durationSecs-1 {
		return 0
	}
	return pos / durationSecs
}

// voiceChainStep is what a run does with the note that follows the one
// that just ran out.
type voiceChainStep int

const (
	// voiceChainStop ends the run.
	voiceChainStop voiceChainStep = iota
	// voiceChainPlay starts the note straight away.
	voiceChainPlay
	// voiceChainFetch downloads the note and plays it when it lands. A
	// note that is not in the cache still belongs to the run: automatic
	// downloads only cover recent messages, and the reader who started
	// the run asked for the notes after it too, so stopping at the first
	// one nobody had opened yet read as the run being broken.
	voiceChainFetch
)

// voiceChainStepFor decides what happens to the next note. A view-once
// note is the one thing a run must not open: it is spent by playing it.
func voiceChainStepFor(mv mediaView) voiceChainStep {
	switch {
	case mv.ViewOnce:
		return voiceChainStop
	case mv.HasLocal:
		return voiceChainPlay
	}
	return voiceChainFetch
}

// voiceChainFetchTimeout bounds a download the run starts by itself. A note
// that takes longer than this to arrive is no longer the one the reader is
// waiting for, so the run ends rather than speaking up minutes later.
const voiceChainFetchTimeout = 60 * time.Second

// nextVoiceMessage is the message to play after msgID ran out: the one
// right after it in msgs when that is an audio message too (ours or
// theirs), the way WhatsApp keeps playing through a run of voice notes.
// Anything else in between (text, a picture, a revoked note) ends the run;
// so does the end of the thread.
func nextVoiceMessage(msgs []client.Message, msgID string) (client.Message, bool) {
	for i := range msgs {
		if msgs[i].ID != msgID {
			continue
		}
		if i+1 >= len(msgs) {
			return client.Message{}, false
		}
		next := msgs[i+1]
		if next.Deleted || next.Attachment == nil || next.Attachment.Kind != "audio" {
			return client.Message{}, false
		}
		return next, true
	}
	return client.Message{}, false
}
