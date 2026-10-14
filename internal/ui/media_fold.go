package ui

import (
	"context"
	"strconv"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// CollapseMedia mirrors settings.CollapseMedia: photos, clips and albums
// fold to one line in the thread until clicked, and a link preview takes
// WhatsApp's compact card, a small picture beside the title.
var CollapseMedia = false

// foldThumbSide is the folded line's thumbnail; linkThumbSide the compact
// link card's picture.
const (
	foldThumbSide = 40
	linkThumbSide = 64
)

// foldsMedia reports whether mv folds under CollapseMedia: the pictures
// and clips that take a bubble's height. Stickers are small already, and
// voice notes and documents are one line anyway.
func foldsMedia(mv mediaView) bool {
	return CollapseMedia && !mv.ViewOnce && (mv.Kind == "image" || mv.Kind == "video")
}

// mediaFoldTooltip says what clicking the folded line will do.
func mediaFoldTooltip(open bool) string {
	if open {
		return "Collapse"
	}
	return "Show"
}

// buildMediaFold is a folded attachment: one line naming it, with a small
// thumbnail, that unfolds body underneath and folds it away again. The
// body is built on the first unfold. onToggle hears each change, so a
// rebuilt row comes back the way the reader left it.
func buildMediaFold(thumb gtk.Widgetter, title, meta string, open bool, body func() gtk.Widgetter, onToggle func(open bool)) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 4)
	box.AddCSSClass("chatot-media-fold")

	head := gtk.NewButton()
	head.AddCSSClass("flat")
	head.AddCSSClass("chatot-media-fold-head")
	head.SetFocusOnClick(false)
	row := gtk.NewBox(gtk.OrientationHorizontal, 9)
	if thumb != nil {
		row.Append(thumb)
	}
	col := gtk.NewBox(gtk.OrientationVertical, 1)
	col.SetVAlign(gtk.AlignCenter)
	col.SetHExpand(true)
	titleLbl := gtk.NewLabel(title)
	titleLbl.SetXAlign(0)
	titleLbl.SetEllipsize(pango.EllipsizeEnd)
	titleLbl.AddCSSClass("chatot-media-row-title")
	col.Append(titleLbl)
	if meta != "" {
		metaLbl := gtk.NewLabel(meta)
		metaLbl.SetXAlign(0)
		metaLbl.AddCSSClass("chatot-media-row-sub")
		col.Append(metaLbl)
	}
	row.Append(col)
	chev := newChevronGlyph(14)
	chev.AddCSSClass("chatot-media-fold-chev")
	row.Append(chev)
	head.SetChild(row)
	box.Append(head)

	var built gtk.Widgetter
	apply := func() {
		if open && built == nil {
			built = body()
			box.Append(built)
		}
		if built != nil {
			gtk.BaseWidget(built).SetVisible(open)
		}
		// Unfolded, the attachment itself is right below: the line
		// keeps only its words and the chevron that folds it again.
		if thumb != nil {
			gtk.BaseWidget(thumb).SetVisible(!open)
		}
		if open {
			head.AddCSSClass("chatot-media-fold-open")
		} else {
			head.RemoveCSSClass("chatot-media-fold-open")
		}
		head.SetTooltipText(mediaFoldTooltip(open))
	}
	apply()
	head.ConnectClicked(func() {
		open = !open
		apply()
		if onToggle != nil {
			onToggle(open)
		}
	})
	return box
}

