package ui

import (
	"errors"
	"testing"
	"time"

	"chatot/internal/transcribe"
)

func TestAutoTranscribeWants(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	fresh := now.Unix() - 3600
	old := now.Add(-8 * 24 * time.Hour).Unix()
	cases := []struct {
		name       string
		on         bool
		transcript string
		ts         int64
		want       bool
	}{
		{"on, fresh inbound note", true, "", fresh, true},
		// A thread where only the other side has text reads as broken, so
		// an own note is transcribed on the same terms as any other.
		{"own note", true, "", fresh, true},
		{"preference off", false, "", fresh, false},
		{"already transcribed", true, "hello", fresh, false},
		{"older than a week", true, "", old, false},
		{"unknown time", true, "", 0, false},
	}
	for _, c := range cases {
		if got := autoTranscribeWants(c.on, c.transcript, c.ts, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTranscriptRowLabels(t *testing.T) {
	now := time.Now()
	if got := transcriptRowLabel(transcriptState{}, now); got != "" {
		t.Errorf("idle label = %q, want no slot at all", got)
	}
	if got := transcriptRowLabel(transcriptState{Busy: true}, now); got != "Transcribing on this computer…" {
		t.Errorf("busy label = %q", got)
	}
	if got := transcriptRowLabel(transcriptState{Err: "No speech found in this voice message."}, now); got != "No speech found in this voice message." {
		t.Errorf("failed label = %q", got)
	}
	if transcriptChevron(true) == transcriptChevron(false) {
		t.Error("chevron does not change with the fold")
	}
	if transcriptFoldTooltip(true) == transcriptFoldTooltip(false) {
		t.Error("fold tooltip does not change with the fold")
	}
}

// Folded, the block shows the transcript rather than the word TRANSCRIPT,
// so a note dictated in paragraphs still has to fold to one line.
func TestTranscriptPreviewIsOneLine(t *testing.T) {
	const text = "Hey, I just got out of the studio.\n\nThe shoot ran long.\tDinner at eight?"
	const want = "Hey, I just got out of the studio. The shoot ran long. Dinner at eight?"
	if got := transcriptPreview(text); got != want {
		t.Errorf("preview = %q, want %q", got, want)
	}
	if got := transcriptPreview(""); got != "" {
		t.Errorf("empty preview = %q", got)
	}
}

// The one button at the end of the row carries what clicking it does: stop
// the run under way, fold the transcript that is there, or start one.
func TestTranscribeButtonGlyphFollowsTheState(t *testing.T) {
	if got := transcribeButtonGlyph(transcriptState{}, false, false); got != "T" {
		t.Errorf("idle glyph = %q, want T", got)
	}
	if got := transcribeButtonGlyph(transcriptState{Busy: true}, false, false); got != "✕" {
		t.Errorf("running glyph = %q, want the stop mark", got)
	}
	if got := transcribeButtonGlyph(transcriptState{Queued: true}, false, false); got != "✕" {
		t.Errorf("waiting glyph = %q, want the stop mark", got)
	}
	if got := transcribeButtonGlyph(transcriptState{}, true, true); got != transcriptChevron(true) {
		t.Errorf("unfolded glyph = %q, want the up chevron", got)
	}
	if got := transcribeButtonGlyph(transcriptState{}, true, false); got != transcriptChevron(false) {
		t.Errorf("folded glyph = %q, want the down chevron", got)
	}
	// A run started over a transcript that is already there is still a run:
	// the stop mark wins over the chevron.
	if got := transcribeButtonGlyph(transcriptState{Busy: true}, true, false); got != "✕" {
		t.Errorf("rerun glyph = %q, want the stop mark", got)
	}
}

// A run that found nothing, and one that could not run at all, are answers
// rather than failures: offering "Try again" would only repeat them.
func TestTranscriptErrorTextRetryOnlyWhereItHelps(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		want      string
		wantRetry bool
	}{
		{"no speech", transcribe.ErrNoSpeech, "No speech found in this voice message.", false},
		{"no engine", transcribe.ErrNoEngine, "whisper.cpp is not installed.", false},
		{"anything else", errors.New("boom"), "Could not transcribe this one.", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, retry := transcriptErrorText(c.err)
			if got != c.want {
				t.Errorf("text = %q, want %q", got, c.want)
			}
			if retry != c.wantRetry {
				t.Errorf("retry = %v, want %v", retry, c.wantRetry)
			}
		})
	}
}

func TestTranscribeButtonTooltipCarriesTheState(t *testing.T) {
	if got := transcribeButtonTooltip(transcriptState{}, false, false); got != "Transcribe voice message" {
		t.Errorf("idle tooltip = %q", got)
	}
	// While a run is going the button takes it back, so the tooltip says so
	// rather than reporting progress the line under the row already gives.
	if got := transcribeButtonTooltip(transcriptState{Busy: true}, false, false); got != "Stop transcribing" {
		t.Errorf("busy tooltip = %q", got)
	}
	if got := transcribeButtonTooltip(transcriptState{Queued: true}, false, false); got != "Stop waiting to transcribe" {
		t.Errorf("queued tooltip = %q", got)
	}
	if got := transcribeButtonTooltip(transcriptState{}, true, false); got != transcriptFoldTooltip(false) {
		t.Errorf("folded tooltip = %q", got)
	}
	if got := transcribeButtonTooltip(transcriptState{}, true, true); got != transcriptFoldTooltip(true) {
		t.Errorf("unfolded tooltip = %q", got)
	}
}

func TestTranscriptOpenFollowsPreferenceUntilClicked(t *testing.T) {
	defer func(v bool) { TranscriptsExpanded = v }(TranscriptsExpanded)
	TranscriptsExpanded = false
	if transcriptOpen(transcriptState{}) {
		t.Error("folded by default with the preference off")
	}
	if !transcriptOpen(transcriptState{Open: true}) {
		t.Error("a click to unfold is ignored")
	}
	TranscriptsExpanded = true
	if !transcriptOpen(transcriptState{}) {
		t.Error("not unfolded with the preference on")
	}
	if transcriptOpen(transcriptState{Folded: true}) {
		t.Error("a click to fold is ignored with the preference on")
	}
}

// A run says how long it has been going once it stops being a moment. The
// same note takes seconds on an idle machine and many minutes on one that
// is swapping, and a spinner alone makes the second case look like a hang.
func TestBusyTranscriptLabelCounts(t *testing.T) {
	now := time.Now()
	const base = "Transcribing on this computer…"
	if got := busyTranscriptLabel(time.Time{}, now); got != base {
		t.Errorf("a run with no start time = %q, want the plain line", got)
	}
	if got := busyTranscriptLabel(now.Add(-3*time.Second), now); got != base {
		t.Errorf("a run of 3s = %q, want no clock on an ordinary run", got)
	}
	if got := busyTranscriptLabel(now.Add(-90*time.Second), now); got != base+" 1:30" {
		t.Errorf("a run of 90s = %q, want the plain line plus 1:30", got)
	}
	if got := busyTranscriptLabel(now.Add(-25*time.Minute), now); got != base+" 25:00" {
		t.Errorf("a long run = %q, want it to read as 25 minutes", got)
	}
}
