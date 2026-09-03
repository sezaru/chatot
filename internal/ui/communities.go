package ui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// The Communities tab: the sidebar's community rows over a "New community"
// bar, the community pane (announcement group, groups you're in, other
// groups with Join), and the info / invite / add-group / join-with-link
// dialogs.

// communityRowView holds the pure display fields of one sidebar row.
type communityRowView struct {
	Name    string
	Sub     string
	Initial string
	Unread  string
}

// communityGroupsCount is the number of non-announcement groups.
func communityGroupsCount(c client.Community) int {
	n := 0
	for _, g := range c.Groups {
		if !g.Announcement {
			n++
		}
	}
	return n
}

// communityUnread sums the unread counts of the joined groups.
func communityUnread(c client.Community) int {
	n := 0
	for _, g := range c.Groups {
		if g.Joined {
			n += g.UnreadCount
		}
	}
	return n
}

// communityRowVM derives a sidebar row: "4 groups · 128 members" and the
// unread bubble.
func communityRowVM(c client.Community) communityRowView {
	name := c.Name
	if name == "" {
		name = "Community"
	}
	unread := ""
	if n := communityUnread(c); n > 0 {
		unread = fmt.Sprint(n)
	}
	return communityRowView{
		Name:    name,
		Sub:     pluralCount(communityGroupsCount(c), "group", "groups") + " · " + pluralCount(c.MemberCount, "member", "members"),
		Initial: initialFor(name),
		Unread:  unread,
	}
}

// communityPaneSub is the pane header's second line: "128 members · 4
// groups · you are an admin · muted".
func communityPaneSub(c client.Community) string {
	parts := []string{
		pluralCount(c.MemberCount, "member", "members"),
		pluralCount(communityGroupsCount(c), "group", "groups"),
	}
	if c.IsAdmin {
		parts = append(parts, "you are an admin")
	}
	if c.Muted {
		parts = append(parts, "muted")
	}
	return strings.Join(parts, " · ")
}

// communityGroupSub is a group row's second line: the announcement group's
// posting rule and member count, a joined group's preview, or a not-joined
// group's member count.
func communityGroupSub(g client.CommunityGroup) string {
	switch {
	case g.Announcement:
		return "Only admins can post · " + pluralCount(g.MemberCount, "member", "members")
	case g.Joined && g.Preview != "":
		return g.Preview
	case g.MemberCount > 0:
		return pluralCount(g.MemberCount, "member", "members")
	default:
		return "Group"
	}
}

// communitySection is one captioned block of the pane.
type communitySection struct {
	Caption string
	Groups  []client.CommunityGroup
	CanJoin bool
}

// communitySections splits a community's groups into the pane's three
// blocks, dropping any empty one.
func communitySections(c client.Community) []communitySection {
	var ann, mine, other []client.CommunityGroup
	for _, g := range c.Groups {
		switch {
		case g.Announcement:
			ann = append(ann, g)
		case g.Joined:
			mine = append(mine, g)
		default:
			other = append(other, g)
		}
	}
	var out []communitySection
	if len(ann) > 0 {
		out = append(out, communitySection{Caption: "Announcement group", Groups: ann})
	}
	if len(mine) > 0 {
		out = append(out, communitySection{Caption: "Groups you're in", Groups: mine})
	}
	if len(other) > 0 {
		out = append(out, communitySection{Caption: "Other groups in this community", Groups: other, CanJoin: true})
	}
	return out
}

// communityGroupState is the info card's chip per group.
func communityGroupState(g client.CommunityGroup) string {
	switch {
	case g.Announcement:
		return "announcements"
	case g.Joined:
		return "joined"
	default:
		return "not joined"
	}
}

// communityCreatedText is the info card's mono line: "Created 12 Mar 2024
// by Priya Raman".
func communityCreatedText(c client.Community, names map[string]string, own string) string {
	when := createdText(c.Created)
	if when == "" {
		return ""
	}
	by := ""
	switch {
	case c.CreatorJID == "":
	case isOwnJID(c.CreatorJID, own):
		by = " by you"
	default:
		by = " by " + posterName(c.CreatorJID, names)
	}
	return "Created " + when + by
}

// linkableGroups are the group chats not yet linked to any community (and
// not communities or their announcement groups themselves).
func linkableGroups(chats []client.Chat, communities []client.Community) []client.Chat {
	linked := map[string]bool{}
	for _, c := range communities {
		linked[c.JID] = true
		for _, g := range c.Groups {
			linked[g.JID] = true
		}
	}
	var out []client.Chat
	for _, ch := range chats {
		if ch.IsGroup && !ch.Archived && !linked[ch.JID] {
			out = append(out, ch)
		}
	}
	return out
}