// mediaFoldThumb is the folded line's square: the downloaded picture
// itself (decoded at the inline photo's size, so unfolding finds it
// memoised), else the sender's embedded preview, else the kind's glyph.
func mediaFoldThumb(mv mediaView, msg client.Message) gtk.Widgetter {
	frame := gtk.NewOverlay()
	frame.AddCSSClass("chatot-media-fold-thumb")
	frame.SetOverflow(gtk.OverflowHidden)
	frame.SetVAlign(gtk.AlignCenter)
	base := gtk.NewBox(gtk.OrientationVertical, 0)
	base.AddCSSClass("chatot-media-hatch")
	base.SetSizeRequest(foldThumbSide, foldThumbSide)
	frame.SetChild(base)

	if mv.HasLocal && mv.Kind == "image" {
		p := gtk.NewPicture()
		p.SetCanShrink(true)
		p.SetContentFit(gtk.ContentFitCover)
		frame.AddOverlay(p)
		loadPictureAsync(mv.LocalPath, inlinePhotoSide*2, func(t gdk.Paintabler) { p.SetPaintable(t) })
		return frame
	}
	// A downloaded clip's view drops the embedded preview (the tile grabs
	// a frame from the file instead); the message still carries it.
	if msg.Attachment != nil && len(msg.Attachment.Thumbnail) > 0 {
		if pb, err := pixbufFromBytes(msg.Attachment.Thumbnail); err == nil {
			frame.AddOverlay(coverThumb(pb, foldThumbSide, foldThumbSide, 0, false))
			return frame
		}
	}
	glyph := gtk.NewLabel(mediaIcon(mv.Kind))
	glyph.SetHAlign(gtk.AlignCenter)
	glyph.SetVAlign(gtk.AlignCenter)
	frame.AddOverlay(glyph)
	return frame
}

// mediaFoldTitle is the folded line's first line: "Photo", "Video · 0:12",
// "GIF".
func mediaFoldTitle(mv mediaView) string {
	switch {
	case mv.IsGIF:
		return "GIF"
	case mv.Kind == "video":
		return joinMeta(mediaNoun(mv.Kind), humanDuration(mv.DurationSecs))
	}
	return mediaNoun(mv.Kind)
}

// albumFoldTitle names a folded album by what it holds: "4 photos",
// "2 videos", "5 photos and videos".
func albumFoldTitle(msgs []client.Message) string {
	photos, videos := 0, 0
	for _, m := range msgs {
		if m.Attachment != nil && m.Attachment.Kind == "video" {
			videos++
		} else {
			photos++
		}
	}
	n := strconv.Itoa(len(msgs))
	switch {
	case videos == 0:
		return n + " photos"
	case photos == 0:
		return n + " videos"
	}
	return n + " photos and videos"
}

// prefetchFolded downloads a folded attachment the auto-download
// preference wants, as the tile it stands in for would have, so the
// viewer and an unfold find the file. An open fold has the tile itself,
// which downloads on its own. mv learns the path, so a body built
// after the download lands shows the picture rather than a tile.
func prefetchFolded(mv *mediaView, msg client.Message, c client.Client, onLocal func(path string)) {
	if mv.HasLocal || c == nil {
		return
	}
	maybeAutoDownload(func() {
		go func() {
			path, err := c.DownloadMedia(context.Background(), msg.ID)
			if err != nil {
				return
			}
			glib.IdleAdd(func() {
				mv.HasLocal = true
				mv.LocalPath = path
				if onLocal != nil {
					onLocal(path)
				}
			})
		}()
	}, mv.Kind, msg.TS)
}

// buildFoldedMediaContent is buildMediaContent folded to one line.
func buildFoldedMediaContent(msg client.Message, mv mediaView, h bubbleHooks, open bool) gtk.Widgetter {
	state := mv
	if !open {
		prefetchFolded(&state, msg, h.c, mv.onLocal)
	}
	return buildMediaFold(mediaFoldThumb(mv, msg), mediaFoldTitle(mv), humanSize(mv.Size), open,
		func() gtk.Widgetter { return buildMediaContent(msg, state, h.c, h.mediaOpener(msg)) },
		h.mediaFoldRecorder(msg.ID))
}

// buildFoldedAlbumContent is an album folded to one line, its first
// picture as the thumbnail.
func buildFoldedAlbumContent(alb *albumView, h bubbleHooks, open bool) gtk.Widgetter {
	for i, m := range alb.Msgs {
		if !open && i < len(alb.Tiles) {
			prefetchFolded(&alb.Tiles[i], m, h.c, h.localRecorder(m.ID))
		}
	}
	head := alb.Msgs[0]
	return buildMediaFold(mediaFoldThumb(alb.Tiles[0], head), albumFoldTitle(alb.Msgs), "", open,
		func() gtk.Widgetter { return buildAlbumContent(alb, h) },
		h.mediaFoldRecorder(head.ID))
}

// mediaFoldRecorder is the fold's report of a change to msgID; nil when
// nobody listens.
func (h bubbleHooks) mediaFoldRecorder(msgID string) func(open bool) {
	if h.onUnfoldMedia == nil {
		return nil
	}
	return func(open bool) { h.onUnfoldMedia(msgID, open) }
}

