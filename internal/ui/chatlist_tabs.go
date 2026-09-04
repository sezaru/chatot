package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// The sidebar's bottom tabs (see tab_bar.go): switching, the header's back
// page, the ＋ menu per tab, the badges, and the content-pane seam main.go
// wires the tabs' panes through.

// selectTab makes id the active tab: sidebar page, ＋ menu, header, tab
// bar and content pane all follow. Called by the tab bar and by flows
// that land in another tab (a community group opening its chat).
func (cl *ChatList) selectTab(id string) {
	cl.discover = false
	cl.tab = id
	cl.tabBar.SetActive(id)
	cl.statusPane.stop()
	if cl.archiveT.Active() {
		// Deactivating refreshes the chat list; harmless on another tab.
		cl.archiveT.SetActive(false)
	}
	cl.showAccountHead()
	cl.modes.SetVisibleChildName(cl.tabPageName())
	cl.refreshPlusMenu()
	cl.updateTabBarVisibility()
	cl.currentChannel, cl.currentCommunity = "", ""
	if id == "chats" {
		cl.showPane("chat")
	} else {
		cl.setEmptyPane(id)
		cl.showPane("tabempty")
	}
	cl.refresh()
}

// SelectTab is selectTab for main.go and the screenshot hooks.
func (cl *ChatList) SelectTab(id string) { cl.selectTab(id) }

// Tab is the active tab id.
func (cl *ChatList) Tab() string { return cl.tab }

// tabPageName is the sidebar stack page the active tab shows.
func (cl *ChatList) tabPageName() string {
	switch {
	case cl.tab == "channels" && cl.discover:
		return "discover"
	case cl.tab == "status", cl.tab == "channels", cl.tab == "communities":
		return cl.tab
	default:
		return "chats"
	}
}

// refreshPlusMenu rebuilds the ＋ menu for the active tab.
func (cl *ChatList) refreshPlusMenu() {
	setMenuPopoverItems(cl.plusPopover, tabPlusMenuItems(cl.tab, tabPlusActions{
		plusMenuActions: plusMenuActions{
			NewChat:      cl.showNewChatDialog,
			NewGroup:     cl.showNewGroupDialog,
			NewCommunity: cl.showNewCommunityDialog,
		},
		JoinInvite:        cl.showJoinGroupDialog,
		PhotoStatus:       cl.postPhotoStatus,
		TextStatus:        cl.showTextStatusDialog,
		StatusPrivacy:     func() { cl.showPreferencesPage("privacy") },
		FindChannels:      cl.openDiscover,
		FollowLink:        cl.showFollowChannelDialog,
		CreateChannel:     func() { cl.toast("Channel creation isn't available yet") },
		JoinCommunityLink: cl.showJoinCommunityLinkDialog,
	}))
}

// showBackHead swaps the header's identity for "← title"; back runs on
// the arrow.
func (cl *ChatList) showBackHead(title string, back func()) {
	cl.archivedTitle.SetText(title)
	cl.backAction = back
	cl.identityStack.SetVisibleChildName("archived")
}

// showAccountHead restores the header's account button.
func (cl *ChatList) showAccountHead() {
	cl.backAction = nil
	cl.identityStack.SetVisibleChildName("account")
}

// updateTabBarVisibility hides the bar under the Find channels page and
// under an in-sidebar form on the Chats tab, as the mockup does.
func (cl *ChatList) updateTabBarVisibility() {
	if cl.tabBar == nil {
		return
	}
	formOpen := cl.modes.VisibleChildName() == sidebarFormPage
	cl.tabBar.SetVisible(!cl.discover && !(cl.tab == "chats" && formOpen))
}

// updateTabBadges refreshes the unread bubbles: chats with unread
// messages, contacts with unviewed status updates, and unread in
// community groups. Channels carry no unread state the client exposes.
// chats is the current chat list, shared with the rest of refresh.
func (cl *ChatList) updateTabBadges(chats []client.Chat) {
	if cl.tabBar == nil {
		return
	}
	unreadChats := 0
	for _, ch := range chats {
		if ch.UnreadCount > 0 && !ch.Archived {
			unreadChats++
		}
	}
	cl.tabBar.SetBadge("chats", unreadChats)
	if cl.tab != "status" {
		cl.tabBar.SetBadge("status", len(cl.loadStatusFeed().Recent))
	}
	if cl.tab != "communities" {
		total := 0
		for _, c := range cl.communities {
			total += communityUnread(c)
		}
		cl.tabBar.SetBadge("communities", total)
	}
}

// setEmptyPane fills the content pane's placeholder for tab.
func (cl *ChatList) setEmptyPane(tab string) {
	removeAllChildren(cl.emptyPane)
	glyph, title, text := tabEmptyCopy(tab)
	cl.emptyPane.Append(newTabEmptyState(glyph, title, text))
}

// showPane asks main.go to show one of the content pane's pages: "chat",
// "tabempty", "status", "channel" or "community".
func (cl *ChatList) showPane(name string) {
	if cl.onShowPane != nil {
		cl.onShowPane(name)
	}
}

// OnPaneRequested registers the content-pane switch.
func (cl *ChatList) OnPaneRequested(f func(name string)) { cl.onShowPane = f }

// OnForwardRequested registers the forward-to-a-chat dialog, used by the
// share dialogs and the status/channel forward rows.
func (cl *ChatList) OnForwardRequested(f func(msg client.Message)) { cl.onForward = f }

// SetToastOverlay wires the overlay the tabs' toasts show on.
func (cl *ChatList) SetToastOverlay(overlay *adw.ToastOverlay) { cl.toasts = overlay }

// toast shows a short plain toast.
func (cl *ChatList) toast(text string) { showToast(cl.toasts, text) }

// TabPanes are the content-pane pages the tabs own, for main.go to stack.
type TabPanes struct {
	Empty     gtk.Widgetter
	Status    gtk.Widgetter
	Channel   gtk.Widgetter
	Community gtk.Widgetter
}

// Panes returns the tabs' content panes.
func (cl *ChatList) Panes() TabPanes {
	return TabPanes{Empty: cl.emptyPane, Status: cl.statusPane, Channel: cl.channelPane, Community: cl.communityPane}
}

// PreloadCommunities fetches the roster once at start so the tab badge
// and the Add group dialog have it before the tab is first opened.
func (cl *ChatList) PreloadCommunities() { cl.refreshCommunities() }
