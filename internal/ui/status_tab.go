package ui

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// The Status tab: the sidebar's feed of contacts' updates grouped by poster
// (mockup: "My status", RECENT UPDATES, VIEWED UPDATES, each row a segmented
// ring), and the dark full-pane viewer with its progress segments, reply
// row and quick reactions.

// statusPoster is one contact's (or our own) run of status updates, newest
// first, as one sidebar row.
type statusPoster struct {
	JID    string
	Name   string
	Items  []client.Message
	Viewed bool
	Mine   bool
	// Muted files the poster under Muted updates (WhatsApp's status mute).
	Muted bool
}

// statusFeed is the sidebar's grouping of the raw status messages.
type statusFeed struct {
	Mine   *statusPoster
	Recent []statusPoster
	Viewed []statusPoster
	Muted  []statusPoster
}

// groupStatuses folds the flat status list into per-poster rows: our own
// updates aside, then contacts split into unviewed, viewed and muted, each
// newest poster first. A poster is viewed once every update carries the
// read status the client records on view. Deleted updates are dropped.
func groupStatuses(msgs []client.Message, own string, names map[string]string, muted map[string]bool) statusFeed {
	byJID := map[string]*statusPoster{}
	var order []string
	for _, m := range msgs {
		if m.Deleted {
			continue
		}
		jid := m.FromJID
		mine := m.FromMe || isOwnJID(jid, own)
		if mine {
			jid = "me"
		}
		p, ok := byJID[jid]
		if !ok {
			p = &statusPoster{JID: jid, Mine: mine}
			if mine {
				p.Name = "My status"
			} else {
				p.Name = posterName(jid, names)
				p.Muted = muted[jid]
			}
			byJID[jid] = p
			order = append(order, jid)
		}
		p.Items = append(p.Items, m)
	}
	var feed statusFeed
	for _, jid := range order {
		p := byJID[jid]
		sort.SliceStable(p.Items, func(i, j int) bool { return p.Items[i].TS > p.Items[j].TS })
		p.Viewed = !p.Mine && allViewed(p.Items)
		switch {
		case p.Mine:
			feed.Mine = p
		case p.Muted:
			feed.Muted = append(feed.Muted, *p)
		case p.Viewed:
			feed.Viewed = append(feed.Viewed, *p)
		default:
			feed.Recent = append(feed.Recent, *p)
		}
	}
	latest := func(p statusPoster) int64 { return p.Items[0].TS }
	sort.SliceStable(feed.Recent, func(i, j int) bool { return latest(feed.Recent[i]) > latest(feed.Recent[j]) })
	sort.SliceStable(feed.Viewed, func(i, j int) bool { return latest(feed.Viewed[i]) > latest(feed.Viewed[j]) })
	sort.SliceStable(feed.Muted, func(i, j int) bool { return latest(feed.Muted[i]) > latest(feed.Muted[j]) })
	return feed
}

// poster finds a poster in any section ("me" for our own), nil if absent.
func (f *statusFeed) poster(jid string) *statusPoster {
	if jid == "me" {
		return f.Mine
	}
	for _, section := range [][]statusPoster{f.Recent, f.Viewed, f.Muted} {
		for i := range section {
			if section[i].JID == jid {
				return &section[i]
			}
		}
	}
	return nil
}

// allViewed is true when every update has been marked read (viewed).
func allViewed(items []client.Message) bool {
	for _, m := range items {
		if m.Status < client.MessageStatusRead {
			return false
		}
	}
	return len(items) > 0
}

// statusRowSub is a poster row's second line: "2 updates · 23:02".
func statusRowSub(p statusPoster, now time.Time) string {
	return pluralCount(len(p.Items), "update", "updates") + " · " + formatChatTime(p.Items[0].TS, now)
}

// myStatusSub is the "My status" row's second line.
func myStatusSub(mine *statusPoster, now time.Time) string {
	if mine == nil || len(mine.Items) == 0 {
		return "Add to my status — it disappears after 24h"
	}
	return "Today, " + formatChatTime(mine.Items[0].TS, now)
}

// statusViewerMeta is the viewer's monospace line: "23:02 · 1 of 2".
func statusViewerMeta(item client.Message, index, total int, now time.Time) string {
	return fmt.Sprintf("%s · %d of %d", formatChatTime(item.TS, now), index+1, total)
}

// Ring geometry, from the mockup: a 46px ring with a 9° gap between
// segments, a 40px sidebar-coloured disc, then the 36px avatar.
const (
	statusRingSize  = 46
	statusRingGap   = 9.0
	statusRingWidth = 3.0
)

// newStatusRing draws a poster's segmented ring around their avatar. n is
// the number of updates (segments); viewed dims the ring. plus adds the
// "My status" ＋ badge for the not-yet-posted state.
func newStatusRing(c client.Client, cache *avatarCache, jid, initial string, n int, viewed, plus bool) gtk.Widgetter {
	ring := gtk.NewDrawingArea()
	ring.SetSizeRequest(statusRingSize, statusRingSize)
	ring.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawStatusRing(cr, float64(w), float64(h), n, viewed, isDark())
	})

	overlay := gtk.NewOverlay()
	overlay.SetChild(ring)
	avatar := buildAvatar(c, cache, jid, initial, 36)
	avatar.SetHAlign(gtk.AlignCenter)
	avatar.SetVAlign(gtk.AlignCenter)
	overlay.AddOverlay(avatar)
	if plus {
		badge := gtk.NewLabel("＋")
		badge.AddCSSClass("chatot-status-plus")
		badge.SetSizeRequest(18, 18)
		badge.SetHAlign(gtk.AlignEnd)
		badge.SetVAlign(gtk.AlignEnd)
		overlay.AddOverlay(badge)
	}
	return overlay
}

