package ui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// The Channels tab: the sidebar's followed list under a "Find channels"
// pill, the Find channels directory page, the channel pane (header, about
// chip, posts with reactions) and the info / share / report dialogs.

// newsletterRowView holds the pure display fields for one channel row in the
// sidebar. newsletterRowVM computes it so it can be unit-tested without a
// display.
type newsletterRowView struct {
	Name     string
	Snippet  string
	Muted    bool
	Verified bool
	Initial  string
}

// newsletterRowVM derives the sidebar view-model for one channel: its name,
// a description snippet, its badges and an avatar initial.
func newsletterRowVM(n client.Newsletter) newsletterRowView {
	name := n.Name
	if name == "" {
		name = "Unknown channel"
	}
	return newsletterRowView{
		Name:     name,
		Snippet:  strings.TrimSpace(n.Description),
		Muted:    n.Muted,
		Verified: n.Verified,
		Initial:  initialFor(name),
	}
}

// newsletterPostView holds the pure display fields for one channel post.
type newsletterPostView struct {
	Text      string
	TimeText  string
	Views     string
	Reactions string
	// Meta is the post's monospace line: "23:11 · 1,204 views".
	Meta string
}

// newsletterPostVM derives the read-view model for a single channel post:
// its text (a placeholder when empty), a formatted time, a view count and a
// top-reactions summary. now is injected for deterministic time formatting.
func newsletterPostVM(m client.NewsletterMessage, now time.Time) newsletterPostView {
	text := strings.TrimSpace(m.Text)
	if text == "" && m.Attachment == nil {
		text = "(no text)"
	}
	timeText := formatChatTime(m.TS, now)
	views := fmt.Sprintf("%d views", m.Views)
	return newsletterPostView{
		Text:      text,
		TimeText:  timeText,
		Views:     views,
		Reactions: reactionSummary(m.Reactions),
		Meta:      timeText + " · " + viewsText(m.Views),
	}
}

// reactionSummary renders up to the three most-reacted emoji as "emoji count"
// pairs, sorted by count (desc) then emoji (asc) for deterministic output.
func reactionSummary(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	for i, it := range sortedReactions(counts) {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", it.emoji, it.n))
	}
	return strings.Join(parts, "   ")
}

type reactionCount struct {
	emoji string
	n     int
}

// sortedReactions orders a post's reactions by count (desc) then emoji.
func sortedReactions(counts map[string]int) []reactionCount {
	items := make([]reactionCount, 0, len(counts))
	for e, n := range counts {
		if n > 0 {
			items = append(items, reactionCount{e, n})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].n != items[j].n {
			return items[i].n > items[j].n
		}
		return items[i].emoji < items[j].emoji
	})
	return items
}

// muteActionLabel is the pure label for a channel's mute toggle, reflecting
// its current muted state.
func muteActionLabel(muted bool) string {
	if muted {
		return "Unmute"
	}
	return "Mute"
}

// followLabel is the Follow/Following button's text.
func followLabel(following bool) string {
	if following {
		return "Following"
	}
	return "Follow"
}

// channelFooterText is the pane's bottom line.
func channelFooterText(following bool) string {
	if following {
		return "Only the channel owner can post here. You can react to and forward updates."
	}
	return "Follow this channel to get its updates."
}

// channelsCaptionText is the followed list's caption.
func channelsCaptionText(n int) string { return fmt.Sprintf("Following · %d", n) }

// discoverCategories are the directory's filter chips, in the mockup's order.
var discoverCategories = []string{"All", "Technology", "News", "Sports", "Food", "Entertainment"}

// discoverHeaderText is the directory list's caption: a result count while
// searching, else the category (or "Most followed" for All).
func discoverHeaderText(query, category string, n int) string {
	if strings.TrimSpace(query) != "" {
		return pluralCount(n, "result", "results")
	}
	if category == "All" || category == "" {
		return "Most followed"
	}
	return category
}

// filterDiscover keeps the channels in category ("All" keeps every one),
// most followed first.
func filterDiscover(list []client.Newsletter, category string) []client.Newsletter {
	out := make([]client.Newsletter, 0, len(list))
	for _, n := range list {
		if category == "All" || category == "" || n.Category == category {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Subscribers > out[j].Subscribers })
	return out
}

// hasCategories is true when any directory entry is filed under a category.
func hasCategories(list []client.Newsletter) bool {
	for _, n := range list {
		if n.Category != "" {
			return true
		}
	}
	return false
}

// discoverEmptyText explains an empty directory list.
func discoverEmptyText(query, category string) string {
	if strings.TrimSpace(query) != "" {
		return "No channels match “" + strings.TrimSpace(query) + "”"
	}
	return "Nothing in " + category + " yet"
}

// --- sidebar: followed channels ---

// buildChannelsPage is the Channels tab's sidebar column.
func (cl *ChatList) buildChannelsPage() gtk.Widgetter {
	page := gtk.NewBox(gtk.OrientationVertical, 0)

	findBar := gtk.NewBox(gtk.OrientationVertical, 0)
	findBar.AddCSSClass("chatot-find-bar")
	find := gtk.NewButton()
	find.AddCSSClass("chatot-find-btn")
	findBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	findBox.SetHAlign(gtk.AlignCenter)
	glyph := gtk.NewLabel("⌕")
	glyph.AddCSSClass("chatot-find-glyph")
	findBox.Append(glyph)
	findBox.Append(gtk.NewLabel("Find channels"))
	find.SetChild(findBox)
	find.ConnectClicked(cl.openDiscover)
	findBar.Append(find)
	page.Append(findBar)

	cl.channelsCaption = newSectionCaption(channelsCaptionText(0))
	cl.channelsCaption.AddCSSClass("chatot-channels-caption")
	page.Append(cl.channelsCaption)

	cl.channelsList = gtk.NewListBox()
	cl.channelsList.AddCSSClass("navigation-sidebar")
	cl.channelsList.AddCSSClass("chatot-tab-list")
	cl.channelsList.SetSelectionMode(gtk.SelectionSingle)
	cl.channelsList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(cl.newsletters) {
			cl.openChannel(cl.newsletters[idx])
		}
	})
	page.Append(sidebarListScroller(cl.channelsList))
	return page
}