// linkPickLabel is the add-group dialog's footer count.
func linkPickLabel(n int) string {
	if n == 0 {
		return "Pick one or more groups"
	}
	return pluralCount(n, "group selected", "groups selected")
}

// joinLinkHint validates a pasted invite as the user types.
func joinLinkHint(input string) (text string, ok bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", false
	}
	if isValidInviteInput(s) {
		return "Link looks valid", true
	}
	return "That does not look like a WhatsApp invite link", false
}

// --- sidebar ---

// buildCommunitiesPage is the Communities tab's sidebar column.
func (cl *ChatList) buildCommunitiesPage() gtk.Widgetter {
	page := gtk.NewBox(gtk.OrientationVertical, 0)
	cl.communitiesList = gtk.NewListBox()
	cl.communitiesList.AddCSSClass("navigation-sidebar")
	cl.communitiesList.AddCSSClass("chatot-tab-list")
	cl.communitiesList.SetSelectionMode(gtk.SelectionSingle)
	cl.communitiesList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx >= 0 && idx < len(cl.communities) {
			cl.openCommunity(cl.communities[idx])
		}
	})
	page.Append(sidebarListScroller(cl.communitiesList))

	bar := gtk.NewBox(gtk.OrientationVertical, 0)
	bar.AddCSSClass("chatot-newcomm-bar")
	btn := gtk.NewButtonWithLabel("New community")
	btn.AddCSSClass("chatot-newcomm-btn")
	btn.ConnectClicked(cl.showNewCommunityDialog)
	bar.Append(btn)
	page.Append(bar)
	return page
}

// refreshCommunities reloads the roster off the main loop and rebuilds the
// list, the open pane and the tab badge.
func (cl *ChatList) refreshCommunities() {
	// Fetches overlap (every event refreshes); only the newest one lands,
	// or a slow stale roster could resurrect a community just left.
	cl.communitiesGen++
	gen := cl.communitiesGen
	go func() {
		comms, err := cl.c.Communities(context.Background())
		glib.IdleAdd(func() {
			if gen != cl.communitiesGen {
				return
			}
			if err != nil {
				log.Printf("chatot: load communities: %v", err)
			} else {
				cl.communities = comms
			}
			cl.renderCommunities()
		})
	}()
}

// renderCommunities draws the cached roster.
func (cl *ChatList) renderCommunities() {
	total := 0
	for _, c := range cl.communities {
		total += communityUnread(c)
	}
	cl.tabBar.SetBadge("communities", total)
	if cl.tab != "communities" {
		return
	}
	cl.communitiesList.RemoveAll()
	if len(cl.communities) == 0 {
		cl.communitiesList.Append(inertRow(newListEmptyState("🏘", "No communities yet", "Communities bring related groups together under one announcement group.")))
	}
	for i, c := range cl.communities {
		cl.communitiesList.Append(cl.buildCommunityRow(c))
		if c.JID == cl.currentCommunity {
			cl.communitiesList.SelectRow(cl.communitiesList.RowAtIndex(i))
		}
	}
	if cl.currentCommunity != "" {
		found := false
		for _, c := range cl.communities {
			if c.JID == cl.currentCommunity {
				cl.communityPane.Show(c)
				found = true
			}
		}
		if !found {
			cl.currentCommunity = ""
			cl.showPane("tabempty")
		}
	}
}

// newListEmptyState is a sidebar list's centred empty state: a rounded
// disc, a bold title and a hint.
func newListEmptyState(glyph, title, hint string) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 9)
	box.AddCSSClass("chatot-list-emptystate")
	box.SetHAlign(gtk.AlignCenter)
	disc := gtk.NewLabel(glyph)
	disc.AddCSSClass("chatot-list-emptystate-disc")
	disc.SetSizeRequest(56, 56)
	disc.SetHAlign(gtk.AlignCenter)
	box.Append(disc)
	t := gtk.NewLabel(title)
	t.AddCSSClass("chatot-list-emptystate-title")
	box.Append(t)
	h := gtk.NewLabel(hint)
	h.AddCSSClass("chatot-list-emptystate-hint")
	h.SetWrap(true)
	h.SetJustify(gtk.JustifyCenter)
	h.SetMaxWidthChars(34)
	box.Append(h)
	return box
}

