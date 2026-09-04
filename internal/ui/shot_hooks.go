package ui

import (
	"context"
	"log"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// Dev/screenshot hooks (CHATOT_SHOT=<state> in main.go): programmatic ways to
// reach every popover/menu/dialog/pane the mockup shows, so the real window
// can be captured in each state without injecting pointer input. Nothing here
// runs unless main.go's hook dispatcher calls it; production paths are
// untouched.

// bubbleShot records the hover-action widgets a message bubble owns so a hook
// can reveal them; populated only while EnableShotHooks has been called.
type bubbleShot struct {
	affordances bubbleAffordances
}

var shotRegistry map[string]*bubbleShot

// EnableShotHooks turns on the per-bubble registry the message hooks read.
func EnableShotHooks() { shotRegistry = make(map[string]*bubbleShot) }

func shotRegister(msgID string, fn func(*bubbleShot)) {
	if shotRegistry == nil {
		return
	}
	s := shotRegistry[msgID]
	if s == nil {
		s = &bubbleShot{}
		shotRegistry[msgID] = s
	}
	fn(s)
}

// --- ChatList ---

// PopupPlusMenu opens the header ＋ popover.
func (cl *ChatList) PopupPlusMenu() { cl.plusBtn.Popup() }

// PopupAppMenu opens the header ⋮ popover.
func (cl *ChatList) PopupAppMenu() { cl.appMenuBtn.Popup() }

// PopupLabelOverflow opens the chip row's "…" label popover.
func (cl *ChatList) PopupLabelOverflow() {
	if cl.overflowBtn != nil {
		cl.overflowBtn.Activate()
	}
}

// PopupRowMenu opens the right-click context menu on the row for jid.
func (cl *ChatList) PopupRowMenu(jid string) {
	chats, err := cl.c.Chats(0)
	if err != nil {
		return
	}
	for i, id := range cl.rowJIDs {
		if id != jid {
			continue
		}
		row := cl.list.RowAtIndex(i)
		if row == nil {
			return
		}
		box, ok := glib.BaseObject(row.Child()).Cast().(*gtk.Box)
		if !ok {
			return
		}
		for _, chat := range chats {
			if chat.JID == jid {
				showChatContextMenu(cl, box, cl.rowMenuItems(chat), 120, 24)
				return
			}
		}
	}
}

// ShowArchived switches the sidebar into archived mode.
func (cl *ChatList) ShowArchived() { cl.archiveT.SetActive(true) }

// ShowStarred switches the sidebar into starred-messages mode.
func (cl *ChatList) ShowStarred() { fire(cl.onStarred) }

// ShowNewChat / ShowNewGroup / ShowNewCommunity / ShowJoinInvite open the ＋
// menu's flows.
func (cl *ChatList) ShowNewChat()      { cl.showNewChatDialog() }
func (cl *ChatList) ShowNewGroup()     { cl.showNewGroupDialog() }
func (cl *ChatList) ShowNewCommunity() { cl.showNewCommunityDialog() }
func (cl *ChatList) ShowJoinInvite()   { cl.showJoinGroupDialog() }

// SetSearchText types q into the sidebar search entry.
func (cl *ChatList) SetSearchText(q string) { cl.search.SetText(q) }

// ShowAbout / ShowShortcuts / ShowBlocked / ShowPrivacy open the ⋮ menu's
// dialogs.
func (cl *ChatList) ShowAbout()     { showAboutDialog(cl.window) }
func (cl *ChatList) ShowShortcuts() { cl.showPreferencesPage("shortcuts") }
func (cl *ChatList) ShowBlocked()   { showBlockedDialog(cl.window, cl.c) }
func (cl *ChatList) ShowPrivacy()   { cl.showPreferencesPage("privacy") }

// showPreferencesPage opens Preferences on one of its pages.
func (cl *ChatList) showPreferencesPage(page string) {
	PreferencesInitialPage = page
	cl.appMenuBtn.ActivateAction("app.preferences", nil)
}

// --- ConversationView ---

// OpenViewer opens m in the attachment viewer pane (the bubble's click).
func (cv *ConversationView) OpenViewer(m client.Message) {
	if cv.onOpenViewer != nil {
		cv.onOpenViewer(m)
	}
}

// WheelZoom scales the viewer's picture by factor about the point
// at (x, y) in stage coordinates — a screenshot hook standing in for the
// pointer event.
func (v *AttachmentViewer) WheelZoom(factor, x, y float64) {
	if v.wheel != nil {
		v.wheel(factor, x, y)
	}
}

// OpenDownloaded is the click on a picture the bubble just downloaded to
// path: the same handler the inline widget runs.
func (cv *ConversationView) OpenDownloaded(m client.Message, path string) {
	cv.hooks().mediaOpener(m)(path)
}

// PopupHeaderMenu opens the conversation header's ⋮ popover.
func (cv *ConversationView) PopupHeaderMenu() {
	if cv.headerMenuPop != nil {
		cv.headerMenuPop.Popup()
	}
}

// ScrollThreadTo positions the thread at a fraction of its scroll range (0 =
// oldest, 1 = newest), so a screenshot can reach bubbles the auto-scroll to
// the newest message leaves off-screen.
func (cv *ConversationView) ScrollThreadTo(fraction float64) {
	set := func() {
		adj := cv.scroller.VAdjustment()
		span := adj.Upper() - adj.PageSize()
		if span < 0 {
			span = 0
		}
		adj.SetValue(adj.Lower() + span*fraction)
	}
	set()
	// The list view realizes rows lazily, so the adjustment's upper bound
	// grows for a few frames after the first jump; re-apply once it settles or
	// the shot lands short of the requested position.
	glib.TimeoutAdd(400, func() bool { set(); return false })
}

// OpenSearch reveals the in-chat search bar with query typed in.
func (cv *ConversationView) OpenSearch(query string) {
	cv.openSearchBar()
	cv.searchEntry.SetText(query)
}

// shotFor resolves the bubbleShot for the message at idx (negative counts
// from the end), nil if that bubble isn't realized.
func (cv *ConversationView) shotFor(idx int) *bubbleShot {
	if idx < 0 {
		idx += len(cv.msgs)
	}
	if idx < 0 || idx >= len(cv.msgs) || shotRegistry == nil {
		return nil
	}
	return shotRegistry[cv.msgs[idx].ID]
}

// MessageAt returns the loaded message at idx (negative counts from the end).
func (cv *ConversationView) MessageAt(idx int) (client.Message, bool) {
	if idx < 0 {
		idx += len(cv.msgs)
	}
	if idx < 0 || idx >= len(cv.msgs) {
		return client.Message{}, false
	}
	return cv.msgs[idx], true
}

// ShowHoverActions reveals the ⌄ and 🙂 of the bubble at idx.
func (cv *ConversationView) ShowHoverActions(idx int) {
	if s := cv.shotFor(idx); s != nil && s.affordances.chevron != nil {
		s.affordances.setVisible(true)
	}
}

// PopupMessageMenu opens the ⌄ menu (reactions over the rows) of the bubble
// at idx.
func (cv *ConversationView) PopupMessageMenu(idx int) {
	cv.ShowHoverActions(idx)
	if s := cv.shotFor(idx); s != nil && s.affordances.openMenu != nil {
		s.affordances.openMenu()
	}
}

// PopupReactPill opens the 🙂 quick-reaction row of the bubble at idx.
func (cv *ConversationView) PopupReactPill(idx int) {
	cv.ShowHoverActions(idx)
	if s := cv.shotFor(idx); s != nil && s.affordances.openReact != nil {
		s.affordances.openReact()
	}
}

// --- Composer ---

// PopEmoji opens the 🙂 emoji chooser.
func (c *Composer) PopEmoji() { c.showPicker("emoji") }

// PopPicker opens the GIF/Stickers picker on the named page ("gif"/"stickers").
func (c *Composer) PopPicker(page string) {
	c.pickerStack.SetVisibleChildName(page)
	c.pickerPopover.Popup()
}

// SetDraft types text into the entry (flips mic → send).
func (c *Composer) SetDraft(text string) { c.entry.SetText(text) }

// ShowRecordingUI paints the in-progress-recording state without touching a
// microphone (screenshot only; never call in production paths).
func (c *Composer) ShowRecordingUI() {
	c.recording = true
	c.enterRecordingUI()
	c.recordTime.SetLabel("0:07")
	c.sendBtn.SetVisible(false)
}

// ReactTo applies emoji to msg through the client so a reactions row renders.
func (c *Composer) ReactTo(msg client.Message, emoji string) error {
	return c.c.React(context.Background(), msg.ChatJID, msg.ID, emoji)
}

// ShowGroupInfo opens the group-info dialog for the open chat.
func (cv *ConversationView) ShowGroupInfo() {
	if cv.jid != "" {
		cv.showGroupInfo(chatByJID(cv.c, cv.jid))
	}
}

// ShowGroupInvite opens the invite-link card of the open group.
func (cv *ConversationView) ShowGroupInvite() {
	if cv.jid != "" {
		cv.showGroupInvite(chatByJID(cv.c, cv.jid))
	}
}

// ShowDisappearing opens the disappearing-messages chooser for the open chat.
func (cv *ConversationView) ShowDisappearing() {
	showDisappearingDialog(cv.window, cv.c, cv.jid, cv.disappearingTimer(cv.jid), nil)
}

// ShowContactInfo opens the contact-info card for the open chat.
func (cv *ConversationView) ShowContactInfo() {
	if cv.jid != "" {
		cv.showContactInfo(chatByJID(cv.c, cv.jid))
	}
}

// ShowJoinRequests opens the pending join requests for the open group.
func (cv *ConversationView) ShowJoinRequests() {
	if cv.jid != "" {
		showJoinRequestsDialog(cv.window, cv.c, cv.jid, func() { cv.refreshJoinBanner() })
	}
}

// LogButtonSizes prints the composer's 📎 and 🙂 allocations, for checking
// they are the mockup's 32px discs without a pointer to hover them.
func (c *Composer) LogButtonSizes() {
	log.Printf("chatot: composer buttons: attach %dx%d, emoji %dx%d",
		c.attachBtn.AllocatedWidth(), c.attachBtn.AllocatedHeight(),
		c.emojiBtn.AllocatedWidth(), c.emojiBtn.AllocatedHeight())
}

// ShowMute opens the mute-duration chooser for the open chat.
func (cv *ConversationView) ShowMute() {
	if cv.jid != "" {
		showMuteDialog(cv.window, cv.c, cv.jid, chatByJID(cv.c, cv.jid).Name)
	}
}

// ShowBlockConfirm opens the block confirmation for the open chat.
func (cv *ConversationView) ShowBlockConfirm(name string) {
	showBlockConfirmDialog(cv.window, cv.c, cv.jid, name)
}

// ShowTray opens the attachment tray on the given files (screenshot only).
func (c *Composer) ShowTray(paths []string) {
	if c.tray != nil {
		c.tray.Open(paths)
	}
}

// ShowMerged switches the sidebar into the merged "All accounts" list.
func (cl *ChatList) ShowMerged() { cl.setMerged(true) }

// PopupReactionPicker opens the ＋ full reaction grid for the bubble at idx.
func (cv *ConversationView) PopupReactionPicker(idx int) {
	if m, ok := cv.MessageAt(idx); ok {
		if s := cv.shotFor(idx); s != nil && s.affordances.chevron != nil {
			openReactionPicker(s.affordances.bubble, m, cv.hooks())
		}
	}
}

// ShowGroupName opens the group-name step with two people already picked.
func (cl *ChatList) ShowGroupName() {
	sel := newParticipantSelection()
	for i, ct := range newChatContacts(chatsOrEmpty(cl.c)) {
		if i == 2 {
			break
		}
		sel.Add(ct.JID, ct.Name)
	}
	cl.shotSel = sel
	cl.showGroupNameStep(sel, false)
}

// CreateGroupNow fills the open group-name step's field and clicks Create.
func (cl *ChatList) CreateGroupNow(name string) {
	if cl.shotName != nil && cl.shotCreate != nil {
		cl.shotName.SetText(name)
		cl.shotCreate.Activate()
	}
}

// ShowRelink opens the relink card for the first account.
func (cl *ChatList) ShowRelink(am *client.AccountManager) {
	if metas := am.Accounts(); len(metas) > 0 {
		showRelinkDialog(cl.window, am, metas[0].ID, cl.RefreshAccounts)
	}
}

// ShowMessageInfo opens the "Message info" card for m.
func ShowMessageInfo(parent *gtk.Window, m client.Message) { showMessageInfoDialog(parent, m) }

// ShowNewGroupPicked opens the people step with the first contact picked,
// so the chip row renders.
func (cl *ChatList) ShowNewGroupPicked() {
	cl.showNewGroupDialog()
	if cl.shotPickFirst != nil {
		cl.shotPickFirst()
	}
}

// PickGroupPhotoNow feeds path through the real picker's conversion and
// into the open name step's disc, bypassing only the file chooser.
func (cl *ChatList) PickGroupPhotoNow(path string) {
	jpeg, err := jpegForUpload(path)
	if err != nil {
		log.Printf("chatot: shot group photo %q: %v", path, err)
		return
	}
	if cl.shotPhoto != nil {
		cl.shotPhoto(jpeg)
	}
}

// ScrollThreadPx scrolls the thread to an absolute offset in CSS px once
// the auto-scroll to the newest message has settled, so a capture can land
// a bubble's edge on any fractional device pixel.
func (cv *ConversationView) ScrollThreadPx(px float64) {
	// Approached in 1px steps from 24px above, one per frame, the way a
	// wheel scroll lands: the renderer artefact under investigation shows
	// up while scrolling, not on a single jump.
	glib.TimeoutAdd(900, func() bool {
		adj := cv.scroller.VAdjustment()
		target := min(px, adj.Upper()-adj.PageSize())
		v := target - 24
		glib.TimeoutAdd(16, func() bool {
			v++
			adj.SetValue(min(v, target))
			return v < target
		})
		return false
	})
}

// --- the bottom tabs ---

// ShowStatus opens a poster's updates ("me" for our own) in the viewer.
func (cl *ChatList) ShowStatus(jid string) { cl.openStatus(jid) }

// PopupStatusRowMenu pops a poster row's context menu over the list.
func (cl *ChatList) PopupStatusRowMenu(jid string) {
	for _, p := range append(cl.statusFeed.Recent, cl.statusFeed.Viewed...) {
		if p.JID == jid {
			popupMenuAt(cl.statusList, cl.statusRowMenu(p), 60, 130)
			return
		}
	}
}

// PopupStatusViewMenu pops the viewer's ⋮.
func (cl *ChatList) PopupStatusViewMenu() {
	popupMenuBelow(cl.statusPane.menuBtn, cl.statusPane.menu())
}

// PopupMyStatusMenu pops the "My status" row's ⋯.
func (cl *ChatList) PopupMyStatusMenu() { popupMenuAt(cl.statusList, cl.myStatusMenu(), 60, 60) }

// PostTextStatusNow posts a text status synchronously and refreshes.
func (cl *ChatList) PostTextStatusNow(text string) {
	if err := cl.c.PostStatus(context.Background(), text); err != nil {
		log.Printf("chatot: post status: %v", err)
	}
	cl.refreshStatus()
}

// ShowTextStatus opens the text status dialog.
func (cl *ChatList) ShowTextStatus() { cl.showTextStatusDialog() }

// OpenChannelJID opens a channel from the followed list or the directory.
func (cl *ChatList) OpenChannelJID(jid string) {
	for _, n := range cl.newsletters {
		if n.ID == jid {
			cl.openChannel(n)
			return
		}
	}
	list, _ := cl.c.DiscoverNewsletters(context.Background(), "")
	for _, n := range list {
		if n.ID == jid {
			cl.openChannel(n)
			return
		}
	}
	cl.openChannelByID(jid)
}

// PopupChannelMenu pops the channel pane's ⋮.
func (cl *ChatList) PopupChannelMenu() {
	n := cl.channelPane.current
	popupMenuBelow(cl.channelPane.menuBtn, channelMenuItems(n, false, cl.channelActions(n)))
}

// PopupChannelRowMenu pops a followed channel row's context menu.
func (cl *ChatList) PopupChannelRowMenu(jid string) {
	for _, n := range cl.newsletters {
		if n.ID == jid {
			popupMenuAt(cl.channelsList, channelMenuItems(n, true, cl.channelActions(n)), 60, 40)
		}
	}
}

// PopupChannelReactionPicker opens the picker on the newest post.
func (cl *ChatList) PopupChannelReactionPicker() {
	if btn := cl.channelPane.firstReactAdd; btn != nil {
		pop := newReactionPickerPopover(btn, func(string) {})
		pop.Popup()
	}
}

// ReactChannelFirst clicks emoji on the newest post times over, to show
// the toggle (once adds and highlights, twice is back where it started).
func (cl *ChatList) ReactChannelFirst(emoji string, times int) {
	for i := 0; i < times; i++ {
		if cl.channelPane.firstReact != nil {
			cl.channelPane.firstReact(emoji)
		}
	}
}

// ConfirmUnfollow opens the unfollow confirmation for the channel jid.
func (cl *ChatList) ConfirmUnfollow(jid string) {
	for _, n := range cl.newsletters {
		if n.ID == jid {
			cl.confirmUnfollow(n)
			return
		}
	}
}

// PauseStatus holds the open status viewer's clock.
func (cl *ChatList) PauseStatus() { cl.statusPane.setUserPaused(true) }

// ShowChannelInfo / ShowShareChannel / ShowReportChannel open a channel's
// dialogs.
func (cl *ChatList) ShowChannelInfo(jid string)   { cl.showChannelInfoDialog(cl.channelForShot(jid)) }
func (cl *ChatList) ShowShareChannel(jid string)  { cl.showShareChannelDialog(cl.channelForShot(jid)) }
func (cl *ChatList) ShowReportChannel(jid string) { cl.showReportChannelDialog(cl.channelForShot(jid)) }

func (cl *ChatList) channelForShot(jid string) client.Newsletter {
	if cl.channelPane.current.ID == jid {
		return cl.channelPane.current
	}
	for _, n := range cl.newsletters {
		if n.ID == jid {
			return n
		}
	}
	return client.Newsletter{ID: jid, Name: jid}
}

// ShowDiscover opens Find channels with a query and category.
func (cl *ChatList) ShowDiscover(query, category string) {
	cl.openDiscover()
	if category != "" {
		cl.discoverCat = category
	}
	cl.discoverEntry.SetText(query)
	cl.refreshDiscover()
}

// ShowFollowLink opens the follow-with-a-link dialog.
func (cl *ChatList) ShowFollowLink() { cl.showFollowChannelDialog() }

// OpenCommunityJID opens a community.
func (cl *ChatList) OpenCommunityJID(jid string) { cl.openCommunityByID(jid) }

// PopupCommunityMenu pops the community pane's ⋮.
func (cl *ChatList) PopupCommunityMenu() {
	c := cl.communityPane.current
	popupMenuBelow(cl.communityPane.menuBtn, communityMenuItems(c, cl.communityActions(c)))
}

// PopupCommunityRowMenu pops a community row's context menu.
func (cl *ChatList) PopupCommunityRowMenu(jid string) {
	for _, c := range cl.communities {
		if c.JID == jid {
			popupMenuAt(cl.communitiesList, communityMenuItems(c, cl.communityActions(c)), 60, 40)
		}
	}
}

func (cl *ChatList) communityForShot(jid string) client.Community {
	for _, c := range cl.communities {
		if c.JID == jid {
			return c
		}
	}
	return cl.communityPane.current
}

// ShowCommunityInfo opens the info card, on the Members or "groups" tab.
func (cl *ChatList) ShowCommunityInfo(jid, tab string) {
	CommunityInfoInitialTab = tab
	cl.showCommunityInfoDialog(cl.communityForShot(jid))
}

// ShowCommunityInvite / ShowAddGroup open a community's dialogs.
func (cl *ChatList) ShowCommunityInvite(jid string) {
	cl.showCommunityInviteDialog(cl.communityForShot(jid))
}
func (cl *ChatList) ShowAddGroup(jid string) { cl.showAddGroupDialog(cl.communityForShot(jid)) }

// ShowJoinLink opens Join with a link with text typed in.
func (cl *ChatList) ShowJoinLink(text string) {
	JoinLinkInitialText = text
	cl.showJoinCommunityLinkDialog()
}

// ConfirmFirstJoin raises the join alert for the community's first
// not-joined group.
func (cl *ChatList) ConfirmFirstJoin(jid string) {
	c := cl.communityForShot(jid)
	for _, g := range c.Groups {
		if !g.Joined && !g.Announcement {
			cl.confirmJoinGroup(c, g)
			return
		}
	}
}

// ShowStatusViewersNow opens the viewer list for our own status.
func (cl *ChatList) ShowStatusViewersNow() {
	if m := cl.statusFeed.Mine; m != nil {
		cl.showStatusViewers(m.Items)
	}
}

// MuteStatusNow mutes jid's status updates as the row menu would.
func (cl *ChatList) MuteStatusNow(jid string) {
	if p := cl.statusFeed.poster(jid); p != nil {
		cl.setStatusMuted(*p, !p.Muted)
	}
}

// ShowImageViewerNow opens the full-size photo viewer for m using path as
// the picture (the fake's downloads are empty files, so a capture supplies
// a real image). Screenshot hook only.
func ShowImageViewerNow(win *gtk.Window, m client.Message, path string) {
	showImageViewer(win, path, m, nil)
}

// FilterByLabel selects the label filter for id — a dev/screenshot hook.
func (cl *ChatList) FilterByLabel(id string) {
	cl.setFilter(chatFilter{Kind: filterLabel, LabelID: id})
}

// OpenCommunityGroupAt is the click on the i-th group row of the open
// community's page.
func (cl *ChatList) OpenCommunityGroupAt(i int) {
	c := cl.communityPane.current
	if i < 0 || i >= len(c.Groups) {
		return
	}
	cl.openCommunityGroup(c.Groups[i])
}

// SearchList types text into the chat list's search box.
func (cl *ChatList) SearchList(text string) {
	cl.search.SetText(text)
	cl.search.GrabFocus()
	cl.search.SetPosition(-1)
}
