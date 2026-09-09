package ui

import (
	"errors"
	"log"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/transcribe"
)

// AutoTranscribe mirrors settings.AutoTranscribe: whether an incoming voice
// note is transcribed as soon as it is downloaded, without a click.
var AutoTranscribe = false

// TranscriptsExpanded mirrors settings.TranscriptsExpanded: whether a
// transcript starts unfolded under its note.
var TranscriptsExpanded = false

// The transcribe control on the voice row and the spinners that stand in
// for its glyph while a run is under way, at the mockup's sizes.
const (
	transcribeBtnSize     = 23
	transcribeSpinnerSize = 11
	transcriptSpinnerSize = 12
)

// transcriptState is what a voice bubble knows about its note's transcript
// beyond the text itself: a run in progress, why the last one produced
// nothing, and whether the text is unfolded.
type transcriptState struct {
	// Queued is a run waiting its turn behind others; Busy one the engine
	// is on. Requested says a click asked for it (the text unfolds when
	// it lands), as against the automatic preference.
	Queued    bool
	Busy      bool
	Requested bool
	// Err is the row's one-line reason for having no transcript, "" when
	// there is none (the detail goes to the log).
	Err string
	// Retry says the reason is worth another click ("could not transcribe"),
	// as against an answer that would not change (no speech, no engine).
	Retry bool
	// Open and Folded are the reader's own fold of the text this session
	// (see transcriptOpen); More whether a long transcript's "Read more"
	// is open.
	Open   bool
	Folded bool
	More   bool
}

// transcriptOpen is whether st's text shows: what the reader last did to
// it, else the preference (unfolded when TranscriptsExpanded, folded
// otherwise).
func transcriptOpen(st transcriptState) bool {
	switch {
	case st.Folded:
		return false
	case st.Open:
		return true
	}
	return TranscriptsExpanded
}

// autoTranscribeWants reports whether a downloaded voice note without a
// transcript, sent at ts, should be transcribed without a click: only under
// the preference, only someone else's, and only a recent one, since
// scrolling back through months of notes must not start hundreds of runs
// (the same week auto-download stops at).
func autoTranscribeWants(on, fromMe bool, transcript string, ts int64, now time.Time) bool {
	if !on || fromMe || transcript != "" {
		return false
	}
	return ts > 0 && now.Unix()-ts <= int64(autoDownloadMaxAge/time.Second)
}

// maybeAutoTranscribe starts mv's transcript in the background when the
// preference wants one and nothing stands in the way: no run under way, no
// earlier failure this session, and the engine and model already in place
// (an automatic run never opens the download prompt; the first click
// does). Deferred to an idle so the row is in the tree before it rebuilds.
func maybeAutoTranscribe(mv mediaView) {
	st := mv.TranscriptState
	if mv.voice.onTranscribe == nil || st.Busy || st.Queued || st.Err != "" ||
		!autoTranscribeWants(AutoTranscribe, mv.FromMe, mv.Transcript, mv.TS, time.Now()) ||
		!transcribe.EngineAvailable() || !transcribe.ModelReady(cacheDir()) {
		return
	}
	id, path, start := mv.MsgID, mv.LocalPath, mv.voice.onTranscribe
	glib.IdleAdd(func() { start(id, path, false) })
}

// transcriptRowLabel is the one-line note the transcript slot shows while
// a note has no transcript of its own, "" when the slot stays away.
func transcriptRowLabel(st transcriptState) string {
	switch {
	case st.Busy:
		return "Transcribing on this computer…"
	case st.Queued:
		return "Waiting to transcribe…"
	case st.Err != "":
		return st.Err
	}
	return ""
}

// transcriptChevron is the fold marker at the end of the transcript head:
// pointing up when the text is showing, down when it is folded away.
func transcriptChevron(open bool) string {
	if open {
		return "▲"
	}
	return "▼"
}

// transcriptHeadTooltip says what clicking the head will do.
func transcriptHeadTooltip(open bool) string {
	if open {
		return "Hide transcript"
	}
	return "Show transcript"
}

// transcriptErrorText is the row's reason for having no transcript, and
// whether clicking again is worth offering. A note with no speech in it and
// a missing engine are both answers, not failures: repeating the run would
// give the same one.
func transcriptErrorText(err error) (text string, retry bool) {
	switch {
	case errors.Is(err, transcribe.ErrNoSpeech):
		return "No speech found in this voice message.", false
	case errors.Is(err, transcribe.ErrNoEngine):
		return "whisper.cpp is not installed.", false
	}
	return "Could not transcribe this one.", true
}