// drawStatusRing paints n arcs (a full ring when n <= 1) in accent green,
// or in the mockup's 20% grey once viewed, over the sidebar-coloured disc.
// dark swaps the grey and the disc for the dark sheet's tokens (see
// style-dark.css), which cairo can't read.
func drawStatusRing(cr *cairo.Context, w, h float64, n int, viewed, dark bool) {
	cx, cy := w/2, h/2
	r := math.Min(w, h)/2 - statusRingWidth/2
	switch {
	case viewed && dark:
		cr.SetSourceRGBA(1, 1, 1, 0.28)
	case viewed:
		cr.SetSourceRGBA(0, 0, 0, 0.2)
	default:
		cr.SetSourceRGB(0x1b/255.0, 0x8c/255.0, 0x72/255.0)
	}
	cr.SetLineWidth(statusRingWidth)
	cr.SetLineCap(cairo.LineCapButt)
	if n <= 1 {
		cr.Arc(cx, cy, r, 0, 2*math.Pi)
		cr.Stroke()
	} else {
		step := 2 * math.Pi / float64(n)
		gap := statusRingGap * math.Pi / 180
		start := -math.Pi / 2
		for i := 0; i < n; i++ {
			a := start + float64(i)*step
			cr.Arc(cx, cy, r, a, a+step-gap)
			cr.Stroke()
		}
	}
	// The gap disc between ring and avatar, in the sidebar's grey.
	if dark {
		cr.SetSourceRGB(0x26/255.0, 0x26/255.0, 0x26/255.0)
	} else {
		cr.SetSourceRGB(0xeb/255.0, 0xeb/255.0, 0xeb/255.0)
	}
	cr.Arc(cx, cy, 20, 0, 2*math.Pi)
	cr.Fill()
}

// refreshStatus rebuilds the Status tab's sidebar list and its tab badge.
func (cl *ChatList) refreshStatus() {
	feed := cl.loadStatusFeed()
	cl.statusFeed = feed
	cl.tabBar.SetBadge("status", len(feed.Recent))
	if cl.tab != "status" {
		return
	}
	cl.statusList.RemoveAll()
	cl.statusActions = nil
	now := time.Now()

	cl.statusList.Append(cl.buildMyStatusRow(feed.Mine, now))
	if feed.Mine != nil && len(feed.Mine.Items) > 0 {
		cl.statusActions = append(cl.statusActions, func() { cl.openStatus("me") })
	} else {
		cl.statusActions = append(cl.statusActions, cl.showTextStatusDialog)
	}
	section := func(caption string, posters []statusPoster) {
		if len(posters) == 0 {
			return
		}
		cl.statusList.Append(captionRow(caption))
		cl.statusActions = append(cl.statusActions, nil)
		for _, p := range posters {
			cl.statusList.Append(cl.buildStatusRow(p, now))
			jid := p.JID
			cl.statusActions = append(cl.statusActions, func() { cl.openStatus(jid) })
		}
	}
	section("Recent updates", feed.Recent)
	section("Viewed updates", feed.Viewed)
	section("Muted updates", feed.Muted)
	if feed.Mine == nil && len(feed.Recent) == 0 && len(feed.Viewed) == 0 && len(feed.Muted) == 0 {
		empty := gtk.NewLabel("No status updates yet")
		empty.AddCSSClass("chatot-search-empty")
		cl.statusList.Append(inertRow(empty))
	}
}

// loadStatusFeed reads and groups the current statuses.
func (cl *ChatList) loadStatusFeed() statusFeed {
	msgs, err := cl.c.Statuses(200)
	if err != nil {
		log.Printf("chatot: load statuses: %v", err)
	}
	muted := map[string]bool{}
	list, err := cl.c.MutedStatusPosters()
	if err != nil {
		log.Printf("chatot: muted status posters: %v", err)
	}
	for _, jid := range list {
		muted[jid] = true
	}
	return groupStatuses(msgs, cl.c.OwnJID(), chatNames(cl.c), muted)
}

// captionRow wraps a section caption as a non-selectable list row.
func captionRow(text string) *gtk.ListBoxRow {
	return inertRow(newSectionCaption(text))
}

// inertRow wraps w as a list row that neither selects nor activates.
func inertRow(w gtk.Widgetter) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetChild(w)
	row.SetActivatable(false)
	row.SetSelectable(false)
	row.AddCSSClass("chatot-inert-row")
	return row
}

