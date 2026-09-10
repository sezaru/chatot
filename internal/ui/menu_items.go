package ui

import "chatot/internal/client"

// This file holds one pure function per popover menu, each returning the
// menu's rows as data. The item sets, their order, icons, accelerators and
// destructive marking all come from the interactive mockup
// (mockup/Chatot Interactive.dc.html, menuDef()), so a mismatch with the
// design shows up as a failing test rather than a screenshot diff.
//
// Each takes an actions struct of callbacks; a nil callback renders the row
// insensitive, which is how surfaces that don't exist yet appear.

// plusMenuActions are the sidebar ＋ menu's callbacks.
type plusMenuActions struct {
	NewChat      func()
	NewGroup     func()
	NewCommunity func()
}

// plusMenuItems is the sidebar's ＋ menu.
func plusMenuItems(a plusMenuActions) []menuItem {
	return []menuItem{
		{Icon: "💬", Label: "New chat", OnActivate: a.NewChat},
		{Icon: "👥", Label: "New group", OnActivate: a.NewGroup},
		{Icon: "🏘", Label: "New community", OnActivate: a.NewCommunity},
	}
}

// appMenuActions are the sidebar ⋮ menu's callbacks.
type appMenuActions struct {
	Archived      func()
	Starred       func()
	Blocked       func()
	LinkedDevices func()
	Preferences   func()
	About         func()
	Unlink        func()
	// Relink takes Unlink's place once the account is signed out.
	Relink func()
	Quit   func()
}

// appMenuItems is the sidebar's ⋮ application menu: the mockup's rows, plus
// "Blocked contacts" under Starred — the one deliberate addition, since the
// design gives the blocked list no other home than a Preferences row.
//
// linked is whether the account still holds a WhatsApp session; it decides
// the last row (see unlinkOrRelinkItem).
func appMenuItems(linked bool, a appMenuActions) []menuItem {
	return []menuItem{
		{Icon: "📂", Label: "Archived", OnActivate: a.Archived},
		{Icon: "⭐", Label: "Starred messages", OnActivate: a.Starred},
		{Icon: "🚫", Label: "Blocked contacts", OnActivate: a.Blocked},
		menuSeparator(),
		{Icon: "🖥", Label: "Linked devices", OnActivate: a.LinkedDevices},
		{Icon: "⚙", Label: "Preferences", Accel: "Ctrl+,", OnActivate: a.Preferences},
		{Mark: true, Label: "About chatot", OnActivate: a.About},
		menuSeparator(),
		unlinkOrRelinkItem(linked, a),
		{Icon: "✕", Label: "Quit", Accel: "Ctrl+Q", OnActivate: a.Quit},
	}
}

// unlinkOrRelinkItem is the menu's last destructive row. Offering "Unlink
// this device" to an account that is already signed out is a dead end — the
// row that does nothing — so a signed-out account gets the action it
// actually needs in that slot instead.
func unlinkOrRelinkItem(linked bool, a appMenuActions) menuItem {
	if linked {
		return menuItem{Icon: "⏻", Label: "Unlink this device", Destructive: true, OnActivate: a.Unlink}
	}
	return menuItem{Icon: "🔗", Label: "Relink this device", OnActivate: a.Relink}
}

// chatMenuActions are the conversation header ⋮ menu's callbacks.
type chatMenuActions struct {
	Info         func()
	Search       func()
	Media        func()
	Mute         func()
	Pin          func()
	Disappearing func()
	// Wallpaper opens the chat's own wallpaper choice.
	Wallpaper func()
	Archive   func()
	Export    func()
	Clear     func()
	Block     func()
}

// chatMenuItems is the conversation header's ⋮ menu for chat. The mute, pin,
// archive and block rows read as the action they'd perform, so their wording
// flips with the chat's current state.
func chatMenuItems(chat client.Chat, blocked bool, a chatMenuActions) []menuItem {
	infoLabel := "Contact info"
	if chat.IsGroup {
		infoLabel = "Group info"
	}

	muteIcon, muteLabel := "🔇", "Mute notifications…"
	if chat.Muted {
		muteIcon, muteLabel = "🔔", "Unmute notifications"
	}

	pinLabel := "Pin chat"
	if chat.Pinned {
		pinLabel = "Unpin chat"
	}

	archiveLabel := "Archive chat"
	if chat.Archived {
		archiveLabel = "Unarchive chat"
	}

	return []menuItem{
		{Icon: "ℹ", Label: infoLabel, OnActivate: a.Info},
		{Icon: "⌕", Label: "Search in chat", Accel: "Ctrl+F", OnActivate: a.Search},
		{Icon: "🖼", Label: "Media, links and docs", OnActivate: a.Media},
		menuSeparator(),
		{Icon: muteIcon, Label: muteLabel, OnActivate: a.Mute},
		{Icon: "📌", Label: pinLabel, OnActivate: a.Pin},
		{Icon: "⏱", Label: "Disappearing messages…", OnActivate: a.Disappearing},
		// Not in the mockup: the chat's own wallpaper sits with the other
		// per-chat settings.
		{Icon: "🖼", Label: "Chat wallpaper…", OnActivate: a.Wallpaper},
		{Icon: "📂", Label: archiveLabel, OnActivate: a.Archive},
		menuSeparator(),
		{Icon: "⤓", Label: "Export chat…", OnActivate: a.Export},
		{Icon: "🗑", Label: "Clear chat…", Destructive: true, OnActivate: a.Clear},
		{Icon: "🚫", Label: blockChatMenuLabel(blocked), Destructive: true, OnActivate: a.Block},
	}
}