// transcribeButtonTooltip is the T button's tooltip, which carries the state
// the glyph alone cannot.
func transcribeButtonTooltip(st transcriptState, hasText bool) string {
	switch {
	case st.Busy:
		return "Transcribing…"
	case st.Queued:
		return "Waiting to transcribe…"
	case hasText:
		return "Transcript ready"
	}
	return "Transcribe voice message"
}

// newTranscribeButton is the T at the end of a voice row: the one control
// that starts a transcription, spinning while a run is under way and
// filled once there is a transcript to show. Nil where the row has no
// conversation behind it (the viewer, tests).
func newTranscribeButton(mv mediaView, fold func()) *gtk.Button {
	if mv.voice.onTranscribe == nil {
		return nil
	}
	st := mv.TranscriptState
	btn := gtk.NewButton()
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-transcribe-btn")
	btn.SetVAlign(gtk.AlignCenter)
	btn.SetFocusOnClick(false)
	btn.SetSizeRequest(transcribeBtnSize, transcribeBtnSize)
	btn.SetTooltipText(transcribeButtonTooltip(st, mv.Transcript != ""))
	if st.Busy || st.Queued {
		spinner := adw.NewSpinner()
		spinner.SetSizeRequest(transcribeSpinnerSize, transcribeSpinnerSize)
		spinner.SetHAlign(gtk.AlignCenter)
		spinner.SetVAlign(gtk.AlignCenter)
		btn.SetChild(spinner)
	} else {
		btn.SetChild(gtk.NewLabel("T"))
	}
	if mv.Transcript != "" {
		btn.AddCSSClass("chatot-transcribe-btn-on")
	}
	id, path, start := mv.MsgID, mv.LocalPath, mv.voice.onTranscribe
	if st.Busy || st.Queued {
		btn.SetSensitive(false)
		return btn
	}
	// With a transcript already in hand the T folds it instead of running
	// the engine over the same audio again.
	if mv.Transcript != "" && fold != nil {
		btn.ConnectClicked(fold)
		return btn
	}
	btn.ConnectClicked(func() { start(id, path, true) })
	return btn
}

// transcriptSlot is what a voice bubble gets back for its transcript: the
// block that goes under the row, and the fold the row's T shares with the
// block's own head. Both are nil where there is nothing to show.
type transcriptSlot struct {
	widget gtk.Widgetter
	fold   func()
}

