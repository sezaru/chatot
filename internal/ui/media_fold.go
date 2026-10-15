package ui

import (
	"context"
	"log"
	"strconv"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// A photo, clip, album or link preview can be collapsed in its bubble: a
// picture folds to one line (a small square of it, what it is, a chevron),
// a link preview to WhatsApp's compact card. Every such bubble carries the
// control, a chevron in the picture's top corner on hover, and the message
// menu has Collapse/Expand too. How the reader left each one is saved on
// the message (client.Message.Fold); one they never touched follows
// CollapseMedia.

// CollapseMedia mirrors settings.CollapseMedia: whether a bubble the
// reader has not folded or unfolded starts collapsed.
var CollapseMedia = false

// foldThumbSide is the folded line's thumbnail; linkThumbSide the compact
// link card's picture.
const (
	foldThumbSide = 40
	linkThumbSide = 64
)

// foldableMedia reports whether mv can fold: the pictures and clips that
// take a bubble's height. Stickers are small already, and voice notes and
// documents are one line anyway.
func foldableMedia(mv mediaView) bool {
	return !mv.ViewOnce && (mv.Kind == "image" || mv.Kind == "video")
}

// foldableMessage reports whether msg's bubble has something to fold: a
// picture or clip, or a link preview.
func foldableMessage(msg client.Message) bool {
	if a := msg.Attachment; a != nil {
		return !a.ViewOnce && (a.Kind == "image" || a.Kind == "video")
	}
	return linkVM(msg.LinkPreview) != nil
}

// foldMenuLabel and foldMenuIcon are the fold control's words and the
// message menu's row, for a bubble that is collapsed now or not.
func foldMenuLabel(folded bool) string {
	if folded {
		return "Expand"
	}
	return "Collapse"
}

func foldMenuIcon(folded bool) string {
	if folded {
		return "⤢"
	}
	return "⤡"
}

// foldFor is msg's fold: what the reader chose for it, else CollapseMedia.
func foldFor(fold client.MediaFold) bool {
	switch fold {
	case client.FoldCollapsed:
		return true
	case client.FoldExpanded:
		return false
	}
	return CollapseMedia
}

// messageFolded is whether msg's picture or link preview is collapsed.
// The view's own copy of the fold wins: it is what the reader just did,
// while msg may be a page read before the store heard of it.
func (h bubbleHooks) messageFolded(msg client.Message) bool {
	if h.mediaFoldOf != nil {
		return foldFor(h.mediaFoldOf(msg))
	}
	return foldFor(msg.Fold)
}

// foldToggle is the fold control's action for msg, nil where the bubble
// cannot fold (no conversation behind it, as in tests).
func (h bubbleHooks) foldToggle(msg client.Message, folded bool) func() {
	if h.foldMediaNow == nil {
		return nil
	}
	id := msg.ID
	return func() { h.foldMediaNow(id, !folded) }
}

// withFoldButton puts the fold control over content's top-right corner,
// whoever sent it. It shows while the pointer is over the content.
func withFoldButton(content gtk.Widgetter, folded bool, toggle func()) gtk.Widgetter {
	if toggle == nil {
		return content
	}
	wrap := gtk.NewOverlay()
	wrap.AddCSSClass("chatot-fold-wrap")
	wrap.SetHAlign(gtk.AlignStart)
	wrap.SetChild(content)
	btn := gtk.NewButton()
	btn.AddCSSClass("chatot-fold-btn")
	if !folded {
		btn.AddCSSClass("chatot-fold-btn-open")
	}
	btn.SetFocusOnClick(false)
	btn.SetTooltipText(foldMenuLabel(folded))
	btn.SetVAlign(gtk.AlignStart)
	btn.SetHAlign(gtk.AlignEnd)
	btn.SetMarginTop(6)
	btn.SetMarginStart(6)
	btn.SetMarginEnd(6)
	btn.SetChild(newChevronGlyph(14))
	btn.ConnectClicked(toggle)
	wrap.AddOverlay(btn)
	return wrap
}

// buildCollapsedRow is a collapsed picture: one line naming it beside a
// small square of it, with a chevron; a click anywhere expands it.
func buildCollapsedRow(thumb gtk.Widgetter, title, meta string, expand func()) gtk.Widgetter {
	head := gtk.NewButton()
	head.AddCSSClass("flat")
	head.AddCSSClass("chatot-media-fold-head")
	head.SetFocusOnClick(false)
	head.SetTooltipText(foldMenuLabel(true))
	row := gtk.NewBox(gtk.OrientationHorizontal, 9)
	row.Append(thumb)
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
	head.AddCSSClass("chatot-media-fold")
	head.ConnectClicked(expand)
	return head
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
// viewer and an expand find the file. mv learns the path.
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

// buildFoldedMediaContent is a collapsed photo or clip.
func buildFoldedMediaContent(msg client.Message, mv mediaView, h bubbleHooks) gtk.Widgetter {
	state := mv
	prefetchFolded(&state, msg, h.c, mv.onLocal)
	return buildCollapsedRow(mediaFoldThumb(mv, msg), mediaFoldTitle(mv), humanSize(mv.Size), h.foldToggle(msg, true))
}

// buildFoldedAlbumContent is a collapsed album, its first picture as the
// thumbnail.
func buildFoldedAlbumContent(alb *albumView, h bubbleHooks) gtk.Widgetter {
	for i, m := range alb.Msgs {
		if i < len(alb.Tiles) {
			prefetchFolded(&alb.Tiles[i], m, h.c, h.localRecorder(m.ID))
		}
	}
	head := alb.Msgs[0]
	return buildCollapsedRow(mediaFoldThumb(alb.Tiles[0], head), albumFoldTitle(alb.Msgs), "", h.foldToggle(head, true))
}

// mediaFoldOf is msgID's fold as this view knows it: the reader's latest
// choice, else what the message was read with.
func (cv *ConversationView) mediaFoldOf(msg client.Message) client.MediaFold {
	if fold, ok := cv.mediaFolds[msg.ID]; ok {
		return fold
	}
	return msg.Fold
}

// foldMedia collapses or expands msgID's picture or link preview: the row
// is rebuilt at once, and the choice saved on the message so the chat
// opens that way next time.
func (cv *ConversationView) foldMedia(msgID string, folded bool) {
	fold := client.FoldExpanded
	if folded {
		fold = client.FoldCollapsed
	}
	if cv.mediaFolds == nil {
		cv.mediaFolds = map[string]client.MediaFold{}
	}
	cv.mediaFolds[msgID] = fold
	cv.refillRow(cv.positionOf(msgID))
	jid := cv.jid
	go func() {
		if err := cv.c.SetMessageFold(jid, msgID, fold); err != nil {
			log.Printf("chatot: save fold of %s: %v", msgID, err)
		}
	}()
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