// buildCommunityRow is one community: a 42px rounded-square avatar, name,
// "N groups · M members" and the unread bubble.
func (cl *ChatList) buildCommunityRow(c client.Community) *gtk.ListBoxRow {
	vm := communityRowVM(c)
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	row.AddCSSClass("chatot-community-row")
	row.Append(newSquareAvatar(cl.c, cl.avatarCache, c.JID, vm.Initial, 42))

	text := gtk.NewBox(gtk.OrientationVertical, 2)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(vm.Name)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.SetMaxWidthChars(1)
	name.SetHExpand(true)
	name.AddCSSClass("chatot-chat-name")
	text.Append(name)
	sub := gtk.NewLabel(vm.Sub)
	sub.SetXAlign(0)
	sub.SetEllipsize(pango.EllipsizeEnd)
	sub.SetMaxWidthChars(1)
	sub.AddCSSClass("chatot-status-sub")
	text.Append(sub)
	row.Append(text)
	if vm.Unread != "" {
		badge := gtk.NewLabel(vm.Unread)
		badge.AddCSSClass("chatot-unread-badge")
		badge.SetVAlign(gtk.AlignCenter)
		row.Append(badge)
	}

	comm := c
	attachRowMenu(row, func() []menuItem { return communityMenuItems(comm, cl.communityActions(comm)) })

	lbr := gtk.NewListBoxRow()
	lbr.SetChild(row)
	return lbr
}

// newSquareAvatar is buildAvatar with the community's rounded-square shape.
func newSquareAvatar(c client.Client, cache *avatarCache, jid, initial string, size int) *gtk.Box {
	box := buildAvatar(c, cache, jid, initial, size)
	box.AddCSSClass("chatot-avatar-square")
	return box
}

// communityActions wires a community's menu rows.
func (cl *ChatList) communityActions(c client.Community) communityMenuActions {
	return communityMenuActions{
		Info: func() {
			cl.openCommunity(c)
			cl.showCommunityInfoDialog(c)
		},
		Invite: func() {
			cl.openCommunity(c)
			cl.showCommunityInviteDialog(c)
		},
		Mute:  func() { cl.setCommunityMuted(c, !c.Muted) },
		Leave: func() { cl.leaveCommunity(c) },
	}
}

// announcementGroup is the community's default sub-group, if listed.
func announcementGroup(c client.Community) (client.CommunityGroup, bool) {
	for _, g := range c.Groups {
		if g.Announcement {
			return g, true
		}
	}
	return client.CommunityGroup{}, false
}

// setCommunityMuted mutes the announcement group, which is what "mute
// announcements" means on WhatsApp.
func (cl *ChatList) setCommunityMuted(c client.Community, mute bool) {
	ann, ok := announcementGroup(c)
	if !ok {
		cl.toast("This community has no announcement group")
		return
	}
	go func() {
		err := cl.c.MuteChat(context.Background(), ann.JID, mute)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: mute announcements: %v", err)
				cl.toast("Couldn't change notifications")
				return
			}
			if mute {
				cl.toast("Announcements muted")
			} else {
				cl.toast("Announcements unmuted")
			}
			cl.refreshCommunities()
		})
	}()
}

// leaveCommunity leaves the parent group.
func (cl *ChatList) leaveCommunity(c client.Community) {
	go func() {
		err := cl.c.LeaveGroup(context.Background(), c.JID)
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: leave community: %v", err)
				cl.toast("Couldn't leave " + c.Name)
				return
			}
			cl.toast("Left " + c.Name)
			if cl.currentCommunity == c.JID {
				cl.currentCommunity = ""
				cl.showPane("tabempty")
			}
			cl.refreshCommunities()
		})
	}()
}

// openCommunity shows c in the pane and highlights its row.
func (cl *ChatList) openCommunity(c client.Community) {
	cl.currentCommunity = c.JID
	cl.communityPane.Show(c)
	cl.showPane("community")
	for i, x := range cl.communities {
		if x.JID == c.JID {
			cl.communitiesList.SelectRow(cl.communitiesList.RowAtIndex(i))
		}
	}
}