// sidebarListScroller wraps a tab's list the way the chat list is wrapped:
// height-constrained so a long list scrolls instead of growing the window.
func sidebarListScroller(list gtk.Widgetter) *gtk.ScrolledWindow {
	s := gtk.NewScrolledWindow()
	s.SetChild(list)
	s.SetVExpand(true)
	s.SetPropagateNaturalHeight(false)
	s.SetMinContentHeight(0)
	s.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	s.SetSizeRequest(-1, 80)
	return s
}

// refreshChannels rebuilds the followed list from Newsletters.
func (cl *ChatList) refreshChannels() {
	if cl.tab != "channels" {
		return
	}
	channels, err := cl.c.Newsletters(context.Background())
	if err != nil {
		log.Printf("chatot: load channels: %v", err)
		channels = nil
	}
	cl.newsletters = channels
	cl.channelsCaption.SetText(strings.ToUpper(channelsCaptionText(len(channels))))
	cl.channelsList.RemoveAll()
	if len(channels) == 0 {
		empty := gtk.NewLabel("No channels followed yet")
		empty.AddCSSClass("chatot-search-empty")
		cl.channelsList.Append(inertRow(empty))
		return
	}
	for i, n := range channels {
		row := cl.buildChannelRow(n)
		cl.channelsList.Append(row)
		if n.ID == cl.currentChannel {
			cl.channelsList.SelectRow(cl.channelsList.RowAtIndex(i))
		}
	}
	if cl.channelPane.current.ID != "" {
		for _, n := range channels {
			if n.ID == cl.channelPane.current.ID {
				cl.channelPane.update(n)
			}
		}
	}
}

// buildChannelRow is one followed channel: 40px avatar, name with its
// verified/muted marks, then the description line.
func (cl *ChatList) buildChannelRow(n client.Newsletter) *gtk.ListBoxRow {
	vm := newsletterRowVM(n)
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-channel-row")
	row.Append(buildAvatar(cl.c, cl.avatarCache, n.ID, vm.Initial, 40))

	text := gtk.NewBox(gtk.OrientationVertical, 2)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	text.Append(channelNameLine(vm.Name, vm.Verified, vm.Muted, 13))
	if vm.Snippet != "" {
		snippet := gtk.NewLabel(vm.Snippet)
		snippet.SetXAlign(0)
		snippet.SetSingleLineMode(true)
		snippet.SetEllipsize(pango.EllipsizeEnd)
		snippet.SetMaxWidthChars(1)
		snippet.SetHExpand(true)
		snippet.AddCSSClass("chatot-chat-preview")
		text.Append(snippet)
	}
	row.Append(text)

	channel := n
	attachRowMenu(row, func() []menuItem { return channelMenuItems(channel, true, cl.channelActions(channel)) })

	lbr := gtk.NewListBoxRow()
	lbr.SetChild(row)
	return lbr
}

// channelNameLine is a channel's bold name followed by the ✓ and 🔇 marks.
func channelNameLine(name string, verified, muted bool, markSize int) *gtk.Box {
	line := gtk.NewBox(gtk.OrientationHorizontal, 5)
	l := gtk.NewLabel(name)
	l.SetXAlign(0)
	l.SetEllipsize(pango.EllipsizeEnd)
	// Natural width, capped: the marks hug the name's end, as in the
	// mockup, rather than being pushed to the row's far edge.
	l.SetMaxWidthChars(24)
	l.AddCSSClass("chatot-chat-name")
	line.Append(l)
	if verified {
		line.Append(newVerifiedMark(markSize))
	}
	if muted {
		m := gtk.NewLabel("🔇")
		m.AddCSSClass("chatot-channel-mutedmark")
		m.SetTooltipText("Muted")
		line.Append(m)
	}
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	line.Append(spacer)
	return line
}

// channelActions wires a channel's menu rows.
func (cl *ChatList) channelActions(n client.Newsletter) channelMenuActions {
	return channelMenuActions{
		Info:   func() { cl.showChannelInfoDialog(n) },
		Share:  func() { cl.showShareChannelDialog(n) },
		Mute:   func() { cl.setChannelMuted(n, !n.Muted) },
		Report: func() { cl.showReportChannelDialog(n) },
		Unfollow: func() {
			cl.confirmUnfollow(n)
		},
	}
}

// setChannelMuted flips a channel's mute and refreshes.
func (cl *ChatList) setChannelMuted(n client.Newsletter, mute bool) {
	go func() {
		err := cl.c.NewsletterSetMuted(context.Background(), n.ID, mute)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: mute channel: %v", err)
				cl.toast("Couldn't change notifications")
				return
			}
			if mute {
				cl.toast("Updates muted")
			} else {
				cl.toast("Updates unmuted")
			}
			n.Muted = mute
			if cl.channelPane.current.ID == n.ID {
				cl.channelPane.update(n)
			}
			cl.refreshChannels()
		})
	}()
}

// setChannelFollowing follows or unfollows a channel and refreshes; an
// unfollowed channel that was open drops back to the empty pane.
func (cl *ChatList) setChannelFollowing(n client.Newsletter, follow bool) {
	go func() {
		var err error
		if follow {
			err = cl.c.FollowNewsletter(context.Background(), n.ID)
		} else {
			err = cl.c.UnfollowNewsletter(context.Background(), n.ID)
		}
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: follow/unfollow channel: %v", err)
				cl.toast("Couldn't update the channel")
				return
			}
			if follow {
				cl.toast("Following " + n.Name)
			} else {
				cl.toast("Unfollowed " + n.Name)
			}
			n.Following = follow
			if cl.channelPane.current.ID == n.ID {
				cl.channelPane.update(n)
			}
			if !follow && cl.currentChannel == n.ID && !cl.discover {
				cl.currentChannel = ""
				cl.showPane("tabempty")
			}
			cl.refreshChannels()
			cl.refreshDiscover()
		})
	}()
}

