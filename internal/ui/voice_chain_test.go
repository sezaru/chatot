package ui

import (
	"testing"

	"chatot/internal/client"
)

func audioMsg(id string, fromMe bool) client.Message {
	return client.Message{ID: id, FromMe: fromMe, Attachment: &client.Attachment{Kind: "audio", LocalPath: "/tmp/" + id + ".ogg", DurationSecs: 5}}
}

func TestNextVoiceMessageFollowsARunOfNotes(t *testing.T) {
	msgs := []client.Message{
		{ID: "t0", Text: "hello"},
		audioMsg("v1", false),
		audioMsg("v2", true), // our own reply note still counts
		audioMsg("v3", false),
		{ID: "t1", Text: "bye"},
		audioMsg("v4", false),
	}
	cases := []struct {
		after string
		want  string
		ok    bool
	}{
		{"v1", "v2", true},
		{"v2", "v3", true},
		{"v3", "", false}, // a text message ends the run
		{"v4", "", false}, // end of the thread
		{"t0", "v1", true},
		{"nope", "", false},
	}
	for _, c := range cases {
		got, ok := nextVoiceMessage(msgs, c.after)
		if ok != c.ok || got.ID != c.want {
			t.Errorf("after %s: got (%q, %v), want (%q, %v)", c.after, got.ID, ok, c.want, c.ok)
		}
	}
}

func TestNextVoiceMessageStopsAtRevokedOrNonAudio(t *testing.T) {
	revoked := audioMsg("v2", false)
	revoked.Deleted = true
	msgs := []client.Message{
		audioMsg("v1", false), revoked, audioMsg("v3", false),
		{ID: "p1", Attachment: &client.Attachment{Kind: "image", LocalPath: "/tmp/p.jpg"}},
	}
	if _, ok := nextVoiceMessage(msgs, "v1"); ok {
		t.Error("a revoked note must end the run")
	}
	if _, ok := nextVoiceMessage(msgs, "v3"); ok {
		t.Error("a picture must end the run")
	}
}

func TestResumeFraction(t *testing.T) {
	cases := []struct {
		posMS int
		dur   float64
		want  float64
	}{
		{0, 12, 0},     // never stopped mid-way
		{6000, 12, .5}, // half way
		{6000, 0, 0},   // length unknown: start over
		{11500, 12, 0}, // within a second of the end: start over
		{-4, 12, 0},
	}
	for _, c := range cases {
		if got := resumeFraction(c.posMS, c.dur); got != c.want {
			t.Errorf("resumeFraction(%d, %v) = %v, want %v", c.posMS, c.dur, got, c.want)
		}
	}
}

func TestMediaVMCarriesVoiceState(t *testing.T) {
	m := client.Message{ID: "v1", ChatJID: "a@s.whatsapp.net", Played: true,
		Attachment: &client.Attachment{Kind: "audio", DurationSecs: 12, PlayPosMS: 4000}}
	mv := mediaVM(m)
	if mv.MsgID != "v1" || mv.ChatJID != "a@s.whatsapp.net" || !mv.Played || mv.PlayPosMS != 4000 {
		t.Errorf("mediaVM dropped voice state: %+v", mv)
	}
}

// A run must not stop at a note nobody has opened yet. Automatic downloads
// only cover recent messages and only the kinds the preference names, so the
// note after the one playing is often still on the server; fetching it is
// what the reader asked for by starting the run. A view-once note is the one
// exception, since playing it spends it.
func TestVoiceChainStepFor(t *testing.T) {
	cases := []struct {
		name string
		mv   mediaView
		want voiceChainStep
	}{
		{"a cached note plays", mediaView{HasLocal: true, LocalPath: "/tmp/v1.ogg"}, voiceChainPlay},
		{"a note that was never fetched is downloaded first", mediaView{}, voiceChainFetch},
		{"a view-once note ends the run", mediaView{ViewOnce: true}, voiceChainStop},
		{"a cached view-once note ends it too", mediaView{HasLocal: true, ViewOnce: true}, voiceChainStop},
	}
	for _, c := range cases {
		if got := voiceChainStepFor(c.mv); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// The same call the chain makes, from a message with no local file: the
	// undownloaded note is still part of the run.
	undownloaded := client.Message{ID: "v2",
		Attachment: &client.Attachment{Kind: "audio", MimeType: "audio/ogg", DurationSecs: 11}}
	if got := voiceChainStepFor(mediaVM(undownloaded)); got != voiceChainFetch {
		t.Errorf("an undownloaded note = %v, want it fetched", got)
	}
}