// openCommunityByID opens a community from the roster, reloading it first
// if it isn't cached yet (a community just joined by link).
func (cl *ChatList) openCommunityByID(jid string) {
	for _, c := range cl.communities {
		if c.JID == jid {
			cl.openCommunity(c)
			return
		}
	}
	cl.communitiesGen++
	gen := cl.communitiesGen
	go func() {
		comms, err := cl.c.Communities(context.Background())
		glib.IdleAdd(func() {
			if err != nil || gen != cl.communitiesGen {
				return
			}
			cl.communities = comms
			cl.renderCommunities()
			for _, c := range comms {
				if c.JID == jid {
					cl.openCommunity(c)
					return
				}
			}
			// Not a community after all: it's a plain group, open its chat.
			cl.selectTab("chats")
			if cl.onSelect != nil {
				cl.onSelect(jid)
			}
		})
	}()
}

// openCommunityGroup jumps to a joined group's chat.
func (cl *ChatList) openCommunityGroup(g client.CommunityGroup) {
	cl.selectTab("chats")
	if cl.onSelect != nil {
		cl.onSelect(g.JID)
	}
}

// confirmJoinGroup is the "Join <group>?" alert, then the join.
func (cl *ChatList) confirmJoinGroup(c client.Community, g client.CommunityGroup) {
	body := communityGroupSub(g) + " · part of " + c.Name + ". Members of the community can see that you joined."
	dialog := adw.NewAlertDialog("Join "+g.Name+"?", body)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("join", "Join")
	dialog.SetDefaultResponse("join")
	dialog.SetCloseResponse("cancel")
	dialog.ConnectResponse(func(response string) {
		if response == "join" {
			cl.joinGroup(c, g)
		}
	})
	dialog.Present(cl.window)
}

// joinGroup joins a linked group and refreshes.
func (cl *ChatList) joinGroup(c client.Community, g client.CommunityGroup) {
	go func() {
		err := cl.c.JoinCommunityGroup(context.Background(), c.JID, g.JID)
		glib.IdleAdd(func() {
			switch {
			case errors.Is(err, client.ErrUnsupported):
				cl.toast("Joining from here isn't available on this account yet — use your phone")
			case err != nil:
				log.Printf("chatot: join community group: %v", err)
				cl.toast("Couldn't join " + g.Name)
			default:
				cl.toast("Joined " + g.Name)
				cl.refreshCommunities()
			}
		})
	}()
}

// --- the community pane ---

// CommunityPane is the mockup's community page: an identity header that
// opens the info card, the grouped rows, and the Add group / Invite link
// footer.
type CommunityPane struct {
	*gtk.Box

	cl *ChatList

	avatarSlot *gtk.Box
	name       *gtk.Label
	sub        *gtk.Label
	rows       *gtk.Box
	addGroup   *gtk.Button
	menuBtn    *gtk.Button

	current client.Community
}

// CommunityInfoInitialTab and JoinLinkInitialText preset the info card's
// tab ("groups") and the join dialog's text for the screenshot hooks.
var (
	CommunityInfoInitialTab string
	JoinLinkInitialText     string
)

func newCommunityPane(cl *ChatList) *CommunityPane {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-community-pane")
	root.SetVExpand(true)
	root.SetHExpand(true)
	p := &CommunityPane{Box: root, cl: cl}

	header := gtk.NewBox(gtk.OrientationHorizontal, 11)
	header.AddCSSClass("chatot-tabpane-header")
	identity := gtk.NewButton()
	identity.AddCSSClass("flat")
	identity.AddCSSClass("chatot-tabpane-identity")
	identity.SetHExpand(true)
	identity.SetTooltipText("Community info")
	idBox := gtk.NewBox(gtk.OrientationHorizontal, 11)
	p.avatarSlot = gtk.NewBox(gtk.OrientationVertical, 0)
	p.avatarSlot.SetVAlign(gtk.AlignCenter)
	idBox.Append(p.avatarSlot)
	text := gtk.NewBox(gtk.OrientationVertical, 1)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	p.name = gtk.NewLabel("")
	p.name.SetXAlign(0)
	p.name.SetEllipsize(pango.EllipsizeEnd)
	p.name.AddCSSClass("chatot-tabpane-title")
	text.Append(p.name)
	p.sub = gtk.NewLabel("")
	p.sub.SetXAlign(0)
	p.sub.SetEllipsize(pango.EllipsizeEnd)
	p.sub.AddCSSClass("chatot-tabpane-sub")
	text.Append(p.sub)
	idBox.Append(text)
	identity.SetChild(idBox)
	identity.ConnectClicked(func() { cl.showCommunityInfoDialog(p.current) })
	header.Append(identity)
	menu := newDotsButton("Community options")
	menu.ConnectClicked(func() { popupMenuBelow(menu, communityMenuItems(p.current, cl.communityActions(p.current))) })
	header.Append(menu)
	p.menuBtn = menu
	root.Append(header)

	p.rows = gtk.NewBox(gtk.OrientationVertical, 0)
	p.rows.AddCSSClass("chatot-community-body")
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetChild(p.rows)
	root.Append(scroller)

	footer := gtk.NewBox(gtk.OrientationHorizontal, 8)
	footer.AddCSSClass("chatot-community-footer")
	footer.SetHomogeneous(true)
	p.addGroup = gtk.NewButtonWithLabel("＋ Add group")
	p.addGroup.AddCSSClass("chatot-outline-wide")
	p.addGroup.ConnectClicked(func() { cl.showAddGroupDialog(p.current) })
	footer.Append(p.addGroup)
	invite := gtk.NewButtonWithLabel("Invite link")
	invite.AddCSSClass("chatot-outline-wide")
	invite.ConnectClicked(func() { cl.showCommunityInviteDialog(p.current) })
	footer.Append(invite)
	root.Append(footer)
	return p
}

