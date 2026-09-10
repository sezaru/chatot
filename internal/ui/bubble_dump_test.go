package ui

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"chatot/internal/client"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The safety net for reusing a row's widgets across binds. A row GtkListView
// recycles from one message onto another has to end up structurally identical
// to a row built for that message from scratch, or the reader sees the previous
// message's quote, tick or author line. bubbleDump renders a tree the way a
// reader reads it (nesting, CSS classes, label text, hidden-ness) and the tests
// compare dumps, so a reuse path that forgets to clear something shows up as a
// diff rather than as a bug report. It caught two on the way in: a parked hover
// pair kept the side and the button order the previous message left it on.

// sortedClasses puts a widget's CSS classes in a fixed order. GTK returns
// them in whatever order they were added, which differs between a widget
// built for one message and the same widget re-toggled onto another; the
// order changes nothing a reader sees, so comparing it would only produce
// false alarms.
func sortedClasses(base *gtk.Widget) []string {
	cs := append([]string(nil), base.CSSClasses()...)
	sort.Strings(cs)
	return cs
}

func bubbleDump(w gtk.Widgetter) string {
	var b strings.Builder
	dumpWidget(&b, w, 0)
	return b.String()
}

func dumpWidget(b *strings.Builder, w gtk.Widgetter, depth int) {
	base := gtk.BaseWidget(w)
	obj := base.Object.Cast()
	fmt.Fprintf(b, "%s%T", strings.Repeat("  ", depth), obj)
	if cs := sortedClasses(base); len(cs) > 0 {
		fmt.Fprintf(b, " .%s", strings.Join(cs, "."))
	}
	if !base.Visible() {
		b.WriteString(" [hidden]")
	}
	if lbl, ok := obj.(*gtk.Label); ok {
		fmt.Fprintf(b, " text=%q", lbl.Label())
	}
	b.WriteString("\n")
	for c := base.FirstChild(); c != nil; c = gtk.BaseWidget(c).NextSibling() {
		dumpWidget(b, c, depth+1)
	}
}

// bubbleCases are the shapes a recycled row has to be able to turn into. They
// deliberately differ in the parts that are easy to leave behind: the author
// line, the quote, the day separator, the tick, the reactions overlay and the
// tombstone (which suppresses the hover affordances entirely).
// testHooks carry a resolved avatar cache: buildAvatar only reaches for the
// client when a jid is unresolved, and these tests have no client.
func testHooks() bubbleHooks {
	cache := newAvatarCache()
	for _, jid := range []string{"1@s.whatsapp.net", "alice@s.whatsapp.net", "bob@s.whatsapp.net"} {
		cache.paths[jid] = ""
	}
	return bubbleHooks{avatars: cache}
}

