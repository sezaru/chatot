package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// A downloaded photo with a caption is a bubble of one size, like the
// undownloaded tile it replaces. The photo used to be a GtkPicture, whose
// height follows the width it is measured at: the thread measured the row
// wider than the bubble ended up, so a tall portrait left a band of empty
// bubble under its caption.
func TestPhotoBubble_MeasuresOneSize(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	path := filepath.Join(t.TempDir(), "portrait.png")
	pb := gdkpixbuf.NewPixbuf(gdkpixbuf.ColorspaceRGB, false, 8, 300, 580)
	pb.Fill(0xffffffff)
	if err := pb.Savev(path, "png", nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Memoised, so the row takes the decoded picture at once.
	tex := gdk.NewTextureForPixbuf(pb)
	pictureTextures.put(path+"|"+strconv.Itoa(inlinePhotoSide*2), tex, textureBytes(tex))

	caption := "seria algo +- assim, basicamente a água entra por um lado e sai pelo outro filtrada. " +
		"Mas sim, imaginei que nessa área comum não daria para botar"
	msg := client.Message{ID: "p", FromMe: true, TS: time.Now().Unix(),
		Attachment: &client.Attachment{Kind: "image", MimeType: "image/png", LocalPath: path, Caption: caption}}
	vm := bubbleVM(msg, nil, nil, time.Now())
	r := newThreadRow()
	r.render(msg, vm, bubbleHooks{})

	bubble := gtk.BaseWidget(r.bubble)
	min, nat, _, _ := bubble.Measure(gtk.OrientationHorizontal, -1)
	if min != nat {
		t.Errorf("bubble width = %d/%d (min/natural); a photo bubble has one width, not a range", min, nat)
	}
	_, hNat, _, _ := bubble.Measure(gtk.OrientationVertical, nat)
	_, hWide, _, _ := bubble.Measure(gtk.OrientationVertical, nat+200)
	if hNat != hWide {
		t.Errorf("bubble height = %d at its width, %d measured 200px wider; a photo must not grow with the width it is measured at", hNat, hWide)
	}
	if !strings.Contains(r.body.Text(), "filtrada") {
		t.Errorf("caption not rendered: %q", r.body.Text())
	}
}

// A reply with a photo is as wide as the photo: a long quoted message wraps
// inside it rather than stretching the bubble across the thread, which left
// the photo at one edge and the caption floating in the middle.
func TestPhotoBubble_QuoteWrapsToThePhoto(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	path := filepath.Join(t.TempDir(), "photo.png")
	pb := gdkpixbuf.NewPixbuf(gdkpixbuf.ColorspaceRGB, false, 8, 600, 800)
	pb.Fill(0xffffffff)
	if err := pb.Savev(path, "png", nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	tex := gdk.NewTextureForPixbuf(pb)
	pictureTextures.put(path+"|"+strconv.Itoa(inlinePhotoSide*2), tex, textureBytes(tex))

	quoted := strings.Repeat("In her uniform there is a small metal thing that looks a bit odd. ", 6)
	msg := client.Message{ID: "p", FromMe: true, TS: time.Now().Unix(),
		ReplyTo:    &client.MsgRef{MsgID: "q", Text: quoted},
		Attachment: &client.Attachment{Kind: "image", MimeType: "image/png", LocalPath: path, Caption: "Just to reiterate, here is an image"}}
	vm := bubbleVM(msg, nil, nil, time.Now())
	if !vm.HasQuote {
		t.Fatal("the reply has no quote to measure")
	}
	r := newThreadRow()
	r.render(msg, vm, bubbleHooks{})

	_, photo, _, _ := gtk.BaseWidget(r.content).Measure(gtk.OrientationHorizontal, -1)
	_, bubble, _, _ := gtk.BaseWidget(r.bubble).Measure(gtk.OrientationHorizontal, -1)
	// The bubble's own padding is all it may add to the photo.
	if bubble > photo+40 {
		t.Errorf("bubble is %dpx wide around a %dpx photo; the quote widened it", bubble, photo)
	}
}