// Show renders c.
func (p *CommunityPane) Show(c client.Community) {
	p.current = c
	vm := communityRowVM(c)
	removeAllChildren(p.avatarSlot)
	p.avatarSlot.Append(newSquareAvatar(p.cl.c, p.cl.avatarCache, c.JID, vm.Initial, 36))
	p.name.SetText(vm.Name)
	p.sub.SetText(communityPaneSub(c))
	p.addGroup.SetSensitive(c.IsAdmin)
	if c.IsAdmin {
		p.addGroup.SetTooltipText("Link an existing group to this community")
	} else {
		p.addGroup.SetTooltipText("Only community admins can add groups")
	}
	removeAllChildren(p.rows)
	for _, s := range communitySections(c) {
		p.rows.Append(newSectionCaption(s.Caption))
		for _, g := range s.Groups {
			p.rows.Append(p.buildGroupRow(c, g, s.CanJoin))
		}
	}
}

// buildGroupRow is one group in the pane: a 38px avatar (the 📣 tile for
// the announcement group), name and line, then the unread bubble or Join.
func (p *CommunityPane) buildGroupRow(c client.Community, g client.CommunityGroup, canJoin bool) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	if g.Announcement {
		tile := gtk.NewLabel("📣")
		tile.AddCSSClass("chatot-announce-tile")
		tile.SetSizeRequest(38, 38)
		row.Append(tile)
	} else {
		row.Append(buildAvatar(p.cl.c, p.cl.avatarCache, g.JID, initialFor(g.Name), 38))
	}
	text := gtk.NewBox(gtk.OrientationVertical, 2)
	text.SetHExpand(true)
	text.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(g.Name)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.SetMaxWidthChars(1)
	name.SetHExpand(true)
	name.AddCSSClass("chatot-chat-name")
	text.Append(name)
	sub := gtk.NewLabel(communityGroupSub(g))
	sub.SetXAlign(0)
	sub.SetEllipsize(pango.EllipsizeEnd)
	sub.SetMaxWidthChars(1)
	sub.AddCSSClass("chatot-status-sub")
	text.Append(sub)
	row.Append(text)
	if g.UnreadCount > 0 && g.Joined {
		badge := gtk.NewLabel(fmt.Sprint(g.UnreadCount))
		badge.AddCSSClass("chatot-unread-badge")
		badge.SetVAlign(gtk.AlignCenter)
		row.Append(badge)
	}
	if canJoin {
		join := gtk.NewButtonWithLabel("Join")
		join.AddCSSClass("chatot-join-btn")
		join.SetVAlign(gtk.AlignCenter)
		join.ConnectClicked(func() { p.cl.joinGroup(c, g) })
		row.Append(join)
	}

	btn := gtk.NewButton()
	btn.AddCSSClass("chatot-community-grouprow")
	btn.SetChild(row)
	btn.ConnectClicked(func() {
		if canJoin {
			p.cl.confirmJoinGroup(c, g)
		} else {
			p.cl.openCommunityGroup(g)
		}
	})
	return btn
}

// --- dialogs ---