// mediaUnfolded reports whether the reader unfolded msgID's attachment.
func (h bubbleHooks) mediaUnfolded(msgID string) bool {
	return h.mediaUnfoldedOf != nil && h.mediaUnfoldedOf(msgID)
}

// mediaUnfolded reports whether msgID's folded attachment is open.
func (cv *ConversationView) mediaUnfolded(msgID string) bool {
	return cv.unfoldedMedia[msgID]
}

// setMediaUnfolded remembers the reader's fold of msgID's attachment, so a
// rebuilt row (a reaction, a recycled row) shows it the same way.
func (cv *ConversationView) setMediaUnfolded(msgID string, open bool) {
	if cv.unfoldedMedia == nil {
		cv.unfoldedMedia = map[string]bool{}
	}
	if open {
		cv.unfoldedMedia[msgID] = true
		return
	}
	delete(cv.unfoldedMedia, msgID)
}

// RefillRows re-renders every row on screen, for a preference that changes
// how bubbles are built.
func (cv *ConversationView) RefillRows() {
	pos := make(map[string]int, len(cv.msgs))
	for i, m := range cv.msgs {
		pos[m.ID] = i
	}
	for _, r := range cv.rows {
		if i, ok := pos[r.msgID]; ok && r.msgID != "" {
			cv.fillRow(r.wrapper, i)
		}
	}
}

// buildCompactLinkCard is the link card under CollapseMedia: the page's
// picture as a small square beside its title, a line of description and
// the site, as WhatsApp lays out a link with a small preview.
func buildCompactLinkCard(v linkView) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationHorizontal, 0)
	card.AddCSSClass("chatot-link-card")
	card.AddCSSClass("chatot-link-card-compact")
	card.SetSizeRequest(linkCardW, -1)
	card.SetOverflow(gtk.OverflowHidden)
	card.SetHAlign(gtk.AlignStart)

	if len(v.Thumbnail) > 0 {
		if pixbuf, err := pixbufFromBytes(v.Thumbnail); err == nil {
			thumb := coverThumb(pixbuf, linkThumbSide, linkThumbSide, 0, false)
			thumb.SetVAlign(gtk.AlignStart)
			card.Append(thumb)
		}
	}

	body := gtk.NewBox(gtk.OrientationVertical, 1)
	body.AddCSSClass("chatot-link-card-body")
	body.SetVAlign(gtk.AlignCenter)
	if v.Title != "" {
		title := gtk.NewLabel(v.Title)
		title.AddCSSClass("chatot-link-title")
		title.SetXAlign(0)
		title.SetWrap(true)
		title.SetWrapMode(pango.WrapWordChar)
		title.SetLines(2)
		title.SetEllipsize(pango.EllipsizeEnd)
		title.SetMaxWidthChars(26)
		body.Append(title)
	}
	if v.Description != "" {
		desc := gtk.NewLabel(v.Description)
		desc.AddCSSClass("chatot-link-desc")
		desc.SetXAlign(0)
		desc.SetSingleLineMode(true)
		desc.SetEllipsize(pango.EllipsizeEnd)
		desc.SetMaxWidthChars(30)
		body.Append(desc)
	}
	if v.Host != "" {
		host := gtk.NewLabel("🔗 " + v.Host)
		host.AddCSSClass("chatot-link-host")
		host.SetXAlign(0)
		host.SetEllipsize(pango.EllipsizeEnd)
		host.SetMaxWidthChars(30)
		body.Append(host)
	}
	// The same clamp as the full card: the text never widens the card
	// past its one width.
	bodyClamp := adw.NewClamp()
	bodyClamp.SetUnit(adw.LengthUnitPx)
	bodyClamp.SetMaximumSize(linkCardW - linkThumbSide)
	bodyClamp.SetTighteningThreshold(linkCardW - linkThumbSide)
	bodyClamp.SetHExpand(true)
	bodyClamp.SetChild(body)
	card.Append(bodyClamp)

	if href := linkHref(v.URL); href != "" {
		click := gtk.NewGestureClick()
		click.ConnectReleased(func(int, float64, float64) { openURI(href) })
		card.AddController(click)
		card.SetCursorFromName("pointer")
		card.SetTooltipText(href)
	}
	return card
}
