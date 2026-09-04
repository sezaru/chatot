package ui

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// composerInput is the message pill: a text view that grows with its lines
// up to composerMaxLines and scrolls past that, as WhatsApp's does. Enter
// sends and Shift+Enter breaks the line. The composer and the @ picker
// drive it by character offset.
type composerInput struct {
	*gtk.Overlay
	view        *gtk.TextView
	buf         *gtk.TextBuffer
	placeholder *gtk.Label
	onActivate  func()
	// enterShift is whether the Return being processed had Shift held:
	// noted on the key press, consumed by the newline it inserts.
	enterPending bool
	enterShift   bool
	// onPasteFiles and onPasteImage take a paste that is files or a
	// picture rather than text (see ConnectPasteAttachments).
	onPasteFiles func(paths []string)
	onPasteImage func(t *gdk.Texture)
	// pasteAsText is set while a paste is being re-emitted as text after
	// the clipboard's files or picture could not be read.
	pasteAsText bool
}

// composerMaxLines is how tall the pill grows before it scrolls.
const composerMaxLines = 6

func newComposerInput() *composerInput {
	view := gtk.NewTextView()
	view.SetWrapMode(gtk.WrapWordChar)
	view.SetAcceptsTab(false)
	view.SetLeftMargin(14)
	view.SetRightMargin(14)
	view.SetTopMargin(7)
	view.SetBottomMargin(7)
	view.AddCSSClass("chatot-composer-text")

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetPropagateNaturalHeight(true)
	scroller.SetChild(view)
	scroller.SetHExpand(true)

	placeholder := gtk.NewLabel("Write a message")
	placeholder.AddCSSClass("chatot-composer-placeholder")
	placeholder.SetHAlign(gtk.AlignStart)
	placeholder.SetVAlign(gtk.AlignEnd)
	placeholder.SetCanTarget(false)
	placeholder.SetMarginStart(14)
	placeholder.SetMarginBottom(7)

	overlay := gtk.NewOverlay()
	overlay.AddCSSClass("chatot-composer-pill")
	overlay.SetChild(scroller)
	overlay.AddOverlay(placeholder)
	overlay.SetHExpand(true)

	in := &composerInput{Overlay: overlay, view: view, buf: view.Buffer(), placeholder: placeholder}

	// Enter is taken at the point the view inserts its newline, after the
	// input method has had the key: an Enter that commits a preedit or
	// ends a compose sequence never reaches the buffer as "\n". The key
	// press only notes whether Shift was held.
	keys := gtk.NewEventControllerKey()
	keys.SetPropagationPhase(gtk.PhaseCapture)
	keys.ConnectKeyPressed(func(keyval, _ uint, state gdk.ModifierType) bool {
		in.enterPending = keyval == gdk.KEY_Return || keyval == gdk.KEY_KP_Enter
		in.enterShift = state&gdk.ShiftMask != 0
		return false
	})
	view.AddController(keys)
	// A paste of files (copied in a file manager) or of a picture (copied
	// from a browser or an editor) becomes an attachment instead of text.
	view.ConnectPasteClipboard(func() {
		if !in.pasteAsText && in.pasteAttachment() {
			view.StopEmission("paste-clipboard")
		}
	})
	in.buf.ConnectInsertText(func(_ *gtk.TextIter, text string, _ int) {
		if text != "\n" || !in.enterPending {
			return
		}
		in.enterPending = false
		if in.enterShift {
			return
		}
		in.buf.StopEmission("insert-text")
		if in.onActivate != nil {
			// After the emission: activating replaces the draft.
			glib.IdleAdd(in.onActivate)
		}
	})

	// The pill's height caps at composerMaxLines of text. The line height
	// comes from the laid-out first line, so it follows the font.
	setCap := func() {
		_, lineH := view.LineYrange(in.buf.StartIter())
		if lineH > 0 {
			scroller.SetMaxContentHeight(lineH*composerMaxLines + view.TopMargin() + view.BottomMargin())
		}
	}
	view.ConnectMap(setCap)
	in.buf.ConnectChanged(func() {
		placeholder.SetVisible(in.buf.CharCount() == 0)
		setCap()
	})
	return in
}

