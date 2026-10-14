package ui

import (
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

func TestFoldsMedia(t *testing.T) {
	defer func(was bool) { CollapseMedia = was }(CollapseMedia)
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
	CollapseMedia = false
	if foldsMedia(mediaView{Kind: "image"}) {
		t.Error("a photo folds with the preference off")
	}
	CollapseMedia = true
	for _, c := range cases {
		if got := foldsMedia(c.mv); got != c.want {
			t.Errorf("foldsMedia(%+v) = %v, want %v", c.mv, got, c.want)
		}
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

// The folded line builds the attachment only when first unfolded, folds
// it away again, and reports each change so a rebuilt row keeps it.
func TestMediaFoldToggles(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	builds := 0
	var heard []bool
	body := gtk.NewLabel("picture")
	fold := buildMediaFold(nil, "Photo", "", false,
		func() gtk.Widgetter { builds++; return body },
		func(open bool) { heard = append(heard, open) }).(*gtk.Box)
	head := fold.FirstChild().(*gtk.Button)
	if builds != 0 {
		t.Fatal("a folded attachment was built before it was unfolded")
	}
	head.Emit("clicked")
	if builds != 1 || !body.Visible() {
		t.Fatalf("after unfolding: builds = %d, visible = %v", builds, body.Visible())
	}
	head.Emit("clicked")
	if builds != 1 || body.Visible() {
		t.Errorf("after folding back: builds = %d, visible = %v", builds, body.Visible())
	}
	if len(heard) != 2 || !heard[0] || heard[1] {
		t.Errorf("onToggle heard %v, want [true false]", heard)
	}

	open := buildMediaFold(nil, "Photo", "", true, func() gtk.Widgetter { return gtk.NewLabel("x") }, nil).(*gtk.Box)
	if open.LastChild() == open.FirstChild() {
		t.Error("an attachment the reader left open came back folded")
	}
}
