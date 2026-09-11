package store

import (
	"path/filepath"
	"strings"
	"testing"
)

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

func TestSearchInChatScopesToChatAndOrdersOldestFirst(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertChat(ChatRow{JID: "b@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "pizza tonight?", TS: 2}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m2", Text: "yes pizza", TS: 1}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "b@s.whatsapp.net", MsgID: "m3", Text: "pizza too", TS: 3}))

	hits, err := s.SearchInChat("a@s.whatsapp.net", "pizza", 10)
	must(t, err)

	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2 (scoped to chat a): %+v", len(hits), hits)
	}
	if hits[0].MsgID != "m2" || hits[1].MsgID != "m1" {
		t.Errorf("got order %s, %s, want m2, m1 (oldest first)", hits[0].MsgID, hits[1].MsgID)
	}
	for _, h := range hits {
		if h.ChatJID != "a@s.whatsapp.net" {
			t.Errorf("hit %+v leaked from another chat", h)
		}
	}
}

func TestSearchInChatEmptyQueryReturnsNoHits(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "m1", Text: "hello", TS: 1}))

	hits, err := s.SearchInChat("a@s.whatsapp.net", "   ", 10)
	must(t, err)
	if len(hits) != 0 {
		t.Errorf("got %d hits, want 0 for blank query", len(hits))
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

func TestSearchFindsVoiceTranscript(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "v1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "v1", Kind: "audio", MimeType: "audio/ogg"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "t1", Text: "the plumber comes tomorrow", TS: 2}))
	must(t, s.SetMediaTranscript("a@s.whatsapp.net", "v1", "call the plumber about the kitchen sink"))

	hits, err := s.Search("plumber", 10)
	must(t, err)
	got := map[string]SearchHit{}
	for _, h := range hits {
		got[h.MsgID] = h
	}
	v, ok := got["v1"]
	if !ok {
		t.Fatalf("Search: got %+v, want a hit for the voice note v1", hits)
	}
	if !v.InTranscript {
		t.Error("v1: InTranscript = false, want true")
	}
	if !strings.Contains(v.Snippet, "[plumber]") {
		t.Errorf("v1: Snippet = %q, want the transcript's match marked", v.Snippet)
	}
	if tt, ok := got["t1"]; !ok || tt.InTranscript {
		t.Errorf("t1: got %+v (present %v), want a text hit with InTranscript false", tt, ok)
	}

	inChat, err := s.SearchInChat("a@s.whatsapp.net", "kitchen", 0)
	must(t, err)
	if len(inChat) != 1 || inChat[0].MsgID != "v1" || !inChat[0].InTranscript {
		t.Fatalf("SearchInChat: got %+v, want only v1 from its transcript", inChat)
	}

	// A new transcript replaces the old one in the index.
	must(t, s.SetMediaTranscript("a@s.whatsapp.net", "v1", "nothing to report"))
	if hits, err := s.SearchInChat("a@s.whatsapp.net", "kitchen", 0); err != nil || len(hits) != 0 {
		t.Errorf("after re-transcribing: got %+v, %v, want no hit", hits, err)
	}
}

// A store whose messages_fts predates the transcript column comes back
// with the transcripts it already had indexed.
func TestSearchIndexRebuiltForTranscriptsOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatot.db")
	s, err := Open(path)
	must(t, err)
	must(t, s.UpsertChat(ChatRow{JID: "a@s.whatsapp.net"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "v1", TS: 1}))
	must(t, s.UpsertMedia(MediaRow{ChatJID: "a@s.whatsapp.net", MsgID: "v1", Kind: "audio"}))
	must(t, s.UpsertMessage(MessageRow{ChatJID: "a@s.whatsapp.net", MsgID: "t1", Text: "hello there", TS: 2}))
	// Back to the one-column index, with the transcript only in media,
	// the way a store written before this release has it.
	_, err = s.db.Exec(`
		DROP TRIGGER messages_fts_ai; DROP TRIGGER messages_fts_ad; DROP TRIGGER messages_fts_au;
		DROP TABLE messages_fts;
		CREATE VIRTUAL TABLE messages_fts USING fts5(text, content='messages', content_rowid='rowid');
		INSERT INTO messages_fts(messages_fts) VALUES ('rebuild');
		CREATE TRIGGER messages_fts_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
		END;
		CREATE TRIGGER messages_fts_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
		END;
		CREATE TRIGGER messages_fts_au AFTER UPDATE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
			INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
		END;
		UPDATE media SET transcript = 'call the plumber' WHERE msg_id = 'v1';
	`)
	must(t, err)
	must(t, s.Close())

	s, err = Open(path)
	must(t, err)
	defer s.Close()
	hits, err := s.Search("plumber", 10)
	must(t, err)
	if len(hits) != 1 || hits[0].MsgID != "v1" || !hits[0].InTranscript {
		t.Fatalf("after reopen: got %+v, want the transcript hit for v1", hits)
	}
	if hits, err := s.Search("hello", 10); err != nil || len(hits) != 1 || hits[0].MsgID != "t1" {
		t.Errorf("after reopen: text search got %+v, %v, want t1", hits, err)
	}
	// The rebuilt index keeps following writes.
	must(t, s.SetMediaTranscript("a@s.whatsapp.net", "v1", "the electrician instead"))
	if hits, err := s.Search("electrician", 10); err != nil || len(hits) != 1 || hits[0].MsgID != "v1" {
		t.Errorf("after reopen: new transcript got %+v, %v, want v1", hits, err)
	}
}
