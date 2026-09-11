package client

import "testing"

func TestFakeSearchMatchesTranscript(t *testing.T) {
	m := Message{ID: "v1", TS: 5, Attachment: &Attachment{Kind: "audio", Transcript: "Call the plumber"}}
	h, ok := fakeSearchHit(m, "plumber")
	if !ok || !h.InTranscript || h.Snippet != "Call the plumber" || h.MsgID != "v1" {
		t.Fatalf("got %+v, %v; want a transcript hit for v1", h, ok)
	}
	if _, ok := fakeSearchHit(m, "electrician"); ok {
		t.Error("matched a word in neither text nor transcript")
	}
	if h, ok := fakeSearchHit(Message{ID: "t1", Text: "the plumber"}, "plumber"); !ok || h.InTranscript {
		t.Errorf("text hit: got %+v, %v; want InTranscript false", h, ok)
	}
}