// openChannel shows a channel in the pane and highlights its row.
func (cl *ChatList) openChannel(n client.Newsletter) {
	cl.currentChannel = n.ID
	cl.channelPane.Show(n)
	cl.showPane("channel")
	for i, c := range cl.newsletters {
		if c.ID == n.ID {
			cl.channelsList.SelectRow(cl.channelsList.RowAtIndex(i))
		}
	}
}

// openChannelByID opens a followed channel by JID (the follow-by-link flow).
func (cl *ChatList) openChannelByID(jid string) {
	for _, n := range cl.newsletters {
		if n.ID == jid {
			cl.openChannel(n)
			return
		}
	}
	cl.openChannel(client.Newsletter{ID: jid, Name: jid, Following: true})
}

// --- sidebar: Find channels ---

// buildDiscoverPage is the directory: a search pill, category chips, a
// caption and the results with their Follow buttons.
func (cl *ChatList) buildDiscoverPage() gtk.Widgetter {
	page := gtk.NewBox(gtk.OrientationVertical, 0)

	pillBar := gtk.NewBox(gtk.OrientationVertical, 0)
	pillBar.AddCSSClass("chatot-discover-search")
	pill, entry := newSearchPill("Search for channels", func(text string) {
		cl.discoverQuery = text
		cl.refreshDiscover()
	})
	cl.discoverEntry = entry
	pillBar.Append(pill)
	page.Append(pillBar)

	cl.discoverChips = gtk.NewBox(gtk.OrientationHorizontal, 6)
	cl.discoverChips.AddCSSClass("chatot-discover-chips")
	chipScroller := gtk.NewScrolledWindow()
	chipScroller.SetPolicy(gtk.PolicyExternal, gtk.PolicyNever)
	chipScroller.SetChild(cl.discoverChips)
	cl.discoverChipBar = chipScroller
	page.Append(chipScroller)

	cl.discoverCaption = newSectionCaption("Most followed")
	cl.discoverCaption.AddCSSClass("chatot-discover-caption")
	page.Append(cl.discoverCaption)

	cl.discoverList = gtk.NewListBox()
	cl.discoverList.AddCSSClass("navigation-sidebar")
	cl.discoverList.AddCSSClass("chatot-tab-list")
	cl.discoverList.SetSelectionMode(gtk.SelectionSingle)
	cl.discoverList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(cl.discoverRows) {
			cl.openChannel(cl.discoverRows[idx])
		}
	})
	page.Append(sidebarListScroller(cl.discoverList))
	return page
}

// openDiscover swaps the sidebar to the directory under a "← Find
// channels" header; the tab bar hides, as in the mockup.
func (cl *ChatList) openDiscover() {
	cl.discover = true
	cl.discoverQuery = ""
	cl.discoverCat = "All"
	cl.discoverEntry.SetText("")
	cl.showBackHead("Find channels", cl.closeDiscover)
	cl.modes.SetVisibleChildName("discover")
	cl.updateTabBarVisibility()
	cl.refreshDiscover()
}

// closeDiscover returns to the followed list.
func (cl *ChatList) closeDiscover() {
	cl.discover = false
	cl.showAccountHead()
	cl.modes.SetVisibleChildName("channels")
	cl.updateTabBarVisibility()
	cl.refreshChannels()
}

// refreshDiscover rebuilds the chips and results for the current query and
// category. The directory is fetched off the main loop.
func (cl *ChatList) refreshDiscover() {
	if !cl.discover {
		return
	}
	removeAllChildren(cl.discoverChips)
	for _, cat := range discoverCategories {
		category := cat
		chip := gtk.NewButtonWithLabel(category)
		chip.AddCSSClass("chatot-chip")
		if category == cl.discoverCat {
			chip.AddCSSClass("chatot-chip-active")
		}
		chip.ConnectClicked(func() {
			cl.discoverCat = category
			cl.refreshDiscover()
		})
		cl.discoverChips.Append(chip)
	}

	query, cat := cl.discoverQuery, cl.discoverCat
	go func() {
		list, err := cl.c.DiscoverNewsletters(context.Background(), query)
		glib.IdleAdd(func() {
			if !cl.discover || query != cl.discoverQuery || cat != cl.discoverCat {
				return
			}
			cl.discoverList.RemoveAll()
			if err != nil {
				cl.discoverRows = nil
				cl.discoverCaption.SetText(strings.ToUpper(discoverHeaderText(query, cat, 0)))
				text := "Couldn't reach the channel directory."
				if errors.Is(err, client.ErrUnsupported) {
					text = "This account can't browse the directory yet. Follow a channel with its link instead."
				}
				cl.discoverList.Append(inertRow(cl.discoverEmpty(text, "Follow with a link", cl.showFollowChannelDialog)))
				return
			}
			// The live directory files nothing under a category (the
			// server offers recommendations, not sections), so the chips
			// only show when the results carry categories.
			if !hasCategories(list) {
				cl.discoverCat, cat = "All", "All"
			}
			cl.discoverChipBar.SetVisible(hasCategories(list))
			rows := filterDiscover(list, cat)
			cl.discoverRows = rows
			cl.discoverCaption.SetText(strings.ToUpper(discoverHeaderText(query, cat, len(rows))))
			if len(rows) == 0 {
				cl.discoverList.Append(inertRow(cl.discoverEmpty(discoverEmptyText(query, cat), "Show all channels", func() {
					cl.discoverCat = "All"
					// SetText("") is silent when the box is already empty,
					// so the category reset refreshes on its own.
					cl.discoverEntry.SetText("")
					cl.refreshDiscover()
				})))
				return
			}
			for i, n := range rows {
				cl.discoverList.Append(cl.buildDiscoverRow(n))
				if n.ID == cl.currentChannel {
					cl.discoverList.SelectRow(cl.discoverList.RowAtIndex(i))
				}
			}
		})
	}()
}