// showCommunityInfoDialog is the info card: avatar, name, about, created
// line, then Members / Groups tabs.
func (cl *ChatList) showCommunityInfoDialog(c client.Community) {
	dialog := newCardDialog()
	dialog.SetTitle("")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(400, -1)
	vm := communityRowVM(c)
	names := chatNames(cl.c)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	head := gtk.NewBox(gtk.OrientationVertical, 8)
	head.AddCSSClass("chatot-info-head")
	avatar := newSquareAvatar(cl.c, cl.avatarCache, c.JID, vm.Initial, 76)
	avatar.SetHAlign(gtk.AlignCenter)
	avatar.AddCSSClass("chatot-info-avatar")
	head.Append(avatar)
	name := gtk.NewLabel(vm.Name)
	name.AddCSSClass("chatot-info-name")
	head.Append(name)
	if strings.TrimSpace(c.Description) != "" {
		about := gtk.NewLabel(c.Description)
		about.AddCSSClass("chatot-info-about")
		about.SetWrap(true)
		about.SetJustify(gtk.JustifyCenter)
		about.SetMaxWidthChars(44)
		head.Append(about)
	}
	if created := communityCreatedText(c, names, cl.c.OwnJID()); created != "" {
		l := gtk.NewLabel(created)
		l.AddCSSClass("chatot-mono-meta")
		head.Append(l)
	}
	box.Append(head)

	stack := gtk.NewStack()
	stack.AddNamed(cl.buildMembersTab(c, names), "members")
	stack.AddNamed(cl.buildGroupsTab(c), "groups")
	if CommunityInfoInitialTab == "groups" {
		stack.SetVisibleChildName("groups")
		CommunityInfoInitialTab = ""
	}
	tabs := newSegmentedSwitcher(stack, []segmentedPage{{"members", "Members"}, {"groups", "Groups"}}, true)
	tabsBox := gtk.NewBox(gtk.OrientationVertical, 0)
	tabsBox.AddCSSClass("chatot-info-tabs")
	tabsBox.Append(tabs)
	box.Append(tabsBox)
	box.Append(stack)

	dialog.SetChild(box)
	dialog.Present()
}

// buildMembersTab is the info card's searchable member list.
func (cl *ChatList) buildMembersTab(c client.Community, names map[string]string) gtk.Widgetter {
	page := gtk.NewBox(gtk.OrientationVertical, 0)
	pillBar := gtk.NewBox(gtk.OrientationVertical, 0)
	pillBar.AddCSSClass("chatot-info-search")
	list := gtk.NewBox(gtk.OrientationVertical, 0)
	list.AddCSSClass("chatot-info-list")
	own := cl.c.OwnJID()
	render := func(query string) {
		removeAllChildren(list)
		q := strings.ToLower(strings.TrimSpace(query))
		shown := 0
		for _, m := range c.Members {
			name := posterName(m.JID, names)
			initial := initialFor(name)
			if isOwnJID(m.JID, own) {
				name, initial = "You", cl.ownInitial()
			}
			if q != "" && !strings.Contains(strings.ToLower(name), q) {
				continue
			}
			row := gtk.NewBox(gtk.OrientationHorizontal, 10)
			row.AddCSSClass("chatot-info-row")
			row.Append(buildAvatar(cl.c, cl.avatarCache, m.JID, initial, 32))
			l := gtk.NewLabel(name)
			l.SetXAlign(0)
			l.SetHExpand(true)
			l.SetEllipsize(pango.EllipsizeEnd)
			l.AddCSSClass("chatot-info-rowname")
			row.Append(l)
			if m.IsAdmin || m.IsSuperAdmin {
				chip := gtk.NewLabel("admin")
				chip.AddCSSClass("chatot-role-chip")
				chip.SetVAlign(gtk.AlignCenter)
				row.Append(chip)
			}
			list.Append(row)
			shown++
		}
		if shown == 0 {
			empty := gtk.NewLabel("No members match “" + strings.TrimSpace(query) + "”")
			empty.AddCSSClass("chatot-search-empty")
			list.Append(empty)
		}
	}
	pill, _ := newSearchPill("Search members", render)
	pillBar.Append(pill)
	page.Append(pillBar)
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetMaxContentHeight(250)
	scroller.SetPropagateNaturalHeight(true)
	scroller.SetChild(list)
	page.Append(scroller)
	render("")
	return page
}