// buildMyStatusRow is the top "My status" row: the ＋-badged ring before
// anything is posted, then the ring plus a ⋯ menu once something is.
func (cl *ChatList) buildMyStatusRow(mine *statusPoster, now time.Time) *gtk.ListBoxRow {
	posted := mine != nil && len(mine.Items) > 0
	n := 0
	if posted {
		n = len(mine.Items)
	}
	box := gtk.NewBox(gtk.OrientationHorizontal, 11)
	box.AddCSSClass("chatot-status-row")
	box.Append(newStatusRing(cl.c, cl.avatarCache, cl.c.OwnJID(), cl.ownInitial(), n, !posted, !posted))

	text := gtk.NewBox(gtk.OrientationVertical, 2)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel("My status")
	name.SetXAlign(0)
	name.AddCSSClass("chatot-chat-name")
	text.Append(name)
	sub := gtk.NewLabel(myStatusSub(mine, now))
	sub.SetXAlign(0)
	sub.SetEllipsize(pango.EllipsizeEnd)
	sub.SetMaxWidthChars(1)
	sub.SetHExpand(true)
	sub.AddCSSClass("chatot-status-sub")
	text.Append(sub)
	box.Append(text)

	if posted {
		more := gtk.NewButtonWithLabel("⋯")
		more.AddCSSClass("flat")
		more.RemoveCSSClass("text-button")
		more.AddCSSClass("chatot-status-more")
		more.SetVAlign(gtk.AlignCenter)
		more.SetTooltipText("Status options")
		more.ConnectClicked(func() { popupMenuBelow(more, cl.myStatusMenu()) })
		box.Append(more)
	}

	row := gtk.NewListBoxRow()
	row.SetChild(box)
	row.SetSelectable(false)
	return row
}

// ownInitial is the avatar letter for our own account.
func (cl *ChatList) ownInitial() string {
	if cl.accountAvatar != nil && cl.accountAvatar.Text() != "" {
		return cl.accountAvatar.Text()
	}
	return "S"
}

// buildStatusRow is a contact's row: ring, bold-or-plain name, count line.
func (cl *ChatList) buildStatusRow(p statusPoster, now time.Time) *gtk.ListBoxRow {
	box := gtk.NewBox(gtk.OrientationHorizontal, 11)
	box.AddCSSClass("chatot-status-row")
	box.Append(newStatusRing(cl.c, cl.avatarCache, p.JID, initialFor(p.Name), len(p.Items), p.Viewed, false))

	text := gtk.NewBox(gtk.OrientationVertical, 2)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(p.Name)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.SetMaxWidthChars(1)
	name.SetHExpand(true)
	if p.Viewed {
		name.AddCSSClass("chatot-status-name-viewed")
	} else {
		name.AddCSSClass("chatot-chat-name")
	}
	text.Append(name)
	sub := gtk.NewLabel(statusRowSub(p, now))
	sub.SetXAlign(0)
	sub.AddCSSClass("chatot-status-sub")
	text.Append(sub)
	box.Append(text)

	poster := p
	attachRowMenu(box, func() []menuItem { return cl.statusRowMenu(poster) })

	row := gtk.NewListBoxRow()
	row.SetChild(box)
	row.SetSelectable(false)
	return row
}

// statusRowMenu is a contact row's right-click menu.
func (cl *ChatList) statusRowMenu(p statusPoster) []menuItem {
	return statusRowMenuItems(p.Name, p.Muted, statusRowMenuActions{
		View:  func() { cl.openStatus(p.JID) },
		Reply: func() { cl.replyPrivately(p.JID) },
		Mute:  func() { cl.setStatusMuted(p, !p.Muted) },
		Hide:  func() { cl.hideStatusFrom(p) },
	})
}

// setStatusMuted mutes (or unmutes) a poster's updates through the client
// and refiles their row.
func (cl *ChatList) setStatusMuted(p statusPoster, mute bool) {
	jid, name := p.JID, p.Name
	go func() {
		err := cl.c.MuteStatus(context.Background(), jid, mute)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: mute status %s: %v", jid, err)
				cl.toast("Couldn't change the mute")
				return
			}
			if cl.statusPane.current.JID == jid {
				cl.statusPane.current.Muted = mute
			}
			if mute {
				cl.toast(name + " moved to Muted updates")
			} else {
				cl.toast(name + "'s updates unmuted")
			}
			cl.refreshStatus()
		})
	}()
}

// hideStatusFrom takes p out of our status audience.
func (cl *ChatList) hideStatusFrom(p statusPoster) {
	jid, name := p.JID, p.Name
	go func() {
		err := cl.c.HideStatusFrom(context.Background(), jid)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: hide status from %s: %v", jid, err)
				cl.toast("Couldn't change who sees your status")
				return
			}
			cl.toast(name + " can no longer see your status")
		})
	}()
}

