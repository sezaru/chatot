package ui

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// Helpers shared by the Status, Channels and Communities tabs: the mockup
// draws their captions, empty states, link boxes and menus identically.

// newSectionCaption is the mockup's small uppercase list caption ("RECENT
// UPDATES", "FOLLOWING · 3", "ANNOUNCEMENT GROUP").
func newSectionCaption(text string) *gtk.Label {
	l := gtk.NewLabel(strings.ToUpper(text))
	l.SetXAlign(0)
	l.AddCSSClass("chatot-section-caption")
	return l
}

// newTabEmptyState is the content pane's placeholder when a tab has nothing
// selected: a 66px rounded-square disc, a bold title and a centred hint.
func newTabEmptyState(glyph, title, text string) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 11)
	box.AddCSSClass("chatot-tab-empty")
	box.SetHAlign(gtk.AlignCenter)
	box.SetVAlign(gtk.AlignCenter)
	box.SetVExpand(true)
	box.SetHExpand(true)

	disc := gtk.NewLabel(glyph)
	disc.AddCSSClass("chatot-tab-empty-disc")
	disc.SetSizeRequest(66, 66)
	disc.SetHAlign(gtk.AlignCenter)
	box.Append(disc)

	t := gtk.NewLabel(title)
	t.AddCSSClass("chatot-tab-empty-title")
	box.Append(t)

	h := gtk.NewLabel(text)
	h.AddCSSClass("chatot-tab-empty-hint")
	h.SetWrap(true)
	h.SetJustify(gtk.JustifyCenter)
	h.SetMaxWidthChars(46)
	box.Append(h)
	return box
}

// tabEmptyCopy is the per-tab wording of the empty content pane.
func tabEmptyCopy(tab string) (glyph, title, text string) {
	switch tab {
	case "status":
		return "◌", "Status updates", "Pick an update on the left to view it. Everything you post disappears after 24 hours."
	case "channels":
		return "📣", "Channels", "Channels are one-way broadcasts. Choose one to read its updates, or find new ones to follow."
	case "communities":
		return "🏘", "Communities", "A community groups related chats under one announcement group. Pick one to see its groups."
	default:
		return "💬", "Chats", ""
	}
}

// newLinkBox is the share dialogs' bordered white box: the link in green
// monospace beside a Copy button that reads "Copied" once pressed.
func newLinkBox(link string, onCopy func()) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationHorizontal, 10)
	box.AddCSSClass("chatot-link-box")

	l := gtk.NewLabel(displayLink(link))
	l.SetXAlign(0)
	l.SetHExpand(true)
	l.SetEllipsize(pango.EllipsizeEnd)
	l.SetMaxWidthChars(1)
	l.AddCSSClass("chatot-link-text")
	box.Append(l)

	btn := gtk.NewButtonWithLabel("Copy")
	btn.AddCSSClass("chatot-copy-btn")
	btn.SetVAlign(gtk.AlignCenter)
	btn.ConnectClicked(func() {
		gdk.DisplayGetDefault().Clipboard().SetText(link)
		btn.SetLabel("Copied")
		if onCopy != nil {
			onCopy()
		}
	})
	box.Append(btn)
	return box
}

// newDialogFooter is the right-aligned button row at the foot of a card
// dialog, above a hairline.
func newDialogFooter() *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-dialog-footer")
	row.SetHAlign(gtk.AlignFill)
	return row
}

// newDialogBodyText is a dialog's dim explanatory paragraph.
func newDialogBodyText(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.SetXAlign(0)
	l.SetWrap(true)
	l.AddCSSClass("chatot-dialog-bodytext")
	return l
}

// newChipButton is a dialog's grey secondary button ("Cancel").
func newChipButton(label string, onClick func()) *gtk.Button {
	b := gtk.NewButtonWithLabel(label)
	b.AddCSSClass("chatot-chip-btn")
	if onClick != nil {
		b.ConnectClicked(onClick)
	}
	return b
}

// newPrimaryButton is a dialog's green primary button.
func newPrimaryButton(label string, onClick func()) *gtk.Button {
	b := gtk.NewButtonWithLabel(label)
	b.AddCSSClass("chatot-primary-btn")
	if onClick != nil {
		b.ConnectClicked(onClick)
	}
	return b
}

