package ui

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// imageViewerSubtitle is the viewer's header subline: who sent the picture
// and when ("You · Today, 11:13" / "Yesterday, 09:02"), reusing the
// conversation's day wording.
func imageViewerSubtitle(msg client.Message, now time.Time) string {
	when := dayText(msg.TS, now) + ", " + time.Unix(msg.TS, 0).Format("15:04")
	if msg.FromMe {
		return "You · " + when
	}
	return when
}

// suggestedImageName is the file name the Save dialog proposes: the
// attachment's own name when WhatsApp sent one, else photo-<date>.<ext>
// from the cached file's extension.
func suggestedImageName(msg client.Message, path string) string {
	if msg.Attachment != nil && msg.Attachment.Filename != "" {
		return msg.Attachment.Filename
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".jpg"
	}
	return "photo-" + time.Unix(msg.TS, 0).Format("2006-01-02-150405") + ext
}

// showImageViewer opens msg's downloaded picture at full size in its own
// window over parent: a dark stage with the image fitted to the window (the
// zoom button or a click on the picture toggles actual size in a scroller)
// and a header with Forward, Copy, Save as… and Open. Esc closes. The
// mockup has no such screen, so this follows plain Adwaita conventions.
func showImageViewer(parent *gtk.Window, path string, msg client.Message, forward func(client.Message)) {
	texture, err := gdk.NewTextureFromFilename(path)
	if err != nil {
		openFile(path) // not decodable here: hand it to the desktop's viewer
		return
	}
	now := time.Now()
	win := gtk.NewWindow()
	win.SetTitle("Photo")
	win.AddCSSClass("chatot-image-viewer")
	if parent != nil {
		win.SetTransientFor(parent)
		w, h := parent.DefaultSize()
		if w > 0 && h > 0 {
			win.SetDefaultSize(w*9/10, h*9/10)
		}
	}
	if w, _ := win.DefaultSize(); w <= 0 {
		win.SetDefaultSize(960, 720)
	}

	toasts := adw.NewToastOverlay()
	view := adw.NewToolbarView()

	header := adw.NewHeaderBar()
	title := adw.NewWindowTitle("Photo", imageViewerSubtitle(msg, now))
	header.SetTitleWidget(title)

	pic := gtk.NewPictureForPaintable(texture)
	pic.SetContentFit(gtk.ContentFitContain)
	pic.SetCanShrink(true)
	pic.SetHExpand(true)
	pic.SetVExpand(true)
	pic.SetHAlign(gtk.AlignCenter)
	pic.SetVAlign(gtk.AlignCenter)
	stage := gtk.NewScrolledWindow()
	stage.AddCSSClass("chatot-image-stage")
	stage.SetChild(pic)
	stage.SetHExpand(true)
	stage.SetVExpand(true)

	// Fit ⇄ actual size: fitted, the picture may shrink to the window;
	// at actual size it asks for the texture's own pixels and the stage
	// scrolls.
	actualSize := false
	zoom := gtk.NewButtonFromIconName("zoom-original-symbolic")
	zoom.SetTooltipText("Actual size")
	setZoom := func(actual bool) {
		actualSize = actual
		pic.SetCanShrink(!actual)
		if actual {
			zoom.SetIconName("zoom-fit-best-symbolic")
			zoom.SetTooltipText("Fit to window")
		} else {
			zoom.SetIconName("zoom-original-symbolic")
			zoom.SetTooltipText("Actual size")
		}
	}
	zoom.ConnectClicked(func() { setZoom(!actualSize) })
	header.PackStart(zoom)
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) { setZoom(!actualSize) })
	pic.AddController(click)
	pic.SetCursorFromName("pointer")

	open := gtk.NewButtonWithLabel("Open")
	open.SetTooltipText("Open with the default application")
	open.ConnectClicked(func() { openFile(path) })
	header.PackEnd(open)

	save := gtk.NewButtonWithLabel("Save as…")
	save.ConnectClicked(func() {
		fd := gtk.NewFileDialog()
		fd.SetTitle("Save photo")
		fd.SetInitialName(suggestedImageName(msg, path))
		fd.Save(context.Background(), win, func(res gio.AsyncResulter) {
			file, err := fd.SaveFinish(res)
			if err != nil {
				return // cancelled
			}
			if err := copyFile(path, file.Path()); err != nil {
				showToast(toasts, "Couldn't save the photo: "+err.Error())
				return
			}
			showToast(toasts, "Saved to "+file.Path())
		})
	})
	header.PackEnd(save)

	copyBtn := gtk.NewButtonWithLabel("Copy")
	copyBtn.SetTooltipText("Copy the photo to the clipboard")
	copyBtn.ConnectClicked(func() {
		gdk.DisplayGetDefault().Clipboard().SetTexture(texture)
		showToast(toasts, "Photo copied to clipboard")
	})
	header.PackEnd(copyBtn)

	if forward != nil {
		fwd := gtk.NewButtonWithLabel("Forward")
		fwd.AddCSSClass("suggested-action")
		fwd.SetTooltipText("Send this photo to another chat")
		fwd.ConnectClicked(func() {
			win.Close()
			forward(msg)
		})
		header.PackEnd(fwd)
	}

	view.AddTopBar(header)
	view.SetContent(stage)
	toasts.SetChild(view)
	win.SetChild(toasts)

	keys := gtk.NewEventControllerKey()
	keys.ConnectKeyPressed(func(keyval, _ uint, _ gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			win.Close()
			return true
		}
		return false
	})
	win.AddController(keys)
	win.Present()
}

// openOnClick makes a downloaded picture clickable: a pointer cursor and a
// click that hands the cached file to open (the full-size viewer).
func openOnClick(widget gtk.Widgetter, path string, open func(path string)) gtk.Widgetter {
	if open == nil {
		return widget
	}
	w := gtk.BaseWidget(widget)
	w.SetCursorFromName("pointer")
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(int, float64, float64) { open(path) })
	w.AddController(click)
	return widget
}
