package ui

import "chatot/internal/client"

// voiceHooks is what a voice-note row tells its conversation about
// playback, keyed by message id: it started (the note counts as played),
// it stopped at a position (remembered for the next play), or it ran out
// (the next voice note in the thread may follow). Any hook may be nil.
type voiceHooks struct {
	onPlay  func(msgID string)
	onStop  func(msgID string, ms int)
	onEnded func(msgID string)
}

// playVoice starts mv's shared player, resuming where it last stopped when
// it has not moved yet this session, and reports the start to hooks. The
// row's play disc and auto-advance both go through here so a note started
// either way is marked played and remembers its position.
func playVoice(mv mediaView, hooks voiceHooks) *mediaPlayer {
	p := sharedVoicePlayer(mv.LocalPath, mv.MimeType, mv.DurationSecs)
	bindVoiceHooks(p, mv, hooks)
	if p.Playing() {
		return p
	}
	if resume := resumeFraction(mv.PlayPosMS, p.Duration()); resume > 0 && p.Elapsed() < 0.5 && !p.ended {
		p.SeekTo(resume)
	}
	p.Toggle()
	if hooks.onPlay != nil {
		hooks.onPlay(mv.MsgID)
	}
	return p
}

// bindVoiceHooks points p's stop/end callbacks at mv's message. The player
// outlives its row (it is shared across rebuilds), so this is redone by
// whoever touches it; the closures only ever name the same message.
func bindVoiceHooks(p *mediaPlayer, mv mediaView, hooks voiceHooks) {
	id := mv.MsgID
	p.onStop = func(secs float64) {
		if hooks.onStop != nil {
			hooks.onStop(id, int(secs*1000+0.5))
		}
	}
	p.onEnded = func() {
		if hooks.onStop != nil {
			hooks.onStop(id, 0)
		}
		if hooks.onEnded != nil {
			hooks.onEnded(id)
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
