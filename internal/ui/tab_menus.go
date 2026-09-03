package ui

import (
	"strings"

	"chatot/internal/client"
)

// The Status, Channels and Communities tabs' menus, as data (see
// menu_items.go): one pure function per popover, each mirroring a menuDef()
// branch of the interactive mockup so a drift from the design is a failing
// test.

// tabPlusActions are the ＋ menu's callbacks across all four tabs.
type tabPlusActions struct {
	plusMenuActions
	// The mockup's own extra: the only route into the join-by-invite flow.
	JoinInvite func()

	PhotoStatus   func()
	TextStatus    func()
	StatusPrivacy func()

	FindChannels  func()
	FollowLink    func()
	CreateChannel func()

	JoinCommunityLink func()
}

// tabPlusMenuItems is the ＋ menu for the active tab.
func tabPlusMenuItems(tab string, a tabPlusActions) []menuItem {
	switch tab {
	case "status":
		return []menuItem{
			{Icon: "📷", Label: "Photo status", OnActivate: a.PhotoStatus},
			{Icon: "✎", Label: "Text status", OnActivate: a.TextStatus},
			menuSeparator(),
			{Icon: "🔒", Label: "Status privacy…", OnActivate: a.StatusPrivacy},
		}
	case "channels":
		return []menuItem{
			{Icon: "⌕", Label: "Find channels", OnActivate: a.FindChannels},
			{Icon: "🔗", Label: "Follow with a link", OnActivate: a.FollowLink},
			menuSeparator(),
			{Icon: "📣", Label: "Create a channel", OnActivate: a.CreateChannel},
		}
	case "communities":
		return []menuItem{
			{Icon: "🏘", Label: "New community", OnActivate: a.NewCommunity},
			{Icon: "🔗", Label: "Join with a link", OnActivate: a.JoinCommunityLink},
		}
	default:
		return append(plusMenuItems(a.plusMenuActions),
			menuItem{Icon: "🔗", Label: "Join with invite link", OnActivate: a.JoinInvite})
	}
}

// statusRowMenuActions are a contact's status-row context menu callbacks.
type statusRowMenuActions struct {
	View, Reply, Mute, Hide func()
}

// statusRowMenuItems is the right-click menu on a contact's status row;
// name is the poster's display name (the mute row uses its first word).
func statusRowMenuItems(name string, muted bool, a statusRowMenuActions) []menuItem {
	first := strings.SplitN(strings.TrimSpace(name), " ", 2)[0]
	mute := "Mute " + first
	if muted {
		mute = "Unmute " + first
	}
	return []menuItem{
		{Icon: "👁", Label: "View updates", OnActivate: a.View},
		{Icon: "↩", Label: "Reply privately", OnActivate: a.Reply},
		menuSeparator(),
		{Icon: "🔇", Label: mute, OnActivate: a.Mute},
		{Icon: "🚫", Label: "Hide my status from them", Destructive: true, OnActivate: a.Hide},
	}
}

// myStatusMenuActions are the "My status" row's ⋯ menu callbacks.
type myStatusMenuActions struct {
	Viewers, Privacy, Delete func()
}

// myStatusMenuItems is the ⋯ beside "My status" once something is posted.
func myStatusMenuItems(a myStatusMenuActions) []menuItem {
	return []menuItem{
		{Icon: "👁", Label: "Who viewed my status", OnActivate: a.Viewers},
		{Icon: "🔒", Label: "Status privacy…", OnActivate: a.Privacy},
		menuSeparator(),
		{Icon: "🗑", Label: "Delete my status", Destructive: true, OnActivate: a.Delete},
	}
}

// statusViewMenuActions are the viewer's ⋮ menu callbacks.
type statusViewMenuActions struct {
	Reply, Forward, Mute, Report func()
}

// statusViewMenuItems is the ⋮ in the status viewer's top row.
func statusViewMenuItems(muted bool, a statusViewMenuActions) []menuItem {
	mute := "Mute this contact"
	if muted {
		mute = "Unmute this contact"
	}
	return []menuItem{
		{Icon: "↩", Label: "Reply privately", OnActivate: a.Reply},
		{Icon: "⤴", Label: "Forward this update", OnActivate: a.Forward},
		menuSeparator(),
		{Icon: "🔇", Label: mute, OnActivate: a.Mute},
		{Icon: "🚩", Label: "Report update", Destructive: true, OnActivate: a.Report},
	}
}

// channelMenuActions are a channel's menu callbacks, shared by the sidebar
// row's context menu and the channel pane's ⋮.
type channelMenuActions struct {
	Info, Share, Mute, Report, Unfollow func()
}

// channelMenuItems is a channel's menu. Only the sidebar row gets Unfollow:
// the pane header already carries a Follow/Following button.
func channelMenuItems(n client.Newsletter, inRow bool, a channelMenuActions) []menuItem {
	muteIcon, muteLabel := "🔇", "Mute updates"
	if n.Muted {
		muteIcon, muteLabel = "🔔", "Unmute updates"
	}
	items := []menuItem{
		{Icon: "ℹ", Label: "Channel info", OnActivate: a.Info},
		{Icon: "⤴", Label: "Share channel link", OnActivate: a.Share},
		menuSeparator(),
		{Icon: muteIcon, Label: muteLabel, OnActivate: a.Mute},
		{Icon: "🚩", Label: "Report channel", Destructive: true, OnActivate: a.Report},
	}
	if inRow {
		items = append(items, menuItem{Icon: "✕", Label: "Unfollow", Destructive: true, OnActivate: a.Unfollow})
	}
	return items
}

// communityMenuActions are a community's menu callbacks, shared by the
// sidebar row's context menu and the community pane's ⋮.
type communityMenuActions struct {
	Info, Invite, Mute, Leave func()
}

// communityMenuItems is a community's menu.
func communityMenuItems(c client.Community, a communityMenuActions) []menuItem {
	muteIcon, muteLabel := "🔇", "Mute announcements"
	if c.Muted {
		muteIcon, muteLabel = "🔔", "Unmute announcements"
	}
	return []menuItem{
		{Icon: "ℹ", Label: "Community info", OnActivate: a.Info},
		{Icon: "🔗", Label: "Invite link", OnActivate: a.Invite},
		menuSeparator(),
		{Icon: muteIcon, Label: muteLabel, OnActivate: a.Mute},
		{Icon: "⤓", Label: "Leave community", Destructive: true, OnActivate: a.Leave},
	}
}

// reportReasons are the channel report dialog's options, in the mockup's
// order.
var reportReasons = []string{
	"Spam or repetitive posts", "Scam or fraud", "Violence or hate speech",
	"Nudity or sexual content", "False information", "Something else",
}
