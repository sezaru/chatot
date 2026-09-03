package store

import "testing"

func TestMetaRoundTrip(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, err := s.Meta("flag"); err != nil || v != "" {
		t.Fatalf("unset Meta = %q, %v", v, err)
	}
	if err := s.SetMeta("flag", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("flag", "2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Meta("flag"); v != "2" {
		t.Errorf("Meta = %q, want 2", v)
	}
}

func TestMessagePayloadRoundTrip(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.UpsertMessage(MessageRow{ChatJID: "c", MsgID: "m", Text: "", TS: 1, Kind: "location", Payload: `{"lat":1}`}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMessagePayload("c", "m", `{"lat":2}`); err != nil {
		t.Fatal(err)
	}
	kind, payload, err := s.MessagePayload("c", "m")
	if err != nil || kind != "location" || payload != `{"lat":2}` {
		t.Errorf("MessagePayload = %q, %q, %v", kind, payload, err)
	}
	if kind, payload, err := s.MessagePayload("c", "missing"); err != nil || kind != "" || payload != "" {
		t.Errorf("missing MessagePayload = %q, %q, %v", kind, payload, err)
	}
}
