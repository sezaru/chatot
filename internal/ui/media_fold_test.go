package ui

import (
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

func TestFoldableMedia(t *testing.T) {
	cases := []struct {
		mv   mediaView
		want bool
	}{
		{mediaView{Kind: "image"}, true},
		{mediaView{Kind: "video", IsGIF: true}, true},
		{mediaView{Kind: "image", ViewOnce: true}, false},
		{mediaView{Kind: "sticker"}, false},
		{mediaView{Kind: "audio"}, false},
		{mediaView{Kind: "document"}, false},
	}
	for _, c := range cases {
		if got := foldableMedia(c.mv); got != c.want {
			t.Errorf("foldableMedia(%+v) = %v, want %v", c.mv, got, c.want)
		}
	}
	link := client.Message{LinkPreview: &client.LinkPreview{URL: "https://example.com", Title: "Example"}}
	if !foldableMessage(link) {
		t.Error("a link preview cannot fold")
	}
	if foldableMessage(client.Message{Text: "hi"}) {
		t.Error("a plain text message can fold")
	}
}

func TestMediaFoldTitles(t *testing.T) {
	if got := mediaFoldTitle(mediaView{Kind: "video", DurationSecs: 12}); got != "Video · 0:12" {
		t.Errorf("video title = %q", got)
	}
	if got := mediaFoldTitle(mediaView{Kind: "video", IsGIF: true}); got != "GIF" {
		t.Errorf("GIF title = %q", got)
	}
	photo := client.Message{Attachment: &client.Attachment{Kind: "image"}}
	video := client.Message{Attachment: &client.Attachment{Kind: "video"}}
	for _, c := range []struct {
		msgs []client.Message
		want string
	}{
		{[]client.Message{photo, photo, photo}, "3 photos"},
		{[]client.Message{video, video}, "2 videos"},
		{[]client.Message{photo, video}, "2 photos and videos"},
	} {
		if got := albumFoldTitle(c.msgs); got != c.want {
			t.Errorf("albumFoldTitle = %q, want %q", got, c.want)
		}
	}
}

// The preference only sets where an untouched bubble starts: a fold saved
// on the message wins either way, and one just made in the view wins over
// a message read before it was saved.
func TestMessageFoldedFollowsTheReader(t *testing.T) {
	defer func(was bool) { CollapseMedia = was }(CollapseMedia)
	cv := &ConversationView{}
	h := bubbleHooks{mediaFoldOf: cv.mediaFoldOf}
	untouched := client.Message{ID: "m1"}
	collapsed := client.Message{ID: "m2", Fold: client.FoldCollapsed}
	expanded := client.Message{ID: "m3", Fold: client.FoldExpanded}

	for _, pref := range []bool{false, true} {
		CollapseMedia = pref
		if got := h.messageFolded(untouched); got != pref {
			t.Errorf("preference %v, untouched: folded = %v", pref, got)
		}
		if !h.messageFolded(collapsed) {
			t.Errorf("preference %v, saved collapsed: shown expanded", pref)
		}
		if h.messageFolded(expanded) {
			t.Errorf("preference %v, saved expanded: shown collapsed", pref)
		}
	}

	cv.mediaFolds = map[string]client.MediaFold{"m2": client.FoldExpanded}
	if h.messageFolded(collapsed) {
		t.Error("an expand made in the view lost to the message's older fold")
	}
}

func TestMessageMenuFoldRow(t *testing.T) {
	has := func(items []menuItem, label string) bool {
		for _, it := range items {
			if it.Label == label {
				return true
			}
		}
		return false
	}
	if items := messageMenuItems(client.Message{}, messageMenuActions{}); has(items, "Collapse") || has(items, "Expand") {
		t.Error("a message with nothing to fold has a fold row")
	}
	if !has(messageMenuItems(client.Message{}, messageMenuActions{Fold: func() {}}), "Collapse") {
		t.Error("an expanded bubble offers no Collapse")
	}
	if !has(messageMenuItems(client.Message{}, messageMenuActions{Fold: func() {}, Folded: true}), "Expand") {
		t.Error("a collapsed bubble offers no Expand")
	}
}

// Every foldable bubble carries the control; a click on it is the toggle.
func TestFoldButton(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	content := gtk.NewLabel("picture")
	if got := withFoldButton(content, false, true, nil); got != gtk.Widgetter(content) {
		t.Error("a bubble with no conversation behind it got a fold control")
	}
	clicks := 0
	wrap := withFoldButton(content, false, true, func() { clicks++ }).(*gtk.Overlay)
	btn := wrap.LastChild().(*gtk.Button)
	btn.Emit("clicked")
	if clicks != 1 {
		t.Errorf("clicking the fold control toggled %d times, want 1", clicks)
	}
	expands := 0
	row := buildCollapsedRow(gtk.NewLabel("thumb"), "Photo", "775 KB", func() { expands++ }).(*gtk.Button)
	row.Emit("clicked")
	if expands != 1 {
		t.Errorf("clicking a collapsed picture expanded it %d times, want 1", expands)
	}
}