// discoverEmpty is the directory's centred empty state with one action.
func (cl *ChatList) discoverEmpty(text, action string, onAction func()) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 9)
	box.AddCSSClass("chatot-list-empty")
	box.SetHAlign(gtk.AlignCenter)
	l := gtk.NewLabel(text)
	l.AddCSSClass("chatot-list-empty-text")
	l.SetWrap(true)
	l.SetJustify(gtk.JustifyCenter)
	l.SetMaxWidthChars(34)
	box.Append(l)
	btn := gtk.NewButtonWithLabel(action)
	btn.AddCSSClass("chatot-list-empty-action")
	btn.SetHAlign(gtk.AlignCenter)
	btn.ConnectClicked(onAction)
	box.Append(btn)
	return box
}

// buildDiscoverRow is one directory result: avatar, name with ✓, follower
// count in mono, and a Follow / Following outline button.
func (cl *ChatList) buildDiscoverRow(n client.Newsletter) *gtk.ListBoxRow {
	vm := newsletterRowVM(n)
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("chatot-channel-row")
	row.Append(buildAvatar(cl.c, cl.avatarCache, n.ID, vm.Initial, 40))

	text := gtk.NewBox(gtk.OrientationVertical, 2)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	text.Append(channelNameLine(vm.Name, vm.Verified, false, 13))
	meta := gtk.NewLabel(followersText(n.Subscribers))
	meta.SetXAlign(0)
	meta.SetEllipsize(pango.EllipsizeEnd)
	meta.SetMaxWidthChars(1)
	meta.SetHExpand(true)
	meta.AddCSSClass("chatot-mono-meta")
	text.Append(meta)
	row.Append(text)

	btn := gtk.NewButtonWithLabel(followLabel(n.Following))
	btn.AddCSSClass("chatot-follow-btn")
	if n.Following {
		btn.AddCSSClass("chatot-follow-btn-on")
		btn.SetTooltipText("Unfollow " + n.Name)
	} else {
		btn.SetTooltipText("Follow " + n.Name)
	}
	btn.SetVAlign(gtk.AlignCenter)
	channel := n
	btn.ConnectClicked(func() { cl.toggleFollowing(channel) })
	row.Append(btn)

	lbr := gtk.NewListBoxRow()
	lbr.SetChild(row)
	return lbr
}

// --- the channel pane ---

// ChannelPane is the mockup's channel reader: identity header with the
// Follow pill and ⋮, the about chip, the posts, and the footer line.
type ChannelPane struct {
	*gtk.Box

	cl *ChatList

	avatarSlot *gtk.Box
	nameLine   *gtk.Box
	followers  *gtk.Label
	followBtn  *gtk.Button
	about      *gtk.Label
	posts      *gtk.Box
	footer     *gtk.Label

	current   client.Newsletter
	postCount int
	menuBtn   *gtk.Button
	// firstReactAdd is the newest post's ＋ and firstReact reacts to that
	// post, both for the screenshot hooks.
	firstReactAdd *gtk.Button
	firstReact    func(emoji string)
	// mine is which emoji we reacted with per post, seeded from the
	// client's record on load and kept current as pills are clicked.
	mine map[string]string
	// viewedPosts is the set of posts already reported as viewed.
	viewedPosts map[string]bool
}

func newChannelPane(cl *ChatList) *ChannelPane {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-channel-pane")
	root.SetVExpand(true)
	root.SetHExpand(true)
	p := &ChannelPane{Box: root, cl: cl, mine: map[string]string{}, viewedPosts: map[string]bool{}}

	header := gtk.NewBox(gtk.OrientationHorizontal, 11)
	header.AddCSSClass("chatot-tabpane-header")
	p.avatarSlot = gtk.NewBox(gtk.OrientationVertical, 0)
	p.avatarSlot.SetVAlign(gtk.AlignCenter)
	header.Append(p.avatarSlot)
	text := gtk.NewBox(gtk.OrientationVertical, 1)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	p.nameLine = gtk.NewBox(gtk.OrientationHorizontal, 5)
	text.Append(p.nameLine)
	p.followers = gtk.NewLabel("")
	p.followers.SetXAlign(0)
	p.followers.AddCSSClass("chatot-tabpane-sub")
	text.Append(p.followers)
	header.Append(text)
	p.followBtn = gtk.NewButtonWithLabel("Follow")
	p.followBtn.AddCSSClass("chatot-follow-pill")
	p.followBtn.SetVAlign(gtk.AlignCenter)
	p.followBtn.ConnectClicked(func() { cl.toggleFollowing(p.current) })
	header.Append(p.followBtn)
	menu := newDotsButton("Channel options")
	menu.ConnectClicked(func() {
		popupMenuBelow(menu, channelMenuItems(p.current, false, cl.channelActions(p.current)))
	})
	header.Append(menu)
	p.menuBtn = menu
	root.Append(header)

	body := gtk.NewBox(gtk.OrientationVertical, 9)
	body.AddCSSClass("chatot-channel-body")
	p.about = gtk.NewLabel("")
	p.about.AddCSSClass("chatot-channel-about")
	p.about.SetWrap(true)
	p.about.SetJustify(gtk.JustifyCenter)
	p.about.SetMaxWidthChars(60)
	p.about.SetHAlign(gtk.AlignCenter)
	body.Append(p.about)
	p.posts = gtk.NewBox(gtk.OrientationVertical, 9)
	body.Append(p.posts)
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetChild(body)
	root.Append(scroller)

	p.footer = gtk.NewLabel("")
	p.footer.AddCSSClass("chatot-channel-footer")
	p.footer.SetWrap(true)
	p.footer.SetJustify(gtk.JustifyCenter)
	root.Append(p.footer)
	return p
}