func bubbleCases(now time.Time) []struct {
	name   string
	msg    client.Message
	prev   *client.Message
	author string
} {
	ts := now.Unix()
	yesterday := now.Add(-26 * time.Hour).Unix()
	return []struct {
		name   string
		msg    client.Message
		prev   *client.Message
		author string
	}{
		{"incoming-text", client.Message{ID: "a", Text: "hello there", FromJID: "1@s.whatsapp.net", TS: ts}, nil, ""},
		{"outgoing-text", client.Message{ID: "b", Text: "hi back", FromMe: true, TS: ts, Status: 3}, nil, ""},
		{"outgoing-pending", client.Message{ID: "c", Text: "sending", FromMe: true, TS: ts}, nil, ""},
		{"incoming-quote", client.Message{ID: "d", Text: "answering", FromJID: "1@s.whatsapp.net", TS: ts,
			ReplyTo: &client.MsgRef{MsgID: "a"}}, nil, ""},
		{"incoming-forwarded", client.Message{ID: "e", Text: "passed on", FromJID: "1@s.whatsapp.net", TS: ts, Forwarded: true}, nil, ""},
		{"incoming-edited", client.Message{ID: "f", Text: "fixed typo", FromJID: "1@s.whatsapp.net", TS: ts, Edited: true}, nil, ""},
		{"deleted", client.Message{ID: "g", FromJID: "1@s.whatsapp.net", TS: ts, Deleted: true}, nil, ""},
		{"reacted", client.Message{ID: "h", Text: "nice", FromJID: "1@s.whatsapp.net", TS: ts,
			Reactions: map[string][]string{"👍": {"2@s.whatsapp.net"}}}, nil, ""},
		{"day-separator", client.Message{ID: "i", Text: "new day", FromJID: "1@s.whatsapp.net", TS: ts},
			&client.Message{ID: "h0", Text: "old day", TS: yesterday}, ""},
		{"long-text", client.Message{ID: "j", Text: strings.Repeat("a long sentence that keeps going. ", 40),
			FromJID: "1@s.whatsapp.net", TS: ts}, nil, ""},
		{"emoji-only", client.Message{ID: "k", Text: "🎉", FromJID: "1@s.whatsapp.net", TS: ts}, nil, ""},
		{"group-alice", client.Message{ID: "ga", Text: "from alice", FromJID: "alice@s.whatsapp.net", TS: ts}, nil, "Alice"},
		{"group-bob", client.Message{ID: "gb", Text: "from bob", FromJID: "bob@s.whatsapp.net", TS: ts}, nil, "Bob"},
		{"group-alice-again", client.Message{ID: "ga2", Text: "alice once more", FromJID: "alice@s.whatsapp.net", TS: ts}, nil, "Alice"},
		{"failed", client.Message{ID: "l", Text: "never left", FromMe: true, TS: ts, Status: client.MessageStatusFailed}, nil, ""},
		{"sending", client.Message{ID: "l2", Text: "on its way", FromMe: true, TS: ts, Status: client.MessageStatusPending}, nil, ""},
		{"delivered", client.Message{ID: "m", Text: "two ticks", FromMe: true, TS: ts, Status: client.MessageStatusDelivered}, nil, ""},
		{"link-preview", client.Message{ID: "n", Text: "look at https://example.com", FromJID: "1@s.whatsapp.net", TS: ts,
			LinkPreview: &client.LinkPreview{URL: "https://example.com", Title: "Example", Description: "a page"}}, nil, ""},
		{"media-captioned", client.Message{ID: "o", FromJID: "1@s.whatsapp.net", TS: ts,
			Attachment: &client.Attachment{Kind: "image", Filename: "photo.jpg", MimeType: "image/jpeg", Caption: "on holiday"}}, nil, ""},
		{"media-bare", client.Message{ID: "p", FromJID: "1@s.whatsapp.net", TS: ts,
			Attachment: &client.Attachment{Kind: "document", Filename: "notes.pdf", MimeType: "application/pdf"}}, nil, ""},
		{"location", client.Message{ID: "q", FromJID: "1@s.whatsapp.net", TS: ts,
			Location: &client.Location{Name: "The Pier", Address: "1 Seafront", Latitude: 1.5, Longitude: -2.5}}, nil, ""},
		{"contact", client.Message{ID: "r", FromJID: "1@s.whatsapp.net", TS: ts,
			Contact: &client.Contact{DisplayName: "Carol", Phones: []string{"+44 1234"}}}, nil, ""},
		{"poll", client.Message{ID: "s", FromJID: "1@s.whatsapp.net", TS: ts,
			Poll: &client.Poll{Name: "Lunch?", SelectableCount: 1, Options: []client.PollOption{{Name: "Yes", Count: 2}, {Name: "No"}}}}, nil, ""},
		{"call", client.Message{ID: "t", FromJID: "1@s.whatsapp.net", TS: ts,
			CallLog: &client.CallLog{Video: true, Outcome: "missed"}}, nil, ""},
		{"event", client.Message{ID: "u", FromJID: "1@s.whatsapp.net", TS: ts,
			EventInvite: &client.EventInvite{Name: "Standup", Location: "Room 2", StartTS: ts}}, nil, ""},
	}
}

func testVM(msg client.Message, prev *client.Message, author string, now time.Time) bubbleView {
	byID := map[string]client.Message{"a": {ID: "a", Text: "hello there"}}
	vm := bubbleVM(msg, prev, byID, now)
	// A group thread's author line is filled by fillRow, not by bubbleVM, and
	// it is what puts an avatar beside the bubble.
	vm.Author = author
	return vm
}

// testBubble renders a message into a row of its own, the way a row that has
// never held anything else would.
func testBubble(t *testing.T, msg client.Message, prev *client.Message, author string, now time.Time) string {
	t.Helper()
	r := newThreadRow()
	r.render(msg, testVM(msg, prev, author, now), testHooks())
	return bubbleDump(r.wrapper)
}

// TestBubbleDumpIsDeterministic pins the dump itself: two builds of the same
// message must read the same, or every later comparison is meaningless.
func TestBubbleDumpIsDeterministic(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	now := mustParse(t, "2026-08-30 12:00:00")
	for _, c := range bubbleCases(now) {
		t.Run(c.name, func(t *testing.T) {
			first := testBubble(t, c.msg, c.prev, c.author, now)
			second := testBubble(t, c.msg, c.prev, c.author, now)
			if first != second {
				t.Errorf("two builds of %s differ:\n--- first ---\n%s\n--- second ---\n%s", c.name, first, second)
			}
			if strings.TrimSpace(first) == "" {
				t.Errorf("%s dumped nothing", c.name)
			}
		})
	}
}

