package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// trayPNG writes a w×h white PNG for the tray to queue.
func trayPNG(t *testing.T, name string, w, h int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	pb := gdkpixbuf.NewPixbuf(gdkpixbuf.ColorspaceRGB, false, 8, w, h)
	pb.Fill(0xffffffff)
	if err := pb.Savev(path, "png", nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}

// withClass is the first widget under w (w included) carrying class.
func withClass(w gtk.Widgetter, class string) *gtk.Widget {
	base := gtk.BaseWidget(w)
	if base.HasCSSClass(class) {
		return base
	}
	for c := base.FirstChild(); c != nil; c = gtk.BaseWidget(c).NextSibling() {
		if found := withClass(c, class); found != nil {
			return found
		}
	}
	return nil
}

// layOut allocates the tray as a w×h pane.
func layOut(tray *AttachTray, w, h int) {
	tray.Measure(gtk.OrientationHorizontal, -1)
	tray.Measure(gtk.OrientationVertical, w)
	tray.Allocate(w, h, -1, nil)
}

// A photo queued in a large window fills it, up to the photo's own size,
// rather than the mockup's 460px slot, which left a phone screenshot a
// small card in the middle of a 2000px pane.
func TestTrayStage_PhotoFillsALargePane(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	path := trayPNG(t, "portrait.png", 1000, 1500)
	tray := NewAttachTray(nil, nil)
	tray.Open([]string{path})
	layOut(tray, 1800, 1400)

	card := withClass(tray, "chatot-tray-image")
	if card == nil {
		t.Fatal("no stage card")
	}
	w, h := card.Width(), card.Height()
	if h < 900 {
		t.Errorf("stage card is %d×%d in a 1800×1400 pane; a 1000×1500 photo should fill its height", w, h)
	}
	if w > 1000+trayStagePad || h > 1500+trayStagePad {
		t.Errorf("stage card is %d×%d, past the photo's own 1000×1500", w, h)
	}
	if ratio := float64(w) / float64(h); ratio < 0.6 || ratio > 0.73 {
		t.Errorf("stage card is %d×%d (%.2f); it should take the photo's 2:3 shape", w, h, ratio)
	}

	// A picture smaller than the slot keeps the mockup's 4:3 card.
	tray.Close()
	tray.Open([]string{trayPNG(t, "icon.png", 64, 64)})
	layOut(tray, 1800, 1400)
	card = withClass(tray, "chatot-tray-image")
	if w := card.Width(); w != trayStageW {
		t.Errorf("small picture's card is %dpx wide, want the %dpx slot", w, trayStageW)
	}
}

// Typing a caption leaves the strip alone: rebuilding it on every keystroke
// decoded each queued picture again and made typing lag.
func TestTrayCaption_TypingKeepsTheStrip(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	var paths []string
	for _, n := range []string{"a.png", "b.png", "c.png", "d.png"} {
		paths = append(paths, trayPNG(t, n, 1200, 1600))
	}
	tray := NewAttachTray(nil, nil)
	tray.Open(paths)
	// Let the helpers land, which redraws the strip with the thumbnails.
	for i := 0; i < 500 && tray.previews[paths[3]].Thumb == nil; i++ {
		glib.MainContextDefault().Iteration(false)
		time.Sleep(10 * time.Millisecond)
	}
	if tray.previews[paths[3]].Thumb == nil {
		t.Fatal("thumbnails never landed")
	}
	first := glib.BaseObject(tray.strip.FirstChild()).Native()

	tray.caption.SetText("Mais alguns exemplos")
	if got := glib.BaseObject(tray.strip.FirstChild()).Native(); got != first {
		t.Error("typing a caption rebuilt the strip")
	}
	if !tray.markers[0].Visible() {
		t.Error("the captioned tile shows no 💬")
	}
	if tray.items[0].Caption != "Mais alguns exemplos" {
		t.Errorf("caption = %q", tray.items[0].Caption)
	}
	tray.caption.SetText("")
	if tray.markers[0].Visible() {
		t.Error("💬 stayed after the caption was cleared")
	}
}
