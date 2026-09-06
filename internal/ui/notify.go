package ui

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"

	"chatot/internal/client"
)

// callActionSep joins chatJID and callID in the "app.reject-call" notification
// action's string parameter. 0x1f (unit separator) can't appear in a JID or a
// whatsmeow call ID, so it's a safe, unambiguous delimiter.
const callActionSep = "\x1f"

// encodeCallActionParam packs chatJID and callID into the single string
// parameter a GAction can carry as a notification-button target.
func encodeCallActionParam(chatJID, callID string) string {
	return chatJID + callActionSep + callID
}

// DecodeCallActionParam reverses encodeCallActionParam, unpacking the
// "app.reject-call" notification action's string parameter. ok is false if
// param isn't well-formed (missing separator).
func DecodeCallActionParam(param string) (chatJID, callID string, ok bool) {
	chatJID, callID, ok = strings.Cut(param, callActionSep)
	return
}

// NotificationsEnabled globally gates desktop notifications. Default true;
// no settings UI yet (mirrors ui.SendReadReceipts).
var NotificationsEnabled = true

// NotificationsPerAccount, when true, prefixes a toast title with the active
// account's label once more than one account is linked. Set from settings.
var NotificationsPerAccount = true

// accountPrefixedTitle prepends "label · " to title when label is non-empty,
// producing e.g. "Work · Sam Okafor"; an empty label leaves title unchanged.
func accountPrefixedTitle(title, label string) string {
	if label == "" {
		return title
	}
	return label + " · " + title
}

// notifyInput is the GTK-free input to decideNotify, gathered by Notifier
// from the event, the chat's stored state and the window's live focus.
type notifyInput struct {
	Kind       string // "message" | "call"
	FromMe     bool
	ChatJID    string
	Muted      bool
	Enabled    bool
	AppFocused bool
	OpenJID    string // currently-open chat JID, "" if none
}

// decideNotify is the pure notification policy, kept free of GTK/gio so it's
// unit-testable without a display.
//
// Messages: suppressed for from-me, disabled, muted chats, and for the chat
// that's currently open while the app window is focused (the user is already
// looking at it) — but not merely open-while-unfocused, since the user could
// be looking at another window.
//
// Calls: an incoming call always rings while notifications are enabled,
// regardless of mute or which chat is open/focused — a call is a real-time
// interruption the user hasn't already seen, unlike a message that's already
// rendered in an open, focused conversation.
func decideNotify(in notifyInput) bool {
	if in.FromMe || !in.Enabled {
		return false
	}
	switch in.Kind {
	case "message":
		if in.Muted {
			return false
		}
		if in.AppFocused && in.OpenJID == in.ChatJID {
			return false
		}
		return true
	case "call":
		return true
	default:
		return false
	}
}

// NotificationText mirrors settings.NotificationText: whether a message
// notification carries the message itself or only says one arrived.
var NotificationText = true

// hiddenNotificationBody stands in for the message when NotificationText
// is off.
const hiddenNotificationBody = "New message"

// messageNotification builds the title/body for a message notification:
// title is the chat's display name, body is the text preview or, for a
// caption-less attachment, a "[kind]" placeholder.
func messageNotification(chatName string, msg client.Message) (title, body string) {
	body = msg.Text
	if body == "" && msg.Attachment != nil {
		body = attachmentPreview(*msg.Attachment)
	}
	return chatName, body
}

// callNotification builds the title/body for an incoming-call notification.
func callNotification(callerName string, video bool) (title, body string) {
	if video {
		return "Incoming video call", callerName
	}
	return "Incoming call", callerName
}

// Notifier watches client.Events() and raises desktop notifications for
// messages and calls via decideNotify. Focus/open-chat state isn't owned by
// any Client implementation, so it's supplied by main.go as a callback
// rather than growing the Client interface; muted/display-name state is
// read from the client's own Chats() snapshot instead of adding a dedicated
// store lookup.
type Notifier struct {
	c       client.Client
	events  <-chan client.Event
	app     *gio.Application
	focused func() (focused bool, openJID string)
	// account reports the active account's label and the number of linked
	// accounts, for the per-account title prefix. Nil in the single-client
	// (non-manager) case.
	account func() (label string, count int)
}

// NewNotifier starts watching c.Events() in its own goroutine. focused
// reports the app window's live activation state and the JID of the
// currently-open chat ("" if none). account (may be nil) reports the active
// account's label and count for the per-account title prefix.
func NewNotifier(c client.Client, app *gio.Application, focused func() (bool, string), account func() (string, int)) *Notifier {
	n := &Notifier{c: c, events: c.Events(), app: app, focused: focused, account: account}
	go n.watchEvents()
	return n
}

// accountPrefix returns the active account's label to prefix a toast title
// with, or "" when the prefix is disabled, there's no account accessor, or only
// one account is linked.
//
// TODO: background-account notifications — the manager only proxies the active
// account's events, so today only the active account can notify; the prefix
// therefore always names the active account.
func (n *Notifier) accountPrefix() string {
	if !NotificationsPerAccount || n.account == nil {
		return ""
	}
	label, count := n.account()
	if count <= 1 {
		return ""
	}
	return label
}

