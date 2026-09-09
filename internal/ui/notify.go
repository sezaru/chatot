package ui

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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
// caption-less attachment, a "[kind]" placeholder. In a group the body
// leads with who wrote it ("Sender: text"), as the reaction toast names
// the reactor; sender is "" for a DM.
func messageNotification(chatName, sender string, msg client.Message) (title, body string) {
	body = msg.Text
	if body == "" && msg.Attachment != nil {
		body = attachmentPreview(*msg.Attachment)
	}
	if sender != "" {
		body = sender + ": " + body
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
	// markPath is the app mark written out for notifications ("" until the
	// first one, or when writing failed); see iconPath.
	markPath string
}

// avatarLookupTimeout bounds how long a notification waits for a chat's
// picture: a cached one answers at once, and a first fetch that is slow is
// not worth holding the toast for.
const avatarLookupTimeout = 3 * time.Second

// iconPath is the picture a notification for jid shows: the chat's own
// (contact or group), else the app mark, else "" (the shell then falls
// back to the desktop entry). Runs on the events goroutine, since a picture
// not cached yet is a round trip.
func (n *Notifier) iconPath(jid string) string {
	ctx, cancel := context.WithTimeout(context.Background(), avatarLookupTimeout)
	defer cancel()
	if path, err := n.c.Avatar(ctx, jid); err == nil && path != "" && fileExists(path) {
		return path
	}
	if n.markPath == "" {
		path, err := writeAppMarkIcon(filepath.Join(cacheDir(), "notify"))
		if err != nil {
			log.Printf("chatot: notification icon: %v", err)
			return ""
		}
		n.markPath = path
	}
	return n.markPath
}

// writeAppMarkIcon puts the app mark under dir as a PNG the notification
// daemon can read by path (GLib passes a file icon to the desktop as
// "image-path"; a bytes icon never reaches a freedesktop daemon) and
// returns its path. An up-to-date copy is left alone.
func writeAppMarkIcon(dir string) (string, error) {
	path := filepath.Join(dir, "app-mark.png")
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, appMarkPNG) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, appMarkPNG, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// fileIcon is the notification icon for the picture at path; nil for "".
func fileIcon(path string) gio.Iconner {
	if path == "" {
		return nil
	}
	return gio.NewFileIcon(gio.NewFileForPath(path))
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
	icon := n.iconPath(r.ChatJID)
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
		if ic := fileIcon(icon); ic != nil {
			notif.SetIcon(ic)
		}
		notif.SetDefaultActionAndTarget("app.open-chat", glib.NewVariantString(r.ChatJID))
		// Shares the chat's message id: the reaction is the chat's latest
		// news and replaces an older toast for it.
		n.send("chatot-chat-"+r.ChatJID, notif)
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
	// A group toast names the sender: the title is the group, and "hello"
	// alone said nothing about who wrote it.
	sender := ""
	if strings.HasSuffix(msg.ChatJID, "@g.us") && msg.FromJID != "" {
		sender = n.personName(msg.FromJID)
	}
	title, body := messageNotification(name, sender, msg)
	if !NotificationText {
		body = hiddenNotificationBody
	}
	title = accountPrefixedTitle(title, n.accountPrefix())
	icon := n.iconPath(msg.ChatJID)
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
		if ic := fileIcon(icon); ic != nil {
			notif.SetIcon(ic)
		}
		notif.SetDefaultActionAndTarget("app.open-chat", glib.NewVariantString(msg.ChatJID))
		// One id per chat: a newer message notification replaces rather than
		// stacks alongside an unread one for the same chat.
		n.send("chatot-chat-"+msg.ChatJID, notif)
		if NotificationSound {
			playNotificationSound()
		}
	})
}

func (n *Notifier) handleCall(call client.Call) {
	name, _ := n.chatInfo(call.ChatJID)
	title, body := callNotification(name, call.Video)
	title = accountPrefixedTitle(title, n.accountPrefix())
	icon := n.iconPath(call.ChatJID)
	glib.IdleAdd(func() {
		if !decideNotify(notifyInput{Kind: "call", ChatJID: call.ChatJID, Enabled: NotificationsEnabled}) {
			return
		}
		notif := gio.NewNotification(title)
		notif.SetBody(body)
		if ic := fileIcon(icon); ic != nil {
			notif.SetIcon(ic)
		}
		notif.SetPriority(gio.NotificationPriorityUrgent)
		notif.SetDefaultActionAndTarget("app.open-chat", glib.NewVariantString(call.ChatJID))
		notif.AddButtonWithTarget("Decline", "app.reject-call", glib.NewVariantString(encodeCallActionParam(call.ChatJID, call.CallID)))
		n.send("chatot-call-"+call.ChatJID, notif)
		if NotificationSound {
			playNotificationSound()
		}
	})
}

// send posts notif under id, first withdrawing whatever chatot still has
// under that id. GLib reuses the daemon's own id for a repeat send (the
// spec's replaces_id), and a shell that never signals NotificationClosed
// for a popup that quietly expired (quickshell, 2026-09) then takes the
// repeat as an update to a notification it has already put away: nothing
// pops up, so only a chat's first message ever notified. Withdrawing first
// makes every send a fresh popup there; on GNOME the old banner is simply
// swapped for the new one.
func (n *Notifier) send(id string, notif *gio.Notification) {
	n.app.WithdrawNotification(id)
	n.app.SendNotification(id, notif)
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