// Text is the whole draft.
func (in *composerInput) Text() string {
	start, end := in.buf.Bounds()
	return in.buf.Text(start, end, true)
}

// SetText replaces the draft and parks the cursor at its end.
func (in *composerInput) SetText(text string) {
	in.buf.SetText(text)
	in.buf.PlaceCursor(in.buf.EndIter())
	// Typing keeps the cursor in view on its own; a draft set whole needs
	// the scroll once the new lines are laid out.
	glib.IdleAdd(func() { in.view.ScrollMarkOnscreen(in.buf.GetInsert()) })
}

// Position is the cursor's offset in characters.
func (in *composerInput) Position() int {
	return in.buf.IterAtMark(in.buf.GetInsert()).Offset()
}

// SetPosition moves the cursor to the character offset pos.
func (in *composerInput) SetPosition(pos int) {
	in.buf.PlaceCursor(in.buf.IterAtOffset(pos))
}

// InsertAtCursor types text at the cursor, leaving the cursor after it.
func (in *composerInput) InsertAtCursor(text string) {
	in.buf.InsertAtCursor(text)
}

// ConnectActivate runs f on a plain Enter.
func (in *composerInput) ConnectActivate(f func()) { in.onActivate = f }

// ConnectChanged runs f whenever the draft changes.
func (in *composerInput) ConnectChanged(f func()) { in.buf.ConnectChanged(f) }

// ConnectCursorMoved runs f when the cursor moves, which every edit does.
func (in *composerInput) ConnectCursorMoved(f func()) {
	in.buf.NotifyProperty("cursor-position", f)
}

// AddController attaches c to the text view, where the keys land.
func (in *composerInput) AddController(c gtk.EventControllerer) { in.view.AddController(c) }

// GrabFocus focuses the text view.
func (in *composerInput) GrabFocus() bool { return in.view.GrabFocus() }

// SetSensitive enables or greys the pill.
func (in *composerInput) SetSensitive(on bool) {
	in.Overlay.SetSensitive(on)
	in.view.SetSensitive(on)
}

// ConnectPasteAttachments routes a paste of files to files and of a
// picture to image; text keeps pasting as text.
func (in *composerInput) ConnectPasteAttachments(files func(paths []string), image func(t *gdk.Texture)) {
	in.onPasteFiles = files
	in.onPasteImage = image
}

// pasteAttachment reads the clipboard as files or as a picture when it
// offers either, handing the result to the paste hooks, and reports
// whether it did (the text paste is then stopped). Files win over a
// picture: a copied image file offers both. A read that fails after all
// falls back to the ordinary text paste, so an odd clipboard never eats
// the keystroke.
func (in *composerInput) pasteAttachment() bool {
	if in.onPasteFiles == nil || in.onPasteImage == nil {
		return false
	}
	clip := in.view.Clipboard()
	formats := clip.Formats()
	switch {
	case formats.ContainGType(gdk.GTypeFileList):
		clip.ReadValueAsync(context.Background(), gdk.GTypeFileList, int(glib.PriorityDefault), func(res gio.AsyncResulter) {
			v, err := clip.ReadValueFinish(res)
			if err != nil {
				in.pasteText()
				return
			}
			if files, ok := v.GoValue().(*gdk.FileList); ok {
				in.onPasteFiles(filePaths(files))
			}
		})
		return true
	case formats.ContainGType(gdk.GTypeTexture):
		clip.ReadTextureAsync(context.Background(), func(res gio.AsyncResulter) {
			t, err := clip.ReadTextureFinish(res)
			if err != nil || t == nil {
				in.pasteText()
				return
			}
			in.onPasteImage(gdk.BaseTexture(t))
		})
		return true
	}
	return false
}

// pasteText runs the text view's own paste once, bypassing the attachment
// interception.
func (in *composerInput) pasteText() {
	in.pasteAsText = true
	in.view.ActivateAction("clipboard.paste", nil)
	in.pasteAsText = false
}

// filePaths is the local paths in a dropped or pasted file list; remote
// files (no path) are skipped.
func filePaths(files *gdk.FileList) []string {
	var paths []string
	for _, f := range files.Files() {
		if p := f.Path(); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}
