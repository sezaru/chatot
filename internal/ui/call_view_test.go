package ui

import (
	"testing"
	"time"

	"chatot/internal/client"
)

func TestCallVMMissedVoice(t *testing.T) {
	v := callVM(client.Message{CallLog: &client.CallLog{Outcome: client.CallMissed}})
	if !v.IsCall || v.Title != "Missed voice call" || v.Glyph != "📞" || !v.Missed || v.Detail != "" {
		t.Fatalf("got %+v", v)
	}
}

func TestCallVMAnsweredVideoWithDuration(t *testing.T) {
	v := callVM(client.Message{CallLog: &client.CallLog{Video: true, Outcome: client.CallAnswered, DurationSecs: 151}})
	if v.Title != "Video call" || v.Glyph != "🎥" || v.Missed || v.Detail != "2:31" {
		t.Fatalf("got %+v", v)
	}
}

func TestCallVMNoCall(t *testing.T) {
	if v := callVM(client.Message{Text: "hi"}); v.IsCall {
		t.Fatalf("got %+v, want zero value", v)
	}
}

func TestCallDurationText(t *testing.T) {
	cases := map[int]string{5: "0:05", 151: "2:31", 3600: "1:00:00", 3725: "1:02:05"}
	for secs, want := range cases {
		if got := callDurationText(secs); got != want {
			t.Errorf("%d -> %q, want %q", secs, got, want)
		}
	}
}

func TestBubbleVMCall(t *testing.T) {
	m := client.Message{ID: "call:1", ChatJID: "a@s.whatsapp.net", TS: 1700000000, CallLog: &client.CallLog{Outcome: client.CallMissed}}
	v := bubbleVM(m, nil, map[string]client.Message{}, time.Now())
	if !v.IsCall || v.IsMedia || v.Text != "" {
		t.Fatalf("got %+v, want a call bubble", v)
	}
	if got := copyableText(m); got != "Missed voice call" {
		t.Errorf("copyableText = %q", got)
	}
	a := bubbleSig(m)
	m.CallLog.Outcome = client.CallAnswered
	if bubbleSig(m) == a {
		t.Error("bubbleSig must change when the call is picked up")
	}
}