// buildGroupsTab is the info card's group list with its state chips.
func (cl *ChatList) buildGroupsTab(c client.Community) gtk.Widgetter {
	list := gtk.NewBox(gtk.OrientationVertical, 0)
	list.AddCSSClass("chatot-info-list")
	list.AddCSSClass("chatot-info-list-groups")
	for _, g := range c.Groups {
		row := gtk.NewBox(gtk.OrientationHorizontal, 10)
		row.AddCSSClass("chatot-info-row")
		if g.Announcement {
			tile := gtk.NewLabel("📣")
			tile.AddCSSClass("chatot-announce-tile")
			tile.AddCSSClass("chatot-announce-tile-sm")
			tile.SetSizeRequest(32, 32)
			row.Append(tile)
		} else {
			row.Append(buildAvatar(cl.c, cl.avatarCache, g.JID, initialFor(g.Name), 32))
		}
		text := gtk.NewBox(gtk.OrientationVertical, 1)
		text.SetHExpand(true)
		text.SetVAlign(gtk.AlignCenter)
		name := gtk.NewLabel(g.Name)
		name.SetXAlign(0)
		name.SetEllipsize(pango.EllipsizeEnd)
		name.SetMaxWidthChars(1)
		name.SetHExpand(true)
		name.AddCSSClass("chatot-info-rowname")
		name.AddCSSClass("chatot-info-rowname-bold")
		text.Append(name)
		sub := gtk.NewLabel(communityGroupSub(g))
		sub.SetXAlign(0)
		sub.SetEllipsize(pango.EllipsizeEnd)
		sub.SetMaxWidthChars(1)
		sub.AddCSSClass("chatot-info-rowsub")
		text.Append(sub)
		row.Append(text)
		chip := gtk.NewLabel(communityGroupState(g))
		chip.AddCSSClass("chatot-role-chip")
		chip.SetVAlign(gtk.AlignCenter)
		row.Append(chip)
		list.Append(row)
	}
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetMaxContentHeight(290)
	scroller.SetPropagateNaturalHeight(true)
	scroller.SetChild(list)
	return scroller
}

// showCommunityInviteDialog is the shared invite card titled for the
// community, with the community wording.
func (cl *ChatList) showCommunityInviteDialog(c client.Community) {
	showInviteLinkDialog(cl.window, cl.c, c.JID, "Invite to "+c.Name, communityInviteBody, cl.toast, cl.onForward)
}