// markStatusViewed records p's unviewed updates as seen and, when read
// receipts are on, sends the poster the receipt WhatsApp counts as a
// view. The row refiles under Viewed once the client has noted it.
func (cl *ChatList) markStatusViewed(p statusPoster) {
	var ids []string
	for _, m := range p.Items {
		if m.Status < client.MessageStatusRead {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	jid := p.JID
	go func() {
		if err := cl.c.MarkStatusViewed(context.Background(), jid, ids, SendReadReceipts); err != nil {
			log.Printf("chatot: mark status viewed %s: %v", jid, err)
		}
		glib.IdleAdd(func() { cl.refreshStatus() })
	}()
}

// myStatusMenu is the "My status" row's ⋯ menu.
func (cl *ChatList) myStatusMenu() []menuItem {
	return myStatusMenuItems(myStatusMenuActions{
		Viewers: func() {
			if m := cl.statusFeed.Mine; m != nil {
				cl.showStatusViewers(m.Items)
			}
		},
		Privacy: func() { cl.showPreferencesPage("privacy") },
		Delete:  cl.deleteMyStatus,
	})
}

// replyPrivately opens the poster's chat in the Chats tab.
func (cl *ChatList) replyPrivately(jid string) {
	cl.selectTab("chats")
	if cl.onSelect != nil {
		cl.onSelect(jid)
	}
}

// deleteMyStatus revokes every update we have up.
func (cl *ChatList) deleteMyStatus() {
	mine := cl.statusFeed.Mine
	if mine == nil {
		return
	}
	items := mine.Items
	go func() {
		for _, m := range items {
			if err := cl.c.DeleteMessage(context.Background(), m.ChatJID, m.ID); err != nil {
				log.Printf("chatot: delete status %s: %v", m.ID, err)
			}
		}
		glib.IdleAdd(func() {
			cl.toast("Status deleted")
			if cl.statusPane.poster == "me" {
				cl.closeStatus()
			}
			cl.refreshStatus()
		})
	}()
}

// openStatus shows poster's updates in the viewer and marks them viewed.
func (cl *ChatList) openStatus(jid string) {
	p := cl.statusFeed.poster(jid)
	if p == nil || len(p.Items) == 0 {
		return
	}
	if !p.Mine {
		cl.markStatusViewed(*p)
	}
	cl.statusPane.Show(*p)
	cl.showPane("status")
	cl.refreshStatus()
}

// closeStatus leaves the viewer for the tab's empty pane.
func (cl *ChatList) closeStatus() {
	cl.statusPane.stop()
	// Nothing left to resume: a reply field losing focus after the close
	// must not restart the clock on a hidden pane.
	cl.statusPane.current.Items = nil
	cl.showPane("tabempty")
}

// showTextStatusDialog posts a text update.
func (cl *ChatList) showTextStatusDialog() {
	dialog := newCardDialog()
	dialog.SetTitle("Text status")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(400, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := gtk.NewBox(gtk.OrientationVertical, 10)
	body.AddCSSClass("chatot-dialog-body")
	body.Append(newDialogBodyText("Everyone who can see your status sees this for 24 hours."))
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("What's on your mind?")
	entry.AddCSSClass("chatot-dialog-entry")
	body.Append(entry)
	hint := gtk.NewLabel("")
	hint.SetXAlign(0)
	hint.AddCSSClass("chatot-dialog-hint")
	body.Append(hint)
	box.Append(body)

	footer := newDialogFooter()
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	footer.Append(spacer)
	footer.Append(newChipButton("Cancel", func() { dialog.Close() }))
	post := newPrimaryButton("Post", nil)
	post.SetSensitive(false)
	footer.Append(post)
	box.Append(footer)

	entry.ConnectChanged(func() { post.SetSensitive(strings.TrimSpace(entry.Text()) != "") })
	send := func() {
		text := strings.TrimSpace(entry.Text())
		if text == "" {
			return
		}
		post.SetSensitive(false)
		go func() {
			err := cl.c.PostStatus(context.Background(), text)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: post status: %v", err)
					hint.SetText("Couldn't post the status, try again")
					post.SetSensitive(true)
					return
				}
				dialog.Close()
				cl.toast("Status posted · visible for 24 hours")
				cl.refreshStatus()
			})
		}()
	}
	post.ConnectClicked(send)
	entry.ConnectActivate(send)

	dialog.SetChild(box)
	dialog.SetDefaultWidget(post)
	dialog.Present()
	entry.GrabFocus()
}

// postPhotoStatus picks an image and posts it to the status broadcast.
func (cl *ChatList) postPhotoStatus() {
	pickImageFile(cl.window, func(path string) {
		att := client.Attachment{Kind: "image", LocalPath: path, Filename: baseName(path), MimeType: mimeForPath(path)}
		go func() {
			_, err := cl.c.SendMedia(context.Background(), "status@broadcast", att, nil)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: post photo status: %v", err)
					cl.toast("Couldn't post the photo")
					return
				}
				cl.toast("Photo status posted · visible for 24 hours")
				cl.refreshStatus()
			})
		}()
	})
}

// --- the viewer pane ---

// StatusPane is the mockup's dark story viewer: progress segments, the
// poster row, the 4:5 card with its caption, and the reply row (or the
// "Viewed by" pill for our own status).
type StatusPane struct {
	*gtk.Box

	cl *ChatList

	segments *gtk.Box
	segs     []*gtk.DrawingArea
	avatar   *gtk.Box
	name     *gtk.Label
	meta     *gtk.Label
	menuBtn  *gtk.Button
	cardSlot *gtk.Box
	caption  *gtk.Label
	footer   *gtk.Stack
	reply    *gtk.Entry
	viewers  *gtk.Button

	poster   string
	current  statusPoster
	index    int
	progress float64
	tick     glib.SourceHandle

	// The clock can be held: explicitly (pause button, a click on the
	// card) or while a reply is being typed. elapsed survives the hold so
	// the segment resumes where it stopped.
	pauseBtn   *gtk.Button
	pauseIcon  *gtk.DrawingArea
	pausedChip *gtk.Label
	userPaused bool
	typing     bool
	elapsed    time.Duration
}

