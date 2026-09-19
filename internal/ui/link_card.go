package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
	"chatot/internal/linkpreview"
)

// linkView is a text bubble's link card: WhatsApp's layout of the page's
// picture over its title, a couple of lines of description and the site,
// above the message's own text.
type linkView struct {
	URL         string
	Title       string
	Description string
	Host        string
	Thumbnail   []byte
}

func linkVM(lp *client.LinkPreview) *linkView {
	if lp == nil || (lp.Title == "" && len(lp.Thumbnail) == 0) {
		return nil
	}
	return &linkView{URL: lp.URL, Title: lp.Title, Description: lp.Description, Thumbnail: lp.Thumbnail, Host: linkpreview.URLHost(lp.URL)}
}

// linkCardW is the card's width: the downloaded-video tile's, so link and
// media bubbles line up. linkCardImgH is the picture's height; the sender's
// thumbnail is cropped to cover it, as WhatsApp does.
const (
	linkCardW    = videoTileW
	linkCardImgH = 150
)

// buildLinkCard is the card itself. Clicking anywhere on it opens the link.
func buildLinkCard(v linkView) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("chatot-link-card")
	card.SetSizeRequest(linkCardW, -1)
	card.SetOverflow(gtk.OverflowHidden)
	// The card is a tile of one width, like the video tile it lines up
	// with: pinned at both ends (the size request is its minimum, halign
	// keeps a wider bubble from stretching it) so a long title, a long
	// description or a long URL beside it can never widen the picture.
	card.SetHAlign(gtk.AlignStart)

	if len(v.Thumbnail) > 0 {
		if pixbuf, err := pixbufFromBytes(v.Thumbnail); err == nil {
			card.Append(coverThumb(pixbuf, linkCardW, linkCardImgH, 0, false))
		}
	}

	// The text sits in a clamp at the card's width: a label's natural
	// width is measured in "characters", which lands wherever the font
	// puts it, and the title's and the description's landed past the
	// card's own 280. That made the card's natural width larger than its
	// minimum, and a card that is not one fixed width leaves its bubble
	// negotiating one — which is how a marketplace listing ended up in a
	// bubble half the pane wide.
	body := gtk.NewBox(gtk.OrientationVertical, 2)
	body.AddCSSClass("chatot-link-card-body")
	if v.Title != "" {
		title := gtk.NewLabel(v.Title)
		title.AddCSSClass("chatot-link-title")
		title.SetXAlign(0)
		title.SetWrap(true)
		title.SetWrapMode(pango.WrapWordChar)
		title.SetLines(2)
		title.SetEllipsize(pango.EllipsizeEnd)
		title.SetMaxWidthChars(36)
		body.Append(title)
	}
	if v.Description != "" {
		desc := gtk.NewLabel(v.Description)
		desc.AddCSSClass("chatot-link-desc")
		desc.SetXAlign(0)
		desc.SetWrap(true)
		desc.SetWrapMode(pango.WrapWordChar)
		desc.SetLines(3)
		desc.SetEllipsize(pango.EllipsizeEnd)
		desc.SetMaxWidthChars(40)
		body.Append(desc)
	}
	if v.Host != "" {
		host := gtk.NewLabel("🔗 " + v.Host)
		host.AddCSSClass("chatot-link-host")
		host.SetXAlign(0)
		host.SetEllipsize(pango.EllipsizeEnd)
		host.SetMaxWidthChars(40)
		host.SetMarginTop(2)
		body.Append(host)
	}
	bodyClamp := adw.NewClamp()
	bodyClamp.SetUnit(adw.LengthUnitPx)
	bodyClamp.SetMaximumSize(linkCardW)
	bodyClamp.SetTighteningThreshold(linkCardW)
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