// Show renders n's header, loads its posts off the main loop and asks the
// server for live count updates while the channel is open.
func (p *ChannelPane) Show(n client.Newsletter) {
	p.update(n)
	removeAllChildren(p.posts)
	loading := gtk.NewLabel("Loading updates…")
	loading.AddCSSClass("chatot-search-empty")
	p.posts.Append(loading)
	p.loadPosts()
	jid := n.ID
	go func() {
		if err := p.cl.c.NewsletterSubscribeLive(context.Background(), jid); err != nil {
			log.Printf("chatot: channel live updates: %v", err)
		}
	}()
}

// Reload re-fetches the posts when jid is the open channel: a new post or
// a live view/reaction update landed.
func (p *ChannelPane) Reload(jid string) {
	if jid == "" || p.current.ID != jid {
		return
	}
	p.loadPosts()
}

// loadPosts fetches the posts and swaps them in, seeding our reactions
// from the client's record and reporting the new ones as viewed.
func (p *ChannelPane) loadPosts() {
	jid := p.current.ID
	go func() {
		posts, err := p.cl.c.NewsletterMessages(context.Background(), jid, 30)
		glib.IdleAdd(func() {
			if p.current.ID != jid {
				return
			}
			removeAllChildren(p.posts)
			if err != nil {
				log.Printf("chatot: load channel posts: %v", err)
				p.posts.Append(newPaneEmptyState("📣", "Couldn't load updates", "Check the connection and open the channel again."))
				return
			}
			p.postCount = len(posts)
			if len(posts) == 0 {
				p.posts.Append(newPaneEmptyState("📣", "No updates yet", "Posts from this channel land here."))
				return
			}
			now := time.Now()
			p.firstReactAdd = nil
			for _, m := range posts {
				if m.MyReaction != "" {
					p.mine[p.mineKey(m)] = m.MyReaction
				} else {
					delete(p.mine, p.mineKey(m))
				}
				p.posts.Append(p.buildPost(m, newsletterPostVM(m, now)))
			}
			p.markViewed(jid, posts)
		})
	}()
}

// markViewed reports the posts not yet reported as viewed, which is what
// feeds the channel's view counts.
func (p *ChannelPane) markViewed(jid string, posts []client.NewsletterMessage) {
	var ids []int64
	for _, m := range posts {
		key := p.mineKey(m)
		if m.ServerID <= 0 || p.viewedPosts[key] {
			continue
		}
		p.viewedPosts[key] = true
		ids = append(ids, m.ServerID)
	}
	if len(ids) == 0 {
		return
	}
	go func() {
		if err := p.cl.c.NewsletterMarkViewed(context.Background(), jid, ids); err != nil {
			log.Printf("chatot: channel mark viewed: %v", err)
		}
	}()
}

// update refreshes the header for n without reloading the posts.
func (p *ChannelPane) update(n client.Newsletter) {
	p.current = n
	vm := newsletterRowVM(n)
	removeAllChildren(p.avatarSlot)
	p.avatarSlot.Append(buildAvatar(p.cl.c, p.cl.avatarCache, n.ID, vm.Initial, 34))
	removeAllChildren(p.nameLine)
	name := gtk.NewLabel(vm.Name)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.AddCSSClass("chatot-tabpane-title")
	p.nameLine.Append(name)
	if vm.Verified {
		p.nameLine.Append(newVerifiedMark(13))
	}
	if vm.Muted {
		m := gtk.NewLabel("🔇")
		m.AddCSSClass("chatot-channel-mutedmark")
		p.nameLine.Append(m)
	}
	p.followers.SetText(followersText(n.Subscribers))
	p.followBtn.SetLabel(followLabel(n.Following))
	if n.Following {
		p.followBtn.AddCSSClass("chatot-follow-pill-on")
	} else {
		p.followBtn.RemoveCSSClass("chatot-follow-pill-on")
	}
	p.about.SetText(strings.TrimSpace(n.Description))
	p.about.SetVisible(strings.TrimSpace(n.Description) != "")
	p.footer.SetText(channelFooterText(n.Following))
}

// buildPost is one update: the text, its meta line with a forward glyph,
// then the reaction pills and the dashed ＋.
func (p *ChannelPane) buildPost(m client.NewsletterMessage, vm newsletterPostView) gtk.Widgetter {
	post := gtk.NewBox(gtk.OrientationVertical, 7)
	post.AddCSSClass("chatot-channel-post")
	post.SetHAlign(gtk.AlignStart)

	if m.Attachment != nil {
		post.Append(p.buildPostMedia(m))
	}
	if vm.Text != "" {
		text := gtk.NewLabel(vm.Text)
		text.SetXAlign(0)
		text.SetWrap(true)
		text.SetWrapMode(pango.WrapWordChar)
		text.SetMaxWidthChars(64)
		text.SetSelectable(true)
		text.AddCSSClass("chatot-channel-post-text")
		post.Append(text)
	}

	metaRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	meta := gtk.NewLabel(vm.Meta)
	meta.SetXAlign(0)
	meta.SetHExpand(true)
	meta.AddCSSClass("chatot-mono-meta")
	metaRow.Append(meta)
	fwd := gtk.NewButtonWithLabel("↪")
	fwd.RemoveCSSClass("text-button")
	fwd.AddCSSClass("flat")
	fwd.AddCSSClass("chatot-channel-fwd")
	fwd.SetTooltipText("Forward to a chat")
	postText, channel := m.Text, p.current.ID
	fwd.ConnectClicked(func() {
		if p.cl.onForward != nil {
			p.cl.onForward(client.Message{ID: m.ID, ChatJID: channel, Text: postText, Attachment: m.Attachment})
		}
	})
	metaRow.Append(fwd)
	post.Append(metaRow)

	reacts := gtk.NewBox(gtk.OrientationHorizontal, 6)
	p.fillReactions(reacts, m)
	post.Append(reacts)
	return post
}

// The media frame in a channel post.
const (
	channelMediaW = 300
	channelMediaH = 200
)