// statusAdvanceMS is how long one update stays up before the next.
const statusAdvanceMS = 5000

// The viewer's round buttons: the mockup's 28px header buttons and 34px
// quick reactions.
const (
	statusRoundBtnPx = 28
	statusQuickBtnPx = 34
)

func newStatusPane(cl *ChatList) *StatusPane {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-status-pane")
	root.SetVExpand(true)
	root.SetHExpand(true)
	p := &StatusPane{Box: root, cl: cl}

	top := gtk.NewBox(gtk.OrientationVertical, 10)
	top.AddCSSClass("chatot-status-top")
	p.segments = gtk.NewBox(gtk.OrientationHorizontal, 4)
	p.segments.SetHomogeneous(true)
	top.Append(p.segments)

	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	p.avatar = gtk.NewBox(gtk.OrientationVertical, 0)
	p.avatar.SetVAlign(gtk.AlignCenter)
	row.Append(p.avatar)
	text := gtk.NewBox(gtk.OrientationVertical, 0)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	p.name = gtk.NewLabel("")
	p.name.SetXAlign(0)
	p.name.SetEllipsize(pango.EllipsizeEnd)
	p.name.AddCSSClass("chatot-status-vname")
	text.Append(p.name)
	p.meta = gtk.NewLabel("")
	p.meta.SetXAlign(0)
	p.meta.AddCSSClass("chatot-status-vmeta")
	text.Append(p.meta)
	row.Append(text)
	// Pause sits before ⋮: the clock otherwise moves on after five seconds,
	// and a long text update or a photo worth a second look needs holding.
	// The glyph is cairo-drawn (no ⏸ font dependency).
	p.pauseIcon = gtk.NewDrawingArea()
	p.pauseIcon.SetSizeRequest(11, 11)
	p.pauseIcon.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawPausePlay(cr, float64(w), float64(h), p.paused())
	})
	p.pauseBtn = newRoundButton(p.pauseIcon, statusRoundBtnPx)
	p.pauseBtn.AddCSSClass("chatot-status-roundbtn")
	p.pauseBtn.SetTooltipText(pauseTooltip(false))
	p.pauseBtn.ConnectClicked(func() { p.setUserPaused(!p.userPaused) })
	row.Append(p.pauseBtn)
	p.menuBtn = newRoundGlyphButton("⋮", statusRoundBtnPx)
	p.menuBtn.AddCSSClass("chatot-status-roundbtn")
	p.menuBtn.SetTooltipText("Status options")
	p.menuBtn.ConnectClicked(func() { popupMenuBelow(p.menuBtn, p.menu()) })
	row.Append(p.menuBtn)
	closeBtn := newRoundGlyphButton("✕", statusRoundBtnPx)
	closeBtn.AddCSSClass("chatot-status-roundbtn")
	closeBtn.AddCSSClass("chatot-status-closebtn")
	closeBtn.SetTooltipText("Close")
	closeBtn.ConnectClicked(func() { cl.closeStatus() })
	row.Append(closeBtn)
	top.Append(row)
	root.Append(top)

	// The card sits centred; invisible prev/next zones cover the left and
	// right quarters of the stage.
	stage := gtk.NewOverlay()
	stage.SetVExpand(true)
	middle := gtk.NewBox(gtk.OrientationVertical, 12)
	middle.SetHAlign(gtk.AlignCenter)
	middle.SetVAlign(gtk.AlignCenter)
	middle.AddCSSClass("chatot-status-stage")
	p.cardSlot = gtk.NewBox(gtk.OrientationVertical, 0)
	p.cardSlot.SetHAlign(gtk.AlignCenter)
	middle.Append(p.cardSlot)
	p.caption = gtk.NewLabel("")
	p.caption.SetWrap(true)
	p.caption.SetJustify(gtk.JustifyCenter)
	p.caption.SetMaxWidthChars(40)
	p.caption.AddCSSClass("chatot-status-caption")
	middle.Append(p.caption)
	stage.SetChild(middle)
	prev := gtk.NewButton()
	prev.AddCSSClass("chatot-status-zone")
	prev.SetHAlign(gtk.AlignStart)
	prev.SetSizeRequest(160, -1)
	prev.SetTooltipText("Previous update")
	prev.ConnectClicked(func() { p.step(-1) })
	stage.AddOverlay(prev)
	next := gtk.NewButton()
	next.AddCSSClass("chatot-status-zone")
	next.SetHAlign(gtk.AlignEnd)
	next.SetSizeRequest(160, -1)
	next.SetTooltipText("Next update")
	next.ConnectClicked(func() { p.step(1) })
	stage.AddOverlay(next)
	// A click on the card itself (between the two zones) holds the clock,
	// and a "Paused" chip over the stage says so.
	hold := gtk.NewGestureClick()
	hold.ConnectReleased(func(_ int, _, _ float64) { p.setUserPaused(!p.userPaused) })
	middle.AddController(hold)
	p.pausedChip = gtk.NewLabel("Paused")
	p.pausedChip.AddCSSClass("chatot-status-paused")
	p.pausedChip.SetHAlign(gtk.AlignCenter)
	p.pausedChip.SetVAlign(gtk.AlignStart)
	p.pausedChip.SetVisible(false)
	stage.AddOverlay(p.pausedChip)
	root.Append(stage)

	p.footer = gtk.NewStack()
	p.footer.AddCSSClass("chatot-status-footer")
	replyRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	p.reply = gtk.NewEntry()
	p.reply.SetHExpand(true)
	p.reply.AddCSSClass("chatot-status-reply")
	p.reply.ConnectActivate(p.sendReply)
	// Typing a reply holds the clock, as WhatsApp does; the hold lifts when
	// focus leaves the field.
	typing := gtk.NewEventControllerFocus()
	typing.ConnectEnter(func() { p.setTyping(true) })
	typing.ConnectLeave(func() { p.setTyping(false) })
	p.reply.AddController(typing)
	replyRow.Append(p.reply)
	for _, e := range []string{"❤️", "😂", "😮", "👍"} {
		emoji := e
		b := newRoundGlyphButton(emoji, statusQuickBtnPx)
		b.AddCSSClass("chatot-status-quick")
		b.SetTooltipText("React " + emoji)
		b.ConnectClicked(func() { p.react(emoji) })
		replyRow.Append(b)
	}
	p.footer.AddNamed(replyRow, "reply")
	p.viewers = gtk.NewButtonWithLabel("Viewed by 0")
	p.viewers.AddCSSClass("chatot-status-viewers")
	p.viewers.SetHExpand(true)
	p.viewers.ConnectClicked(func() { p.showViewers() })
	p.footer.AddNamed(p.viewers, "mine")
	root.Append(p.footer)

	return p
}

