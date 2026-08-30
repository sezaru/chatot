package store

import "testing"

func TestSearchFindsMessageWithChatNameAndSnippet(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertContact(ContactRow{JID: "a@s.whatsapp.net", PushName: "Ada"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "let's grab pizza tonight", TS: 1}))

	hits, err := s.Search("pizza", 10)
	must(t, err)

	var msgHit *SearchHit
	for i := range hits {
		if hits[i].MsgID == "m1" {
			msgHit = &hits[i]
		}
	}
	if msgHit == nil {
		t.Fatalf("got %+v, want a hit for m1", hits)
	}
	if msgHit.ChatJID != "a@s.whatsapp.net" || msgHit.ChatName != "Ada" {
		t.Errorf("got ChatJID=%q ChatName=%q, want a@s.whatsapp.net / Ada", msgHit.ChatJID, msgHit.ChatName)
	}
	if msgHit.Snippet == "" {
		t.Error("Snippet is empty, want a highlighted excerpt")
	}
}

func TestSearchRanksBetterAndNewerMatchFirst(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	// m1: "pizza" mentioned once, older. m2: "pizza" is the whole (short)
	// message and newer, so bm25 should favor it as more relevant, and the
	// tie-break (ts DESC) also favors it independently.
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "we talked about many things, pizza being one topic among several unrelated ones", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", Text: "pizza", TS: 2}))

	hits, err := s.Search("pizza", 10)
	must(t, err)
	if len(hits) < 2 {
		t.Fatalf("got %d hits, want at least 2", len(hits))
	}
	if hits[0].MsgID != "m2" {
		t.Fatalf("got first hit %q, want m2 to rank first", hits[0].MsgID)
	}
}

func TestSearchMatchesChatName(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "120363000000000000@g.us", IsGroup: true}))
	must(t, s.UpsertGroup(GroupRow{JID: "120363000000000000@g.us", Name: "Weekend Trip"}))

	hits, err := s.Search("weekend", 10)
	must(t, err)

	var found bool
	for _, h := range hits {
		if h.ChatJID == "120363000000000000@g.us" && h.ChatName == "Weekend Trip" {
			found = true
		}
	}
	if !found {
		t.Fatalf("got %+v, want a chat-name hit for Weekend Trip", hits)
	}
}

func TestSearchAdversarialQueriesDoNotError(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hello world", TS: 1}))

	queries := []string{
		`foo"bar`,
		"a AND b",
		"*",
		"   ",
		`"`,
		"NEAR(x y)",
		"col:term",
		"-hello",
	}
	for _, q := range queries {
		if _, err := s.Search(q, 10); err != nil {
			t.Errorf("Search(%q) returned error: %v", q, err)
		}
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	for i, id := range []string{"m1", "m2", "m3", "m4", "m5"} {
		must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: id, Text: "banana split", TS: int64(i + 1)}))
	}

	hits, err := s.Search("banana", 2)
	must(t, err)
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
}

func TestSearchFindsMessagesWrittenBeforeStoreReopen(t *testing.T) {
	// Proves the fts index is populated by backfill on Open, not only by
	// triggers firing on new writes: this uses a real file-backed db so a
	// second Open re-runs schema.sql/backfillFTS against pre-existing rows.
	dir := t.TempDir()
	path := dir + "/chatot-search-backfill.db"

	s1, err := Open(path)
	must(t, err)
	must(t, s1.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s1.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "backfill me please", TS: 1}))
	must(t, s1.Close())

	s2, err := Open(path)
	must(t, err)
	t.Cleanup(func() { _ = s2.Close() })

	hits, err := s2.Search("backfill", 10)
	must(t, err)
	var found bool
	for _, h := range hits {
		if h.MsgID == "m1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("got %+v, want a hit for the pre-existing message m1", hits)
	}
}

func TestBuildFTSQuery(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"single token gets prefix star", "hello", `"hello"*`},
		{"multiple tokens: only last gets prefix star", "hello world", `"hello" "world"*`},
		{"embedded quote is doubled", `foo"bar`, `"foo""bar"*`},
		{"fts operator words are quoted, not parsed", "a AND b", `"a" "AND" "b"*`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildFTSQuery(tc.input)
			if got != tc.want {
				t.Errorf("buildFTSQuery(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