// buildPostMedia is a post's attachment: a photo, sticker or video frame
// (the embedded thumbnail at once, the download swapped in when it lands;
// a video keeps its frame and a badge), any other kind as a chip line.
func (p *ChannelPane) buildPostMedia(m client.NewsletterMessage) gtk.Widgetter {
	att := m.Attachment
	switch att.Kind {
	case "image", "video", "sticker":
	default:
		chip := gtk.NewLabel(attachmentChipText(att))
		chip.SetXAlign(0)
		chip.AddCSSClass("chatot-channel-post-chip")
		return chip
	}
	frame := gtk.NewBox(gtk.OrientationVertical, 0)
	frame.AddCSSClass("chatot-channel-media")
	frame.SetSizeRequest(channelMediaW, channelMediaH)
	frame.SetOverflow(gtk.OverflowHidden)
	show := func(a *client.Attachment) bool {
		pic := statusPicture(a)
		if pic == nil {
			return false
		}
		removeAllChildren(frame)
		frame.Append(coverInBox(pic, channelMediaW, channelMediaH))
		return true
	}
	if !show(att) {
		glyph := gtk.NewLabel(postMediaGlyph(att.Kind))
		glyph.AddCSSClass("chatot-status-glyph")
		glyph.SetVExpand(true)
		glyph.SetHExpand(true)
		frame.Append(glyph)
	}
	if att.Kind == "video" {
		col := gtk.NewBox(gtk.OrientationVertical, 4)
		col.Append(frame)
		badge := gtk.NewLabel(attachmentChipText(att))
		badge.SetXAlign(0)
		badge.AddCSSClass("chatot-channel-post-chip")
		col.Append(badge)
		return col
	}
	if att.LocalPath == "" {
		id := m.ID
		go func() {
			path, err := p.cl.c.DownloadMedia(context.Background(), id)
			if err != nil {
				log.Printf("chatot: channel media %s: %v", id, err)
				return
			}
			if path == "" {
				return
			}
			glib.IdleAdd(func() {
				if frame.Root() == nil {
					return
				}
				a := *att
				a.LocalPath = path
				show(&a)
			})
		}()
	}
	return frame
}

// postMediaGlyph is the placeholder shown before a media post downloads.
func postMediaGlyph(kind string) string {
	switch kind {
	case "video":
		return "🎥"
	case "sticker":
		return "🌟"
	}
	return "🖼"
}

// attachmentChipText is the one-line description of a non-picture post
// attachment, as in the chat list previews.
func attachmentChipText(att *client.Attachment) string {
	switch att.Kind {
	case "audio":
		return "🎤 Voice message"
	case "document":
		if att.Filename != "" {
			return "📎 " + att.Filename
		}
		return "📎 Document"
	case "video":
		if att.DurationSecs > 0 {
			return fmt.Sprintf("🎥 Video · %d:%02d", att.DurationSecs/60, att.DurationSecs%60)
		}
		return "🎥 Video"
	}
	return "📎 " + att.Kind
}

// fillReactions lays a post's reaction pills (ours highlighted) and the ＋.
func (p *ChannelPane) fillReactions(row *gtk.Box, m client.NewsletterMessage) {
	removeAllChildren(row)
	mine := p.mine[p.mineKey(m)]
	for _, rc := range sortedReactions(m.Reactions) {
		emoji := rc.emoji
		pill := gtk.NewButtonWithLabel(fmt.Sprintf("%s %d", emoji, rc.n))
		pill.RemoveCSSClass("text-button")
		pill.AddCSSClass("chatot-channel-react")
		if emoji == mine {
			pill.AddCSSClass("chatot-channel-react-mine")
			pill.SetTooltipText("Remove your " + emoji)
		} else {
			pill.SetTooltipText("React with " + emoji)
		}
		pill.ConnectClicked(func() { p.react(row, m, emoji) })
		row.Append(pill)
	}
	plus := gtk.NewButtonWithLabel("＋")
	plus.RemoveCSSClass("text-button")
	plus.AddCSSClass("chatot-channel-react-add")
	plus.SetTooltipText("Add a reaction")
	plus.ConnectClicked(func() {
		pop := newReactionPickerPopover(plus, func(emoji string) { p.react(row, m, emoji) })
		pop.Popup()
	})
	row.Append(plus)
	if p.firstReactAdd == nil {
		p.firstReactAdd = plus
		p.firstReact = func(emoji string) { p.react(row, m, emoji) }
	}
}

// mineKey is the per-channel key our reaction to m is remembered under;
// post IDs alone repeat across channels.
func (p *ChannelPane) mineKey(m client.NewsletterMessage) string {
	return p.current.ID + "/" + m.ID
}

// react toggles our reaction on a post: the same emoji again removes it,
// another replaces it. The pills update at once (so a second click sees
// the first, rather than two adds racing the server round-trip) and roll
// back if the server refuses.
func (p *ChannelPane) react(row *gtk.Box, m client.NewsletterMessage, emoji string) {
	key := p.mineKey(m)
	prev := p.mine[key]
	next := emoji
	if prev == emoji {
		next = ""
	}
	if m.Reactions == nil {
		m.Reactions = map[string]int{}
	}
	set := func(from, to string) {
		applyReactionChange(m.Reactions, from, to)
		if to != "" {
			p.mine[key] = to
		} else {
			delete(p.mine, key)
		}
		p.fillReactions(row, m)
	}
	set(prev, next)
	jid := p.current.ID
	go func() {
		err := p.cl.c.NewsletterReact(context.Background(), jid, m.ID, m.ServerID, next)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: channel reaction: %v", err)
				p.cl.toast("Couldn't react")
				set(next, prev)
			}
		})
	}()
}

// applyReactionChange moves one reaction from prev to next in a post's
// counts: "" on either side means none, and a count that reaches zero
// drops its pill.
func applyReactionChange(counts map[string]int, prev, next string) {
	if prev != "" {
		if counts[prev] <= 1 {
			delete(counts, prev)
		} else {
			counts[prev]--
		}
	}
	if next != "" {
		counts[next]++
	}
}