// Show starts poster's run from its newest update.
func (p *StatusPane) Show(poster statusPoster) {
	p.stop()
	p.current = poster
	p.poster = poster.JID
	p.index = 0
	p.userPaused = false
	p.typing = false
	p.updatePauseUI()
	removeAllChildren(p.avatar)
	initial := initialFor(poster.Name)
	jid := poster.JID
	if poster.Mine {
		jid = p.cl.c.OwnJID()
		initial = p.cl.ownInitial()
	}
	p.avatar.Append(buildAvatar(p.cl.c, p.cl.avatarCache, jid, initial, 32))
	p.name.SetText(poster.Name)
	if poster.Mine {
		p.footer.SetVisibleChildName("mine")
	} else {
		p.footer.SetVisibleChildName("reply")
		p.reply.SetText("")
		p.reply.SetPlaceholderText("Reply to " + strings.SplitN(poster.Name, " ", 2)[0])
	}
	p.rebuildSegments()
	p.showItem()
}

// rebuildSegments lays one progress bar per update.
func (p *StatusPane) rebuildSegments() {
	removeAllChildren(p.segments)
	p.segs = nil
	for i := range p.current.Items {
		idx := i
		seg := gtk.NewDrawingArea()
		seg.SetSizeRequest(-1, 3)
		seg.SetHExpand(true)
		seg.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
			fill := 0.0
			switch {
			case idx < p.index:
				fill = 1
			case idx == p.index:
				fill = p.progress
			}
			drawStatusSegment(cr, float64(w), float64(h), fill)
		})
		p.segs = append(p.segs, seg)
		p.segments.Append(seg)
	}
}

// drawStatusSegment paints one progress bar: a 26% white track with a
// solid white fill of the given fraction, both pill-ended.
func drawStatusSegment(cr *cairo.Context, w, h, fill float64) {
	pill := func(width float64) {
		r := h / 2
		cr.NewPath()
		cr.Arc(r, r, r, math.Pi/2, 3*math.Pi/2)
		cr.Arc(width-r, r, r, -math.Pi/2, math.Pi/2)
		cr.ClosePath()
	}
	cr.SetSourceRGBA(1, 1, 1, 0.26)
	pill(w)
	cr.Fill()
	if fw := w * fill; fw >= h {
		cr.SetSourceRGB(1, 1, 1)
		pill(fw)
		cr.Fill()
	}
}

// showItem renders the current update and restarts the advance clock.
func (p *StatusPane) showItem() {
	if p.index < 0 || p.index >= len(p.current.Items) {
		return
	}
	item := p.current.Items[p.index]
	p.meta.SetText(statusViewerMeta(item, p.index, len(p.current.Items), time.Now()))
	removeAllChildren(p.cardSlot)
	p.cardSlot.Append(p.buildCard(item))
	caption := item.Text
	if item.Attachment == nil {
		caption = ""
	} else if item.Attachment.Caption != "" {
		caption = item.Attachment.Caption
	}
	p.caption.SetText(caption)
	p.caption.SetVisible(caption != "")
	if p.current.Mine {
		p.loadViewers(item)
	}
	p.progress = 0
	p.elapsed = 0
	if p.index < len(p.segs) {
		p.segs[p.index].QueueDraw()
	}
	p.startClock()
}