// attachRowMenu pops a mockup-style menu on a right-click anywhere in row.
// items is called at click time so the rows reflect current state.
func attachRowMenu(row gtk.Widgetter, items func() []menuItem) {
	gesture := gtk.NewGestureClick()
	gesture.SetButton(gdk.BUTTON_SECONDARY)
	gesture.ConnectPressed(func(nPress int, x, y float64) {
		popupMenuAt(row, items(), x, y)
	})
	gtk.BaseWidget(row).AddController(gesture)
}

// popupMenuAt pops a menu of items anchored at (x, y) within parent.
func popupMenuAt(parent gtk.Widgetter, items []menuItem, x, y float64) {
	pop := newMenuPopover(items)
	rect := gdk.NewRectangle(int(x), int(y), 1, 1)
	pop.SetParent(parent)
	pop.ConnectClosed(func() { pop.Unparent() })
	pop.SetPointingTo(&rect)
	pop.Popup()
}

// popupMenuBelow pops a menu of items hanging under btn, right-aligned to
// it the way the mockup's ⋮ menus open.
func popupMenuBelow(btn gtk.Widgetter, items []menuItem) {
	pop := newMenuPopover(items)
	pop.SetParent(btn)
	pop.SetPosition(gtk.PosBottom)
	pop.SetHAlign(gtk.AlignEnd)
	pop.ConnectClosed(func() { pop.Unparent() })
	pop.Popup()
}

// newDotsButton is a 28px flat ⋮ button for a pane header.
func newDotsButton(tooltip string) *gtk.Button {
	b := gtk.NewButtonWithLabel("⋮")
	b.AddCSSClass("flat")
	b.AddCSSClass("chatot-hdr-icon")
	b.SetVAlign(gtk.AlignCenter)
	b.SetTooltipText(tooltip)
	return b
}

// newVerifiedMark is the 13px green ✓ disc after a verified channel's name.
func newVerifiedMark(size int) *gtk.DrawingArea {
	l := newCheckGlyph(size, true)
	l.AddCSSClass("chatot-verified")
	l.SetTooltipText("Verified channel")
	return l
}

// followersText renders a subscriber count the way the mockup does: exact
// with thousands separators under ten thousand, then "38.4K" / "244K".
func followersText(n int) string {
	if n <= 0 {
		return "No followers yet"
	}
	return compactCount(n) + " followers"
}

// compactCount is followersText's number: "9,860", "38.4K", "244K", "1.2M".
func compactCount(n int) string {
	switch {
	case n >= 1_000_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1e6)) + "M"
	case n >= 100_000:
		return fmt.Sprintf("%dK", n/1000)
	case n >= 10_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1e3)) + "K"
	default:
		return groupThousands(n)
	}
}

func trimZero(s string) string { return strings.TrimSuffix(s, ".0") }