// confirmUnfollow asks before unfollowing n: leaving drops the channel's
// history from the list, so a stray click on "Following" shouldn't do it.
func (cl *ChatList) confirmUnfollow(n client.Newsletter) {
	dialog := adw.NewAlertDialog("Unfollow "+n.Name+"?", "You'll stop getting its updates and it leaves your Channels list. You can follow it again from Find channels.")
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("unfollow", "Unfollow")
	dialog.SetResponseAppearance("unfollow", adw.ResponseDestructive)
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response == "unfollow" {
			cl.setChannelFollowing(n, false)
		}
	})
	presentAlert(dialog, cl.window)
}

// toggleFollowing follows n straight away, or asks before unfollowing.
func (cl *ChatList) toggleFollowing(n client.Newsletter) {
	if n.Following {
		cl.confirmUnfollow(n)
		return
	}
	cl.setChannelFollowing(n, true)
}

// newReactionPickerPopover is the "Pick a reaction" card (see the
// conversation's picker) hung off anchor.
func newReactionPickerPopover(anchor gtk.Widgetter, onPick func(emoji string)) *gtk.Popover {
	pop := gtk.NewPopover()
	pop.SetHasArrow(false)
	pop.SetParent(anchor)
	pop.AddCSSClass("chatot-menu")
	pop.AddCSSClass("chatot-react-picker")
	pop.ConnectClosed(func() { pop.Unparent() })

	col := gtk.NewBox(gtk.OrientationVertical, 6)
	caption := gtk.NewLabel("PICK A REACTION")
	caption.SetXAlign(0)
	caption.AddCSSClass("chatot-card-caption")
	col.Append(caption)
	grid := gtk.NewFlowBox()
	grid.SetSelectionMode(gtk.SelectionNone)
	grid.SetMinChildrenPerLine(reactPickerCols)
	grid.SetMaxChildrenPerLine(reactPickerCols)
	grid.SetRowSpacing(2)
	grid.SetColumnSpacing(2)
	grid.SetHomogeneous(true)
	for _, glyph := range reactPickerEmojis {
		emoji := glyph
		b := gtk.NewButtonWithLabel(emoji)
		b.AddCSSClass("flat")
		b.AddCSSClass("chatot-picker-emoji")
		b.SetSizeRequest(-1, pickerEmojiCell)
		b.ConnectClicked(func() {
			pop.Popdown()
			onPick(emoji)
		})
		grid.Insert(b, -1)
	}
	grid.SetSizeRequest(reactPickerWidth, -1)
	col.Append(grid)
	pop.SetChild(col)
	return pop
}

// --- dialogs ---

// channelInfoMeta is the info card's mono line: "4,218 followers · Technology".
func channelInfoMeta(n client.Newsletter) string {
	meta := followersText(n.Subscribers)
	if n.Category != "" {
		meta += " · " + n.Category
	}
	return meta
}

// showChannelInfoDialog is the mockup's channel info card.
func (cl *ChatList) showChannelInfoDialog(n client.Newsletter) {
	dialog := newCardDialog()
	dialog.SetTitle("")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(400, -1)
	vm := newsletterRowVM(n)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	head := gtk.NewBox(gtk.OrientationVertical, 8)
	head.AddCSSClass("chatot-info-head")
	avatar := buildAvatar(cl.c, cl.avatarCache, n.ID, vm.Initial, 76)
	avatar.SetHAlign(gtk.AlignCenter)
	avatar.AddCSSClass("chatot-info-avatar")
	head.Append(avatar)
	nameLine := gtk.NewBox(gtk.OrientationHorizontal, 6)
	nameLine.SetHAlign(gtk.AlignCenter)
	name := gtk.NewLabel(vm.Name)
	name.AddCSSClass("chatot-info-name")
	nameLine.Append(name)
	if vm.Verified {
		nameLine.Append(newVerifiedMark(15))
	}
	head.Append(nameLine)
	meta := gtk.NewLabel(channelInfoMeta(n))
	meta.AddCSSClass("chatot-mono-meta")
	head.Append(meta)
	if vm.Snippet != "" {
		about := gtk.NewLabel(vm.Snippet)
		about.AddCSSClass("chatot-info-about")
		about.SetWrap(true)
		about.SetJustify(gtk.JustifyCenter)
		about.SetMaxWidthChars(44)
		head.Append(about)
	}
	box.Append(head)

	card := newSettingsCard()
	// Informational rows: a no-op handler keeps them at full strength (a
	// nil one renders the row insensitive). The update count is fetched
	// when the pane hasn't loaded this channel's posts yet.
	updatesRow, updatesValue := newIconValueRow("📣", "Updates", "…", func() {})
	card.Add(updatesRow)
	if cl.channelPane.current.ID == n.ID && cl.channelPane.postCount > 0 {
		updatesValue.SetText(pluralCount(cl.channelPane.postCount, "update", "updates"))
	} else {
		go func() {
			posts, err := cl.c.NewsletterMessages(context.Background(), n.ID, 100)
			glib.IdleAdd(func() {
				if err != nil {
					updatesValue.SetText("—")
					return
				}
				updatesValue.SetText(pluralCount(len(posts), "update", "updates"))
			})
		}()
	}
	card.Add(newIconRow("📅", "Created", createdText(n.Created), false, func() {}))
	card.Add(newIconRow("🔗", "Channel link", strings.TrimPrefix(client.NewsletterLink(n), "https://"), false, func() {
		dialog.Close()
		cl.showShareChannelDialog(n)
	}))
	notif := "On"
	if n.Muted {
		notif = "Muted"
	}
	card.Add(newIconRow(muteRowIcon(n.Muted), "Notifications", notif, false, func() {
		dialog.Close()
		cl.setChannelMuted(n, !n.Muted)
	}))
	cardBox := gtk.NewBox(gtk.OrientationVertical, 0)
	cardBox.AddCSSClass("chatot-info-cardwrap")
	cardBox.Append(card)
	box.Append(cardBox)

	dialog.SetChild(box)
	dialog.Present()
}