// startClock resumes the advance clock from p.elapsed, unless the viewer is
// held; reaching statusAdvanceMS steps to the next update.
func (p *StatusPane) startClock() {
	p.stop()
	if p.paused() {
		return
	}
	base := p.elapsed
	started := time.Now()
	p.tick = glib.TimeoutAdd(40, func() bool {
		p.elapsed = base + time.Since(started)
		p.progress = statusProgress(p.elapsed)
		if p.index < len(p.segs) {
			p.segs[p.index].QueueDraw()
		}
		if p.progress >= 1 {
			p.tick = 0
			p.step(1)
			return false
		}
		return true
	})
}

// statusProgress is how far along its statusAdvanceMS an update is, capped
// at one.
func statusProgress(elapsed time.Duration) float64 {
	return math.Min(1, float64(elapsed.Milliseconds())/statusAdvanceMS)
}

// paused reports whether the clock is held, by the user or by a reply
// being typed.
func (p *StatusPane) paused() bool { return p.userPaused || p.typing }

// setUserPaused is the pause button and the card click.
func (p *StatusPane) setUserPaused(on bool) {
	if p.userPaused == on {
		return
	}
	p.userPaused = on
	p.applyPause()
}

// setTyping is the reply field's focus hold.
func (p *StatusPane) setTyping(on bool) {
	if p.typing == on {
		return
	}
	p.typing = on
	p.applyPause()
}

// applyPause stops or resumes the clock for the current hold state and
// refreshes the button and chip. Resuming only restarts a clock for an
// update still on screen.
func (p *StatusPane) applyPause() {
	p.updatePauseUI()
	if p.paused() {
		p.stop()
		return
	}
	if p.tick == 0 && p.index < len(p.current.Items) {
		p.startClock()
	}
}

// updatePauseUI redraws the pause/play glyph and shows the Paused chip only
// for an explicit hold (typing needs no banner over the card).
func (p *StatusPane) updatePauseUI() {
	if p.pauseIcon != nil {
		p.pauseIcon.QueueDraw()
	}
	if p.pauseBtn != nil {
		p.pauseBtn.SetTooltipText(pauseTooltip(p.paused()))
	}
	if p.pausedChip != nil {
		p.pausedChip.SetVisible(p.userPaused)
	}
}

// pauseTooltip names the pause button's next action.
func pauseTooltip(paused bool) string {
	if paused {
		return "Resume"
	}
	return "Pause"
}

// stop halts the advance clock.
func (p *StatusPane) stop() {
	if p.tick != 0 {
		glib.SourceRemove(p.tick)
		p.tick = 0
	}
}

// step moves to the previous/next update; past the last one closes the
// viewer, before the first stays put.
func (p *StatusPane) step(d int) {
	next := p.index + d
	if next < 0 {
		next = 0
	}
	if next >= len(p.current.Items) {
		p.cl.closeStatus()
		return
	}
	p.index = next
	for _, s := range p.segs {
		s.QueueDraw()
	}
	p.showItem()
}

// Card geometry: the mockup's 318px-wide 4:5 card.
const (
	statusCardW = 318
	statusCardH = 398
)

// buildCard is the update itself: a photo (or its placeholder glyph) on a
// tinted card, or the text update's big centred type.
func (p *StatusPane) buildCard(item client.Message) gtk.Widgetter {
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("chatot-status-card")
	card.AddCSSClass(avatarColorClass(p.current.JID))
	card.SetSizeRequest(statusCardW, statusCardH)
	card.SetOverflow(gtk.OverflowHidden)

	if att := item.Attachment; att != nil && att.Kind == "image" {
		if pic := statusPicture(att); pic != nil {
			card.Append(coverInCard(pic))
			return card
		}
		glyph := gtk.NewLabel("🖼")
		glyph.AddCSSClass("chatot-status-glyph")
		glyph.SetVExpand(true)
		glyph.SetHExpand(true)
		card.Append(glyph)
		p.fetchStatusMedia(item, card, glyph)
		return card
	}
	text := gtk.NewLabel(item.Text)
	text.AddCSSClass("chatot-status-text")
	text.SetWrap(true)
	text.SetJustify(gtk.JustifyCenter)
	text.SetMaxWidthChars(24)
	text.SetVExpand(true)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	card.Append(text)
	return card
}

// statusPicture renders an image status from its local file or embedded
// thumbnail, nil when neither is at hand yet.
func statusPicture(att *client.Attachment) gtk.Widgetter {
	var texture *gdk.Texture
	var err error
	switch {
	case att.LocalPath != "":
		texture, err = gdk.NewTextureFromFilename(att.LocalPath)
	case len(att.Thumbnail) > 0:
		texture, err = gdk.NewTextureFromBytes(glib.NewBytesWithGo(att.Thumbnail))
	default:
		return nil
	}
	if err != nil {
		log.Printf("chatot: status picture: %v", err)
		return nil
	}
	pic := gtk.NewPictureForPaintable(texture)
	pic.SetContentFit(gtk.ContentFitCover)
	pic.SetCanShrink(true)
	pic.SetHAlign(gtk.AlignFill)
	pic.SetVAlign(gtk.AlignFill)
	return pic
}