// buildTranscriptSlot is the block under a voice row, as WhatsApp folds one
// under a note: a TRANSCRIPT head with a chevron over the text, the run in
// progress, or the reason there is no text. Nothing at all before anyone
// asks — the row's T is the invitation — and nothing where the row has no
// conversation behind it (the viewer, tests).
func buildTranscriptSlot(mv mediaView) transcriptSlot {
	if mv.voice.onTranscribe == nil {
		return transcriptSlot{}
	}
	st := mv.TranscriptState
	id := mv.MsgID
	if mv.Transcript == "" {
		label := transcriptRowLabel(st)
		if label == "" {
			return transcriptSlot{}
		}
		box := gtk.NewBox(gtk.OrientationVertical, 0)
		box.AddCSSClass("chatot-transcript")
		if st.Busy || st.Queued {
			// The run says where it is happening: on this computer, not on
			// a server somewhere.
			line := gtk.NewBox(gtk.OrientationHorizontal, 8)
			spinner := adw.NewSpinner()
			spinner.SetSizeRequest(transcriptSpinnerSize, transcriptSpinnerSize)
			spinner.SetVAlign(gtk.AlignCenter)
			line.Append(spinner)
			l := gtk.NewLabel(label)
			l.AddCSSClass("chatot-transcript-dim")
			l.SetXAlign(0)
			line.Append(l)
			box.Append(line)
			return transcriptSlot{widget: box}
		}
		line := gtk.NewBox(gtk.OrientationHorizontal, 8)
		l := gtk.NewLabel(label)
		l.AddCSSClass("chatot-transcript-dim")
		l.SetXAlign(0)
		l.SetWrap(true)
		l.SetWrapMode(pango.WrapWordChar)
		l.SetMaxWidthChars(34)
		l.SetHExpand(true)
		line.Append(l)
		if st.Retry {
			again := gtk.NewButtonWithLabel("Try again")
			again.AddCSSClass("flat")
			again.AddCSSClass("chatot-transcript-head")
			again.SetVAlign(gtk.AlignStart)
			again.SetFocusOnClick(false)
			path, start := mv.LocalPath, mv.voice.onTranscribe
			again.ConnectClicked(func() { start(id, path, true) })
			line.Append(again)
		}
		box.Append(line)
		return transcriptSlot{widget: box}
	}

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-transcript")
	open := transcriptOpen(st)
	head := gtk.NewButton()
	head.AddCSSClass("flat")
	head.AddCSSClass("chatot-transcript-head-row")
	head.SetFocusOnClick(false)
	head.SetTooltipText(transcriptHeadTooltip(open))
	headRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	caption := gtk.NewLabel("TRANSCRIPT")
	caption.AddCSSClass("chatot-transcript-caption")
	caption.SetXAlign(0)
	caption.SetHExpand(true)
	headRow.Append(caption)
	chevron := gtk.NewLabel(transcriptChevron(open))
	chevron.AddCSSClass("chatot-transcript-chevron")
	headRow.Append(chevron)
	head.SetChild(headRow)

	// A long transcript folds behind a "Read more" like a long text
	// body, at the same cut.
	long := isLongText(mv.Transcript)
	more := st.More
	body := func() string {
		if long && !more {
			return clipText(mv.Transcript)
		}
		return mv.Transcript
	}
	text := gtk.NewLabel(body())
	text.AddCSSClass("chatot-transcript-text")
	text.SetXAlign(0)
	text.SetWrap(true)
	text.SetWrapMode(pango.WrapWordChar)
	text.SetMaxWidthChars(40)
	text.SetSelectable(true)
	text.SetVisible(open)
	var moreBtn *gtk.Button
	if long {
		moreBtn = gtk.NewButtonWithLabel(readMoreLabel(more))
		moreBtn.AddCSSClass("flat")
		moreBtn.AddCSSClass("chatot-read-more")
		moreBtn.SetHAlign(gtk.AlignStart)
		moreBtn.SetFocusOnClick(false)
		moreBtn.SetVisible(open)
		rememberMore := mv.voice.onTranscriptMore
		moreBtn.ConnectClicked(func() {
			more = !more
			text.SetLabel(body())
			moreBtn.SetLabel(readMoreLabel(more))
			if rememberMore != nil {
				rememberMore(id, more)
			}
		})
	}
	// Where the words came from, so nobody has to wonder whether the audio
	// left the machine.
	foot := gtk.NewLabel("Transcribed on this device")
	foot.AddCSSClass("chatot-transcript-foot")
	foot.SetXAlign(0)
	foot.SetVisible(open)

	remember := mv.voice.onToggleTranscript
	fold := func() {
		open = !open
		text.SetVisible(open)
		if moreBtn != nil {
			moreBtn.SetVisible(open)
		}
		foot.SetVisible(open)
		chevron.SetLabel(transcriptChevron(open))
		head.SetTooltipText(transcriptHeadTooltip(open))
		if remember != nil {
			remember(id, open)
		}
	}
	head.ConnectClicked(fold)
	box.Append(head)
	box.Append(text)
	if moreBtn != nil {
		box.Append(moreBtn)
	}
	box.Append(foot)
	return transcriptSlot{widget: box, fold: fold}
}

// transcriptOf is msgID's transcription state this session.
func (cv *ConversationView) transcriptOf(msgID string) transcriptState {
	return cv.transcripts[msgID]
}

func (cv *ConversationView) setTranscriptState(msgID string, st transcriptState) {
	if cv.transcripts == nil {
		cv.transcripts = map[string]transcriptState{}
	}
	cv.transcripts[msgID] = st
}

// setTranscriptOpen remembers whether msgID's transcript is unfolded, so
// the row shows it that way when it is rebuilt.
func (cv *ConversationView) setTranscriptOpen(msgID string, open bool) {
	st := cv.transcriptOf(msgID)
	st.Open, st.Folded = open, !open
	cv.setTranscriptState(msgID, st)
}