// showAddGroupDialog is "Add a group to <community>": tick the groups to
// link, then Add.
func (cl *ChatList) showAddGroupDialog(c client.Community) {
	if !c.IsAdmin {
		cl.toast("Only community admins can add groups")
		return
	}
	dialog := newCardDialog()
	dialog.SetTitle("Add a group to " + c.Name)
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(400, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := gtk.NewBox(gtk.OrientationVertical, 6)
	body.AddCSSClass("chatot-dialog-body")
	body.Append(newDialogBodyText("Linked groups keep their own members and history. Everyone in them can see the community announcements."))

	picked := map[string]bool{}
	countLabel := gtk.NewLabel(linkPickLabel(0))
	countLabel.SetXAlign(0)
	countLabel.SetHExpand(true)
	countLabel.AddCSSClass("chatot-dialog-hint")
	add := newPrimaryButton("Add", nil)
	add.SetSensitive(false)
	sync := func() {
		countLabel.SetText(linkPickLabel(len(picked)))
		add.SetSensitive(len(picked) > 0)
	}

	list := gtk.NewBox(gtk.OrientationVertical, 1)
	list.AddCSSClass("chatot-report-list")
	groups := linkableGroups(chatsOrEmpty(cl.c), cl.communities)
	if len(groups) == 0 {
		empty := gtk.NewLabel("Every group you are in is already linked to a community.")
		empty.AddCSSClass("chatot-search-empty")
		empty.SetWrap(true)
		list.Append(empty)
	}
	for _, g := range groups {
		group := g
		row := gtk.NewToggleButton()
		row.AddCSSClass("chatot-report-row")
		content := gtk.NewBox(gtk.OrientationHorizontal, 11)
		content.Append(buildAvatar(cl.c, cl.avatarCache, group.JID, initialFor(group.Name), 34))
		text := gtk.NewBox(gtk.OrientationVertical, 1)
		text.SetHExpand(true)
		text.SetVAlign(gtk.AlignCenter)
		name := gtk.NewLabel(group.Name)
		name.SetXAlign(0)
		name.SetEllipsize(pango.EllipsizeEnd)
		name.SetMaxWidthChars(1)
		name.SetHExpand(true)
		name.AddCSSClass("chatot-info-rowname")
		name.AddCSSClass("chatot-info-rowname-bold")
		text.Append(name)
		sub := gtk.NewLabel(groupRowSub(group))
		sub.SetXAlign(0)
		sub.SetEllipsize(pango.EllipsizeEnd)
		sub.SetMaxWidthChars(1)
		sub.AddCSSClass("chatot-info-rowsub")
		text.Append(sub)
		content.Append(text)
		tick := gtk.NewCheckButton()
		tick.AddCSSClass("chatot-report-check")
		tick.SetVAlign(gtk.AlignCenter)
		tick.SetCanFocus(false)
		content.Append(tick)
		row.SetChild(content)
		row.ConnectToggled(func() {
			if row.Active() {
				picked[group.JID] = true
			} else {
				delete(picked, group.JID)
			}
			if tick.Active() != row.Active() {
				tick.SetActive(row.Active())
			}
			sync()
		})
		tick.ConnectToggled(func() {
			if row.Active() != tick.Active() {
				row.SetActive(tick.Active())
			}
		})
		list.Append(row)
	}
	body.Append(list)
	box.Append(body)

	footer := newDialogFooter()
	footer.Append(countLabel)
	footer.Append(newChipButton("Cancel", func() { dialog.Close() }))
	add.ConnectClicked(func() {
		jids := make([]string, 0, len(picked))
		names := map[string]string{}
		for _, g := range groups {
			if picked[g.JID] {
				jids = append(jids, g.JID)
				names[g.JID] = g.Name
			}
		}
		if len(jids) == 0 {
			return
		}
		add.SetSensitive(false)
		go func() {
			var firstErr error
			for _, jid := range jids {
				if err := cl.c.LinkGroupToCommunity(context.Background(), c.JID, jid); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			glib.IdleAdd(func() {
				if firstErr != nil {
					log.Printf("chatot: link group: %v", firstErr)
					cl.toast("Couldn't add the groups")
					add.SetSensitive(true)
					return
				}
				dialog.Close()
				if len(jids) == 1 {
					cl.toast(names[jids[0]] + " added to " + c.Name)
				} else {
					cl.toast(fmt.Sprintf("%d groups added to %s", len(jids), c.Name))
				}
				cl.refreshCommunities()
			})
		}()
	})
	footer.Append(add)
	box.Append(footer)
	dialog.SetChild(box)
	dialog.Present()
}

// groupRowSub is a linkable group's line: its preview, else "Group".
func groupRowSub(g client.Chat) string {
	if strings.TrimSpace(g.Preview) != "" {
		return g.Preview
	}
	return "Group"
}

// showJoinCommunityLinkDialog is "Join with a link".
func (cl *ChatList) showJoinCommunityLinkDialog() {
	dialog := newCardDialog()
	dialog.SetTitle("Join with a link")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetDefaultSize(400, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := gtk.NewBox(gtk.OrientationVertical, 6)
	body.AddCSSClass("chatot-dialog-body")
	body.Append(newDialogBodyText("Paste a community invite link. You will join the announcement group first and can pick the other groups afterwards."))
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("chat.whatsapp.com/…")
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
	join := newPrimaryButton("Join", nil)
	join.SetSensitive(false)
	footer.Append(join)
	box.Append(footer)

	entry.ConnectChanged(func() {
		text, ok := joinLinkHint(entry.Text())
		hint.SetText(text)
		if ok {
			hint.RemoveCSSClass("chatot-dialog-hint-bad")
			hint.AddCSSClass("chatot-dialog-hint-good")
		} else {
			hint.RemoveCSSClass("chatot-dialog-hint-good")
			hint.AddCSSClass("chatot-dialog-hint-bad")
		}
		join.SetSensitive(strings.TrimSpace(entry.Text()) != "")
	})
	do := func() {
		code := strings.TrimSpace(entry.Text())
		if code == "" {
			return
		}
		if _, ok := joinLinkHint(code); !ok {
			cl.toast("That link is not a WhatsApp invite")
			return
		}
		join.SetSensitive(false)
		go func() {
			jid, err := cl.c.JoinGroupWithLink(context.Background(), code)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: join by link: %v", err)
					hint.SetText("Couldn't join with that link")
					join.SetSensitive(true)
					return
				}
				dialog.Close()
				cl.toast("Joined via invite link")
				cl.openCommunityByID(jid)
			})
		}()
	}
	join.ConnectClicked(do)
	entry.ConnectActivate(do)
	dialog.SetChild(box)
	dialog.SetDefaultWidget(join)
	dialog.Present()
	if JoinLinkInitialText != "" {
		entry.SetText(JoinLinkInitialText)
		JoinLinkInitialText = ""
	}
}