// muteRowIcon is 🔔 for a muted channel's row (tap to unmute), else 🔇.
func muteRowIcon(muted bool) string {
	if muted {
		return "🔇"
	}
	return "🔔"
}

// showShareChannelDialog is "Share <channel>": the link box, Cancel and
// Send to a chat.
func (cl *ChatList) showShareChannelDialog(n client.Newsletter) {
	link := client.NewsletterLink(n)
	if link == "" {
		cl.toast("This channel has no share link")
		return
	}
	dialog := newCardDialog()
	dialog.SetTitle("Share " + n.Name)
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(420, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := gtk.NewBox(gtk.OrientationVertical, 10)
	body.AddCSSClass("chatot-dialog-body")
	body.Append(newDialogBodyText("Anyone with this link can open the channel and follow it. Followers are never shown to each other."))
	body.Append(newLinkBox(link, func() { cl.toast("Channel link copied to the clipboard") }))
	box.Append(body)

	footer := newDialogFooter()
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	footer.Append(spacer)
	footer.Append(newChipButton("Cancel", func() { dialog.Close() }))
	footer.Append(newPrimaryButton("Send to a chat", func() {
		dialog.Close()
		if cl.onForward != nil {
			cl.onForward(client.Message{Text: link})
		}
	}))
	box.Append(footer)
	dialog.SetChild(box)
	dialog.Present()
}

// showReportChannelDialog is "Report <channel>?": a reason list, the
// "Also unfollow" tick and a Report button that arms once a reason is picked.
func (cl *ChatList) showReportChannelDialog(n client.Newsletter) {
	dialog := newCardDialog()
	dialog.SetTitle("Report " + n.Name + "?")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(400, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := gtk.NewBox(gtk.OrientationVertical, 6)
	body.AddCSSClass("chatot-dialog-body")
	body.Append(newDialogBodyText("The most recent updates from this channel are sent to chatot for review. The channel is not told who reported it."))

	reason := ""
	var rows []*gtk.ToggleButton
	list := gtk.NewBox(gtk.OrientationVertical, 1)
	list.AddCSSClass("chatot-report-list")
	report := newPrimaryButton("Report", nil)
	report.SetSensitive(false)
	for _, r := range reportReasons {
		label := r
		row := gtk.NewToggleButton()
		row.AddCSSClass("chatot-report-row")
		content := gtk.NewBox(gtk.OrientationHorizontal, 11)
		dot := gtk.NewBox(gtk.OrientationHorizontal, 0)
		dot.AddCSSClass("chatot-radio")
		dot.SetSizeRequest(17, 17)
		dot.SetVAlign(gtk.AlignCenter)
		content.Append(dot)
		text := gtk.NewLabel(label)
		text.SetXAlign(0)
		text.SetHExpand(true)
		content.Append(text)
		row.SetChild(content)
		if len(rows) > 0 {
			row.SetGroup(rows[0])
		}
		row.ConnectToggled(func() {
			if row.Active() {
				reason = label
				report.SetSensitive(true)
			}
		})
		rows = append(rows, row)
		list.Append(row)
	}
	body.Append(list)

	unfollow := gtk.NewCheckButtonWithLabel("Also unfollow this channel")
	unfollow.AddCSSClass("chatot-report-check")
	body.Append(unfollow)
	box.Append(body)

	footer := newDialogFooter()
	spacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	spacer.SetHExpand(true)
	footer.Append(spacer)
	footer.Append(newChipButton("Cancel", func() { dialog.Close() }))
	report.AddCSSClass("chatot-danger-btn")
	report.ConnectClicked(func() {
		if reason == "" {
			cl.toast("Pick a reason first")
			return
		}
		drop := unfollow.Active()
		dialog.Close()
		// WhatsApp exposes no report call to linked devices: the
		// unfollow is real, the report has to be sent from the phone.
		if drop {
			cl.setChannelFollowing(n, false)
			cl.toast("Unfollowed " + n.Name + " · reports can only be sent from your phone")
		} else {
			cl.toast("Reports can only be sent from your phone")
		}
	})
	footer.Append(report)
	box.Append(footer)
	dialog.SetChild(box)
	dialog.Present()
}

// showFollowChannelDialog is "Follow with a link": paste a channel link,
// follow it, open it.
func (cl *ChatList) showFollowChannelDialog() {
	dialog := newCardDialog()
	dialog.SetTitle("Follow with a link")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(400, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := gtk.NewBox(gtk.OrientationVertical, 10)
	body.AddCSSClass("chatot-dialog-body")
	body.Append(newDialogBodyText("Paste a whatsapp.com/channel link. You start getting the channel's updates right away."))
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("whatsapp.com/channel/…")
	entry.AddCSSClass("chatot-dialog-entry")
	entry.AddCSSClass("chatot-mono-entry")
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
	follow := newPrimaryButton("Follow", nil)
	follow.SetSensitive(false)
	footer.Append(follow)
	box.Append(footer)

	entry.ConnectChanged(func() { follow.SetSensitive(strings.TrimSpace(entry.Text()) != "") })
	do := func() {
		link := strings.TrimSpace(entry.Text())
		if link == "" {
			return
		}
		follow.SetSensitive(false)
		go func() {
			jid, err := cl.c.FollowNewsletterByLink(context.Background(), link)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: follow by link: %v", err)
					hint.SetText("Couldn't follow that link, check it and try again")
					follow.SetSensitive(true)
					return
				}
				dialog.Close()
				cl.refreshChannels()
				cl.openChannelByID(jid)
			})
		}()
	}
	follow.ConnectClicked(do)
	entry.ConnectActivate(do)
	dialog.SetChild(box)
	dialog.SetDefaultWidget(follow)
	dialog.Present()
}