// TestBubbleCasesAreDistinct guards the table: if two cases dump the same
// tree, one of them is not testing what its name claims and a reuse bug could
// hide behind it.
func TestBubbleCasesAreDistinct(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	now := mustParse(t, "2026-08-30 12:00:00")
	seen := map[string]string{}
	for _, c := range bubbleCases(now) {
		dump := testBubble(t, c.msg, c.prev, c.author, now)
		if other, dup := seen[dump]; dup {
			t.Errorf("case %s dumps the same tree as %s", c.name, other)
		}
		seen[dump] = c.name
	}
}

// TestRecycledRowMatchesFreshRow is the invariant the whole reuse rests on: a
// row carried from one message to another must end up indistinguishable from a
// row that only ever held the second message. Every ordered pair is tried, so a
// leftover quote, tick, avatar, reaction strip or parked hover pair fails here
// rather than on screen.
func TestRecycledRowMatchesFreshRow(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	now := mustParse(t, "2026-08-30 12:00:00")
	cases := bubbleCases(now)
	for _, first := range cases {
		for _, second := range cases {
			if first.name == second.name {
				continue
			}
			t.Run(first.name+"->"+second.name, func(t *testing.T) {
				recycled := newThreadRow()
				recycled.render(first.msg, testVM(first.msg, first.prev, first.author, now), testHooks())
				recycled.render(second.msg, testVM(second.msg, second.prev, second.author, now), testHooks())

				want := testBubble(t, second.msg, second.prev, second.author, now)
				if got := bubbleDump(recycled.wrapper); got != want {
					t.Errorf("row recycled from %s onto %s differs from a fresh %s:\n--- recycled ---\n%s\n--- fresh ---\n%s",
						first.name, second.name, second.name, got, want)
				}
			})
		}
	}
}