// fetchStatusMedia downloads a photo status in the background and swaps it
// into card in place of the glyph once it lands (if the card is still up).
func (p *StatusPane) fetchStatusMedia(item client.Message, card *gtk.Box, glyph gtk.Widgetter) {
	go func() {
		path, err := p.cl.c.DownloadMedia(context.Background(), item.ID)
		if err != nil || path == "" {
			return
		}
		glib.IdleAdd(func() {
			if card.Parent() == nil {
				return
			}
			att := *item.Attachment
			att.LocalPath = path
			if pic := statusPicture(&att); pic != nil {
				card.Remove(glyph)
				card.Append(coverInCard(pic))
			}
		})
	}()
}

// menu is the viewer's ⋮: the contact actions, or our own status's menu
// when it is ours up.
func (p *StatusPane) menu() []menuItem {
	if p.current.Mine {
		return p.cl.myStatusMenu()
	}
	item := client.Message{}
	if p.index < len(p.current.Items) {
		item = p.current.Items[p.index]
	}
	return statusViewMenuItems(p.current.Muted, statusViewMenuActions{
		Reply: func() { p.reply.GrabFocus() },
		Forward: func() {
			if p.cl.onForward != nil {
				p.cl.onForward(item)
			}
		},
		Mute: func() { p.cl.setStatusMuted(p.current, !p.current.Muted) },
		// WhatsApp exposes no report call to linked devices.
		Report: func() { p.cl.toast("Reporting isn't available from a linked device — use your phone") },
	})
}

// loadViewers fills the "Viewed by" pill from item's read receipts.
func (p *StatusPane) loadViewers(item client.Message) {
	p.viewers.SetLabel("Viewed by …")
	id := item.ID
	go func() {
		viewers, err := p.cl.c.StatusViewers(id)
		glib.IdleAdd(func() {
			if p.index >= len(p.current.Items) || p.current.Items[p.index].ID != id {
				return
			}
			if err != nil {
				log.Printf("chatot: status viewers: %v", err)
				p.viewers.SetLabel("Viewed by ?")
				return
			}
			p.viewers.SetLabel(viewedByText(len(viewers)))
		})
	}()
}

// showViewers opens the viewer list for the update on screen.
func (p *StatusPane) showViewers() {
	if !p.current.Mine || p.index >= len(p.current.Items) {
		return
	}
	p.cl.showStatusViewers([]client.Message{p.current.Items[p.index]})
}

// sendReply sends the reply entry as a quoted reply to the poster.
func (p *StatusPane) sendReply() {
	text := strings.TrimSpace(p.reply.Text())
	if text == "" || p.current.Mine || p.index >= len(p.current.Items) {
		return
	}
	item := p.current.Items[p.index]
	poster, name := p.current.JID, p.current.Name
	p.reply.SetText("")
	go func() {
		_, err := p.cl.c.SendText(context.Background(), poster, text, &client.MsgRef{ChatJID: item.ChatJID, MsgID: item.ID})
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: status reply: %v", err)
				p.cl.toast("Couldn't send the reply")
				return
			}
			p.cl.toast("Reply sent to " + name)
		})
	}()
}

// react sends a quick reaction to the current update.
func (p *StatusPane) react(emoji string) {
	if p.current.Mine || p.index >= len(p.current.Items) {
		return
	}
	item := p.current.Items[p.index]
	poster, name := p.current.JID, p.current.Name
	go func() {
		err := p.cl.c.ReactToStatus(context.Background(), poster, item.ID, emoji)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: status reaction: %v", err)
				p.cl.toast("Couldn't react")
				return
			}
			p.cl.toast("Reacted " + emoji + " to " + name + "'s status")
		})
	}()
}

// buildStatusPage is the Status tab's sidebar column: one list holding the
// "My status" row, the captions and the poster rows.
func (cl *ChatList) buildStatusPage() gtk.Widgetter {
	cl.statusList = gtk.NewListBox()
	cl.statusList.AddCSSClass("navigation-sidebar")
	cl.statusList.AddCSSClass("chatot-tab-list")
	cl.statusList.SetSelectionMode(gtk.SelectionNone)
	cl.statusList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(cl.statusActions) && cl.statusActions[idx] != nil {
			cl.statusActions[idx]()
		}
	})
	return sidebarListScroller(cl.statusList)
}

// coverInCard pins a picture to the card's exact size. A GtkPicture's
// natural size is its texture's, which would stretch the card to the photo;
// as an overlay child it is allocated the sized base box instead and
// cover-fits into it.
func coverInCard(pic gtk.Widgetter) gtk.Widgetter {
	return coverInBox(pic, statusCardW, statusCardH)
}

// coverInBox is coverInCard at an arbitrary w×h.
func coverInBox(pic gtk.Widgetter, w, h int) gtk.Widgetter {
	base := gtk.NewBox(gtk.OrientationVertical, 0)
	base.SetSizeRequest(w, h)
	overlay := gtk.NewOverlay()
	overlay.SetChild(base)
	overlay.AddOverlay(pic)
	overlay.SetClipOverlay(pic, true)
	overlay.SetOverflow(gtk.OverflowHidden)
	return overlay
}
