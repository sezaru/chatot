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
		on, fromMe bool
		transcript string
		ts         int64
		want       bool
	}{
		{"on, fresh inbound note", true, false, "", fresh, true},
		{"preference off", false, false, "", fresh, false},
		{"own note", true, true, "", fresh, false},
		{"already transcribed", true, false, "hello", fresh, false},
		{"older than a week", true, false, "", old, false},
		{"unknown time", true, false, "", 0, false},
	}
	for _, c := range cases {
		if got := autoTranscribeWants(c.on, c.fromMe, c.transcript, c.ts, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTranscriptRowLabels(t *testing.T) {
	if got := transcriptRowLabel(transcriptState{}); got != "" {
		t.Errorf("idle label = %q, want no slot at all", got)
	}
	if got := transcriptRowLabel(transcriptState{Busy: true}); got != "Transcribing on this computer…" {
		t.Errorf("busy label = %q", got)
	}
	if got := transcriptRowLabel(transcriptState{Err: "No speech found in this voice message."}); got != "No speech found in this voice message." {
		t.Errorf("failed label = %q", got)
	}
	if transcriptChevron(true) == transcriptChevron(false) {
		t.Error("chevron does not change with the fold")
	}
	if transcriptHeadTooltip(true) == transcriptHeadTooltip(false) {
		t.Error("head tooltip does not change with the fold")
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
	if got := transcribeButtonTooltip(transcriptState{}, false); got != "Transcribe voice message" {
		t.Errorf("idle tooltip = %q", got)
	}
	if got := transcribeButtonTooltip(transcriptState{Busy: true}, false); got != "Transcribing…" {
		t.Errorf("busy tooltip = %q", got)
	}
	if got := transcribeButtonTooltip(transcriptState{}, true); got != "Transcript ready" {
		t.Errorf("done tooltip = %q", got)
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