// TestTypingRowRecycles covers the sentinel, which takes a different path
// through the row than any message does.
func TestTypingRowRecycles(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	now := mustParse(t, "2026-08-30 12:00:00")
	msg := bubbleCases(now)[0]

	r := newThreadRow()
	r.render(msg.msg, testVM(msg.msg, msg.prev, msg.author, now), testHooks())
	r.renderTyping()
	r.render(msg.msg, testVM(msg.msg, msg.prev, msg.author, now), testHooks())

	if got, want := bubbleDump(r.wrapper), testBubble(t, msg.msg, msg.prev, msg.author, now); got != want {
		t.Errorf("row recycled through the typing sentinel differs:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestWidgetKeySurvivesWrapperRoundTrip pins why rows are keyed by GObject address
// rather than by the wrapper. gotk4 hands back a fresh *gtk.Box for every call
// that returns the same GtkBox, so a map keyed on the Go pointer stores under
// one address and looks up under another: every bind missed and the thread
// rendered blank.
func TestRowKeySurvivesWrapperRoundTrip(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	r := newThreadRow()
	clamp := chatClamp(r.wrapper)
	roundTripped, ok := clamp.Child().(*gtk.Box)
	if !ok {
		t.Fatal("clamp child is not the wrapper box")
	}
	if roundTripped == r.wrapper {
		t.Skip("gotk4 now returns the same wrapper; the key indirection is moot")
	}
	if got, want := widgetKey(roundTripped), widgetKey(r.wrapper); got != want {
		t.Fatalf("widgetKey after round trip = %#x, want %#x", got, want)
	}
}

// avatarKey identifies the widget in a row's avatar slot, 0 when it is empty.
func avatarKey(r *threadRow) uintptr {
	child := r.avatarSlot.FirstChild()
	if child == nil {
		return 0
	}
	return widgetKey(child)
}

// TestSameSenderKeepsItsAvatarWidget is the point of the cache: a group runs
// long stretches from one sender, and rebinding onto another of their messages
// must reuse the picture rather than build a second one. Changing sender must
// still replace it.
func TestSameSenderKeepsItsAvatarWidget(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	now := mustParse(t, "2026-08-30 12:00:00")
	var alice1, alice2, bob struct {
		msg    client.Message
		author string
	}
	for _, c := range bubbleCases(now) {
		switch c.name {
		case "group-alice":
			alice1.msg, alice1.author = c.msg, c.author
		case "group-alice-again":
			alice2.msg, alice2.author = c.msg, c.author
		case "group-bob":
			bob.msg, bob.author = c.msg, c.author
		}
	}

	r, h := newThreadRow(), testHooks()
	r.render(alice1.msg, testVM(alice1.msg, nil, alice1.author, now), h)
	first := avatarKey(r)
	if first == 0 {
		t.Fatal("no avatar beside an incoming group bubble")
	}

	r.render(alice2.msg, testVM(alice2.msg, nil, alice2.author, now), h)
	if got := avatarKey(r); got != first {
		t.Error("rebinding onto the same sender rebuilt the avatar")
	}

	r.render(bob.msg, testVM(bob.msg, nil, bob.author, now), h)
	if got := avatarKey(r); got == first {
		t.Error("rebinding onto another sender kept the previous avatar")
	}
}

// TestNewPictureReplacesAVisibleAvatar covers the generation counter. A row
// sitting on screen when its sender changes their picture used to keep the old
// one until GTK happened to rebind it, which on a still thread is never.
func TestNewPictureReplacesAVisibleAvatar(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	now := mustParse(t, "2026-08-30 12:00:00")
	var alice client.Message
	for _, c := range bubbleCases(now) {
		if c.name == "group-alice" {
			alice = c.msg
		}
	}

	gens := map[string]int{}
	h := testHooks()
	h.avatarGen = func(jid string) int { return gens[jid] }

	r := newThreadRow()
	r.render(alice, testVM(alice, nil, "Alice", now), h)
	before := avatarKey(r)

	// Same picture: a rebind keeps the widget.
	r.render(alice, testVM(alice, nil, "Alice", now), h)
	if avatarKey(r) != before {
		t.Fatal("avatar was rebuilt with no new picture")
	}

	// Alice changed her picture, and the row is still showing her.
	gens["alice@s.whatsapp.net"]++
	r.rebuildAvatar(h)
	if got := avatarKey(r); got == before {
		t.Error("a new picture left the visible avatar alone")
	}
}

// visibleDump is bubbleDump with the hidden parts left out: what a reader
// actually sees, rather than what the row is carrying. A row that reuses a
// widget instead of rebuilding it leaves hidden leftovers where a fresh
// build had nothing at all, so a golden including them could not survive
// the very change it exists to guard.
func visibleDump(w gtk.Widgetter) string {
	var b strings.Builder
	dumpVisible(&b, w, 0)
	return b.String()
}

func dumpVisible(b *strings.Builder, w gtk.Widgetter, depth int) {
	base := gtk.BaseWidget(w)
	if !base.Visible() {
		return
	}
	obj := base.Object.Cast()
	fmt.Fprintf(b, "%s%T", strings.Repeat("  ", depth), obj)
	if cs := sortedClasses(base); len(cs) > 0 {
		fmt.Fprintf(b, " .%s", strings.Join(cs, "."))
	}
	if lbl, ok := obj.(*gtk.Label); ok {
		fmt.Fprintf(b, " text=%q", lbl.Label())
	}
	b.WriteString("\n")
	for c := base.FirstChild(); c != nil; c = gtk.BaseWidget(c).NextSibling() {
		dumpVisible(b, c, depth+1)
	}
}

var updateBubbles = flag.Bool("update-bubbles", false, "rewrite testdata/bubbles.txt from the current rendering")

// TestBubblesRenderAsBefore is the net for changing how a bubble is built.
// The recycling test above only says a recycled row matches a fresh one, so
// it passes just as happily when both are wrong. This one pins what every
// case in the table actually renders, against a file captured before the
// change. Rewrite it with -update-bubbles, and read the diff: every line of
// it is something the reader would have seen differently.
func TestBubblesRenderAsBefore(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	now := mustParse(t, "2026-08-30 12:00:00")
	var b strings.Builder
	for _, c := range bubbleCases(now) {
		fmt.Fprintf(&b, "== %s ==\n%s\n", c.name, testVisibleBubble(t, c.msg, c.prev, c.author, now))
	}
	got := b.String()

	const golden = "testdata/bubbles.txt"
	if *updateBubbles {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("wrote " + golden)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (capture it with: go test ./internal/ui -run TestBubblesRenderAsBefore -update-bubbles)", err)
	}
	if string(want) == got {
		return
	}
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		g, w := lineAt(gotLines, i), lineAt(wantLines, i)
		if g != w {
			t.Fatalf("%s line %d:\n  was  %s\n  now  %s", golden, i+1, w, g)
		}
	}
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(end of file)"
}

func testVisibleBubble(t *testing.T, msg client.Message, prev *client.Message, author string, now time.Time) string {
	t.Helper()
	r := newThreadRow()
	r.render(msg, testVM(msg, prev, author, now), testHooks())
	return visibleDump(r.wrapper)
}