func (n *Notifier) watchEvents() {
	for ev := range n.events {
		switch ev.Kind {
		case client.EventMessage:
			// Catch-up traffic (history replay after a link or reconnect) is
			// old news: it fills the store but never rings.
			if ev.Message != nil && !ev.Synced {
				n.handleMessage(*ev.Message)
			}
		case client.EventReaction:
			// Someone reacting to our message is news the way a message is
			// (WhatsApp notifies it); a replayed one, a cleared one, or an
			// internal refresh (no reactor) is not.
			if r := ev.Reaction; r != nil && !ev.Synced && r.TargetFromMe && r.Emoji != "" && r.ReactorJID != "" {
				n.handleReaction(*r)
			}
		case client.EventCall:
			if ev.Call == nil {
				continue
			}
			switch {
			case ev.Call.Offer && !ev.Synced:
				n.handleCall(*ev.Call)
			case !ev.Call.Offer:
				// The call settled (picked up on the phone, timed out,
				// declined): whatever was ringing here comes down.
				jid := ev.Call.ChatJID
				glib.IdleAdd(func() { n.app.WithdrawNotification("chatot-call-" + jid) })
			}
		}
	}
}

// hiddenReactionBody stands in for the reaction when NotificationText is
// off.
const hiddenReactionBody = "New reaction"

// reactionNotification builds the title/body for someone reacting to one of
// our messages: the chat's name over `Reacted 👍 to "..."`, the reactor
// named first in a group (reactor is "" for a DM).
func reactionNotification(chatName, reactor, emoji, target string) (title, body string) {
	return chatName, client.ReactionText(reactor, emoji, target)
}

// handleReaction runs on the background events goroutine, like
// handleMessage; the same message policy applies (muted chats and the open,
// focused chat stay quiet).
func (n *Notifier) handleReaction(r client.Reaction) {
	name, muted := n.chatInfo(r.ChatJID)
	reactor := ""
	if strings.HasSuffix(r.ChatJID, "@g.us") {
		reactor = n.personName(r.ReactorJID)
	}
	title, body := reactionNotification(name, reactor, r.Emoji, r.TargetPreview)
	if !NotificationText {
		body = hiddenReactionBody
	}
	title = accountPrefixedTitle(title, n.accountPrefix())
	glib.IdleAdd(func() {
		focused, openJID := n.focused()
		if !decideNotify(notifyInput{
			Kind: "message", ChatJID: r.ChatJID,
			Muted: muted, Enabled: NotificationsEnabled,
			AppFocused: focused, OpenJID: openJID,
		}) {
			return
		}
		notif := gio.NewNotification(title)
		notif.SetBody(body)
		notif.SetDefaultActionAndTarget("app.open-chat", glib.NewVariantString(r.ChatJID))
		// Shares the chat's message id: the reaction is the chat's latest
		// news and replaces an older toast for it.
		n.app.SendNotification("chatot-chat-"+r.ChatJID, notif)
		if NotificationSound {
			playNotificationSound()
		}
	})
}

// personName is the reactor's display name: the contact name chatot knows,
// else the phone number.
func (n *Notifier) personName(jid string) string {
	if name := n.c.ContactName(jid); name != "" {
		return name
	}
	if strings.HasSuffix(jid, "@lid") {
		return nonADJID(jid)
	}
	return phoneFromJID(jid)
}

// handleMessage runs on the background events goroutine. chatInfo (a plain
// store read) and messageNotification (pure) are safe off the main loop, but
// the focus/open-chat read and decideNotify are deferred into glib.IdleAdd
// alongside the send: n.focused() touches live GTK state (win.IsActive) and
// cv.jid, both of which must only be read on the main loop.
func (n *Notifier) handleMessage(msg client.Message) {
	name, muted := n.chatInfo(msg.ChatJID)
	title, body := messageNotification(name, msg)
	if !NotificationText {
		body = hiddenNotificationBody
	}
	title = accountPrefixedTitle(title, n.accountPrefix())
	glib.IdleAdd(func() {
		focused, openJID := n.focused()
		if !decideNotify(notifyInput{
			Kind: "message", FromMe: msg.FromMe, ChatJID: msg.ChatJID,
			Muted: muted, Enabled: NotificationsEnabled,
			AppFocused: focused, OpenJID: openJID,
		}) {
			return
		}
		notif := gio.NewNotification(title)
		notif.SetBody(body)
		notif.SetDefaultActionAndTarget("app.open-chat", glib.NewVariantString(msg.ChatJID))
		// One id per chat: a newer message notification replaces rather than
		// stacks alongside an unread one for the same chat.
		n.app.SendNotification("chatot-chat-"+msg.ChatJID, notif)
		if NotificationSound {
			playNotificationSound()
		}
	})
}

func (n *Notifier) handleCall(call client.Call) {
	name, _ := n.chatInfo(call.ChatJID)
	title, body := callNotification(name, call.Video)
	title = accountPrefixedTitle(title, n.accountPrefix())
	glib.IdleAdd(func() {
		if !decideNotify(notifyInput{Kind: "call", ChatJID: call.ChatJID, Enabled: NotificationsEnabled}) {
			return
		}
		notif := gio.NewNotification(title)
		notif.SetBody(body)
		notif.SetPriority(gio.NotificationPriorityUrgent)
		notif.SetDefaultActionAndTarget("app.open-chat", glib.NewVariantString(call.ChatJID))
		notif.AddButtonWithTarget("Decline", "app.reject-call", glib.NewVariantString(encodeCallActionParam(call.ChatJID, call.CallID)))
		n.app.SendNotification("chatot-call-"+call.ChatJID, notif)
		if NotificationSound {
			playNotificationSound()
		}
	})
}

// chatInfo resolves jid's display name and muted flag from the client's
// current chat snapshot, falling back to the raw JID if the chat isn't
// (yet) in it.
func (n *Notifier) chatInfo(jid string) (name string, muted bool) {
	chats, err := n.c.Chats(0)
	if err != nil {
		return jid, false
	}
	for _, c := range chats {
		if c.JID == jid {
			return c.Name, c.Muted
		}
	}
	return jid, false
}