// blockChatMenuLabel is the chat menu's block row, which reads as the action
// it performs rather than the contact's current state. Blocked state lives in
// the client's cache (client.IsBlocked), not on client.Chat, so it is passed
// in rather than read off the chat.
func blockChatMenuLabel(blocked bool) string {
	if blocked {
		return "Unblock contact"
	}
	return "Block contact…"
}

// messageMenuActions are a bubble's ⋯ menu callbacks.
type messageMenuActions struct {
	Reply   func()
	Forward func()
	Star    func()
	Copy    func()
	Edit    func()
	Pin     func()
	Info    func()
	Delete  func()
	// AddToStickers files a sticker bubble's picture in the picker's library.
	AddToStickers func()
}

// messageMenuItems is a message bubble's ⋯ menu.
//
// "Edit message" and "Add to stickers" are the rows with no counterpart in
// the mockup: chatot can edit its own sent messages and save a sticker from
// a chat into the picker, and the design never drew those affordances, so
// they sit in the closest slots rather than being dropped.
func messageMenuItems(msg client.Message, a messageMenuActions) []menuItem {
	items := []menuItem{
		{Icon: "↩", Label: "Reply", OnActivate: a.Reply},
		{Icon: "↪", Label: "Forward", OnActivate: a.Forward},
	}
	if isStickerMessage(msg) {
		items = append(items, menuItem{Icon: "🙂", Label: "Add to stickers", OnActivate: a.AddToStickers})
	}
	items = append(items,
		menuItem{Icon: "⭐", Label: starMenuItemLabel(msg.Starred), OnActivate: a.Star},
		menuItem{Icon: "📋", Label: "Copy text", OnActivate: a.Copy},
	)
	if msg.FromMe {
		items = append(items, menuItem{Icon: "✎", Label: "Edit message", OnActivate: a.Edit})
	}
	return append(items,
		menuItem{Icon: "📌", Label: "Pin in chat", OnActivate: a.Pin},
		menuSeparator(),
		menuItem{Icon: "ℹ", Label: "Message info", OnActivate: a.Info},
		menuItem{Icon: "🗑", Label: "Delete message", Destructive: true, OnActivate: a.Delete},
	)
}

// isStickerMessage reports whether msg carries a sticker.
func isStickerMessage(msg client.Message) bool {
	return msg.Attachment != nil && msg.Attachment.Kind == "sticker"
}

// starMenuItemLabel is the message menu's star row for the message's current
// starred state.
func starMenuItemLabel(starred bool) string {
	if starred {
		return "Unstar message"
	}
	return "Star message"
}

// chatLabelState is one label offered in the chat-row menu's "Lists" section,
// with whether the chat currently carries it.
type chatLabelState struct {
	// ID is what ToggleLabel receives; Name is what the row displays.
	ID      string
	Name    string
	Applied bool
}

// chatRowMenuActions are the chat-list row context menu's callbacks.
type chatRowMenuActions struct {
	Pin     func()
	Mute    func()
	Unread  func()
	Archive func()
	Delete  func()
	NewList func()
	// ToggleLabel adds or removes the label with this ID on the chat.
	ToggleLabel func(id string)
}

// chatRowMenuItems is a chat row's right-click menu: the quick toggles, an
// inline checklist of the account's lists, then archive and delete.
//
// "Mark as unread"/"Mark as read" has no counterpart in the mockup; it is an
// existing chatot action with no other entry point, so it stays.
func chatRowMenuItems(chat client.Chat, labels []chatLabelState, a chatRowMenuActions) []menuItem {
	pinLabel := "Pin chat"
	if chat.Pinned {
		pinLabel = "Unpin chat"
	}

	muteIcon, muteLabel := "🔇", "Mute"
	if chat.Muted {
		muteIcon, muteLabel = "🔔", "Unmute"
	}

	archiveIcon, archiveLabel := "📂", "Archive chat"
	if chat.Archived {
		archiveIcon, archiveLabel = "📥", "Unarchive chat"
	}

	items := []menuItem{
		{Icon: "📌", Label: pinLabel, OnActivate: a.Pin},
		{Icon: muteIcon, Label: muteLabel, OnActivate: a.Mute},
		{Icon: "📨", Label: unreadMenuItemLabel(chat), OnActivate: a.Unread},
		// A caption, not an action: the checklist below belongs to it.
		{Icon: "🏷", Label: "Lists", Dim: true},
	}
	for _, l := range labels {
		icon := "☐"
		if l.Applied {
			icon = "☑"
		}
		id := l.ID
		var onActivate func()
		if a.ToggleLabel != nil {
			onActivate = func() { a.ToggleLabel(id) }
		}
		items = append(items, menuItem{Icon: icon, Label: l.Name, OnActivate: onActivate})
	}

	return append(items,
		menuItem{Icon: "＋", Label: "New list…", OnActivate: a.NewList},
		menuSeparator(),
		menuItem{Icon: archiveIcon, Label: archiveLabel, OnActivate: a.Archive},
		menuItem{Icon: "🗑", Label: "Delete chat", Destructive: true, OnActivate: a.Delete},
	)
}

// unreadMenuItemLabel is the row menu's read/unread toggle wording.
func unreadMenuItemLabel(chat client.Chat) string {
	if chat.UnreadCount > 0 {
		return "Mark as read"
	}
	return "Mark as unread"
}