// setTranscriptMore remembers whether msgID's long transcript is read in
// full.
func (cv *ConversationView) setTranscriptMore(msgID string, open bool) {
	st := cv.transcriptOf(msgID)
	st.More = open
	cv.setTranscriptState(msgID, st)
}

// transcribe puts the downloaded voice note msgID, at path, on the
// transcription queue (see transcribe.Queue: one engine at a time, clicks
// ahead of automatic runs, the newest automatic first). The row is
// rebuilt as the note waits, as the engine takes it, and as the text (or
// the failure) lands. A missing
// model is offered for download first when the run was requested by a
// click, and the run follows the download. requested also unfolds the
// text once it is there; an automatic run leaves it folded. The path
// comes from the bubble rather than the view's copy of the message,
// which lags behind a download finished in the row. Must run on the GTK
// main loop.
func (cv *ConversationView) transcribe(msgID, path string, requested bool) {
	pos := cv.positionOf(msgID)
	if pos < 0 || path == "" {
		log.Printf("chatot: transcribe %s: not in the open thread (path %q)", msgID, path)
		return
	}
	if st := cv.transcriptOf(msgID); st.Busy || st.Queued {
		if requested && !st.Requested {
			// A click on a note already waiting its turn moves it ahead
			// of the automatic ones, and unfolds the text when it lands.
			st.Requested = true
			cv.setTranscriptState(msgID, st)
			transcribe.Default.Enqueue(transcribe.Job{Key: msgID, Path: path, CacheDir: cacheDir(), Requested: true})
		}
		return
	}
	if !transcribe.EngineAvailable() {
		log.Printf("chatot: transcribe %s: %s or ffmpeg not on PATH", msgID, transcribe.Engine)
		showToast(cv.toastOverlay, "Install whisper.cpp (whisper-cli) to transcribe voice notes")
		return
	}
	if !transcribe.ModelReady(cacheDir()) {
		if requested {
			showModelDownloadDialog(cv.window, func() { cv.transcribe(msgID, path, true) })
		}
		return
	}
	jid, c := cv.msgs[pos].ChatJID, cv.c
	var started time.Time
	taken := transcribe.Default.Enqueue(transcribe.Job{
		Key: msgID, Path: path, CacheDir: cacheDir(), Requested: requested,
		OnStart: func() {
			started = time.Now()
			glib.IdleAdd(func() {
				st := cv.transcriptOf(msgID)
				st.Queued, st.Busy = false, true
				cv.setTranscriptState(msgID, st)
				cv.refillByID(msgID)
			})
		},
		OnDone: func(text string, err error) {
			if err == nil {
				if perr := c.SetTranscript(jid, msgID, text); perr != nil {
					log.Printf("chatot: save transcript failed: %v", perr)
				}
			}
			glib.IdleAdd(func() {
				prev := cv.transcriptOf(msgID)
				st := transcriptState{Open: prev.Open}
				if err != nil {
					log.Printf("chatot: transcribe %s: %v", msgID, err)
					st.Err, st.Retry = transcriptErrorText(err)
				} else {
					trace(1, "transcribed %s in %s: %d chars", msgID, time.Since(started).Round(time.Millisecond), len(text))
					st.Open = st.Open || prev.Requested
					cv.setTranscript(msgID, text)
				}
				cv.setTranscriptState(msgID, st)
				cv.refillByID(msgID)
			})
		},
	})
	if !taken {
		// The automatic backlog is full; the bubble asks again the next
		// time it is built.
		return
	}
	cv.setTranscriptState(msgID, transcriptState{Queued: true, Requested: requested})
	cv.refillByID(msgID)
	trace(1, "transcribe queued: %s (requested %v, %d waiting)", msgID, requested, transcribe.Default.Pending())
}

// setTranscript puts text on every loaded copy of msgID's attachment,
// without touching the list.
func (cv *ConversationView) setTranscript(msgID, text string) {
	for i := range cv.msgs {
		m := &cv.msgs[i]
		if m.ID != msgID || m.Attachment == nil {
			continue
		}
		a := *m.Attachment
		a.Transcript = text
		m.Attachment = &a
		cv.byID[msgID] = *m
	}
}

// refillByID rebuilds msgID's row, if it is loaded.
func (cv *ConversationView) refillByID(msgID string) {
	if pos := cv.positionOf(msgID); pos >= 0 {
		cv.refillRow(pos)
	}
}