// groupThousands inserts commas: 4218 -> "4,218".
func groupThousands(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// viewsText is a channel post's view count: "1,204 views", "96K views".
func viewsText(n int) string {
	if n == 1 {
		return "1 view"
	}
	return compactCount(n) + " views"
}

// createdText renders a creation time as "8 Jan 2024", or "" when unknown.
func createdText(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2 Jan 2006")
}

// pluralCount is "1 update" / "3 updates".
func pluralCount(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// displayNameFor resolves jid to a contact name known from the chat list,
// falling back to a "+number" like posterName does.
func displayNameFor(jid string, names map[string]string) string {
	return posterName(jid, names)
}

// isOwnJID reports whether jid is this account's own user (ignoring the
// device suffix), for "you" labels and the admin flag.
func isOwnJID(jid, own string) bool {
	if jid == "" || own == "" {
		return false
	}
	return bareJIDUser(jid) == bareJIDUser(own)
}

// newFlatEntry is a borderless GtkEntry to sit inside a pill or box.
func newFlatEntry(placeholder string) *gtk.Entry {
	e := gtk.NewEntry()
	e.SetHasFrame(false)
	e.SetPlaceholderText(placeholder)
	e.SetHExpand(true)
	e.AddCSSClass("chatot-flat-entry")
	return e
}

// newSearchPill is the mockup's 34px white search pill with a ⌕ glyph and
// an optional clear ✕ that appears once there is text.
func newSearchPill(placeholder string, onChange func(text string)) (gtk.Widgetter, *gtk.Entry) {
	pill := gtk.NewBox(gtk.OrientationHorizontal, 8)
	pill.AddCSSClass("chatot-search-pill")
	glyph := gtk.NewLabel("⌕")
	glyph.AddCSSClass("chatot-search-pill-glyph")
	pill.Append(glyph)
	entry := newFlatEntry(placeholder)
	pill.Append(entry)
	clear := gtk.NewButtonWithLabel("✕")
	clear.AddCSSClass("flat")
	clear.RemoveCSSClass("text-button")
	clear.AddCSSClass("chatot-search-pill-clear")
	clear.SetVAlign(gtk.AlignCenter)
	clear.SetVisible(false)
	clear.SetTooltipText("Clear search")
	clear.ConnectClicked(func() { entry.SetText("") })
	pill.Append(clear)
	entry.ConnectChanged(func() {
		text := entry.Text()
		clear.SetVisible(text != "")
		if onChange != nil {
			onChange(text)
		}
	})
	return pill, entry
}

// chatByJIDOrNil is chatByJID for callers that only need the name.
func chatNameOr(c client.Client, jid, fallback string) string {
	if ch := chatByJID(c, jid); ch.Name != "" {
		return ch.Name
	}
	return fallback
}

// pickImageFile opens the image chooser and hands back the chosen path.
func pickImageFile(parent *gtk.Window, onPicked func(path string)) {
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Choose a photo")
	filter := gtk.NewFileFilter()
	filter.SetName("Images")
	filter.AddMIMEType("image/*")
	filters := gio.NewListStore(glib.TypeObject)
	filters.Append(filter.Object)
	dialog.SetFilters(filters)
	dialog.Open(context.Background(), parent, func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil || file == nil {
			return
		}
		if path := file.Path(); path != "" {
			onPicked(path)
		}
	})
}

// baseName is the file name part of path.
func baseName(path string) string { return filepath.Base(path) }

// mimeForPath guesses a MIME type from the extension, "image/jpeg" if none.
func mimeForPath(path string) string {
	if mt := mime.TypeByExtension(filepath.Ext(path)); mt != "" {
		return mt
	}
	return "image/jpeg"
}

// NewPlainHeader is the content strip the non-chat tabs show: the same
// draggable 46px band as the conversation header, carrying only the window
// controls.
func NewPlainHeader() gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-conv-headerrow")
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	row.Append(spacer)
	controls := gtk.NewWindowControls(gtk.PackEnd)
	controls.SetVAlign(gtk.AlignCenter)
	row.Append(controls)
	h := gtk.NewWindowHandle()
	h.SetChild(row)
	h.AddCSSClass("chatot-conv-header")
	return h
}

// displayLink is a link as the mockup prints it: without its scheme.
func displayLink(link string) string {
	link = strings.TrimPrefix(link, "https://")
	return strings.TrimPrefix(link, "http://")
}

// newIconValueRow is newIconRow returning the value label too, for a row
// whose value arrives later.
func newIconValueRow(icon, label, value string, onClick func()) (gtk.Widgetter, *gtk.Label) {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	glyph := gtk.NewLabel(icon)
	glyph.AddCSSClass("chatot-menu-icon")
	glyph.SetSizeRequest(16, -1)
	glyph.SetVAlign(gtk.AlignCenter)
	row.Append(glyph)
	text := gtk.NewLabel(label)
	text.SetXAlign(0)
	text.SetHExpand(true)
	text.AddCSSClass("chatot-card-label")
	row.Append(text)
	val := gtk.NewLabel(value)
	val.AddCSSClass("chatot-card-value")
	val.SetVAlign(gtk.AlignCenter)
	row.Append(val)
	btn := gtk.NewButton()
	btn.SetChild(row)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-card-btnrow")
	if onClick != nil {
		btn.ConnectClicked(onClick)
	}
	return btn, val
}
