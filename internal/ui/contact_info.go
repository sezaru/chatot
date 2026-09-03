package ui

import (
	"context"
	"log"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// contactInfoSub is the dim line under the name on a contact-info card: the
// contact's phone number, recovered from the JID (chatot stores no separate
// number). A JID that isn't a phone user renders nothing rather than the raw
// server string.
func contactInfoSub(jid string) string {
	user, server, ok := strings.Cut(jid, "@")
	if !ok || server != "s.whatsapp.net" || user == "" {
		return ""
	}
	if i := strings.IndexByte(user, ':'); i >= 0 { // device-suffixed JID
		user = user[:i]
	}
	for _, r := range user {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return "+" + user
}

// contactInfoActions are the callbacks the info card's rows fire.
type contactInfoActions struct {
	// Muted flips the Mute row's wording to "Unmute notifications".
	Muted bool
	Mute  func()
	// DisappearingValue and MediaCount are the dim trailing values on their
	// rows ("Off", "15"); "" shows nothing.
	DisappearingValue string
	Disappearing      func()
	MediaCount        string
	Media             func()
	Block             func()
}

// muteInfoRowLabel is the card's Mute row wording for the chat's state.
func muteInfoRowLabel(muted bool) string {
	if muted {
		return "Unmute notifications"
	}
	return "Mute notifications"
}

// showContactInfoDialog opens the mockup's contact-info card for a 1:1 chat:
// a large avatar over the name and number, then a card of actions. This is
// the surface the chat ⋮ menu's "Contact info" row used to be insensitive
// for; groups keep their own richer group-info dialog.
func showContactInfoDialog(parent *gtk.Window, c client.Client, cache *avatarCache, jid, name string, blocked bool, a contactInfoActions) {
	dialog := newCardDialog()
	dialog.SetTitle(name)
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(360, -1)

	content := gtk.NewBox(gtk.OrientationVertical, 0)

	head := gtk.NewBox(gtk.OrientationVertical, 8)
	head.AddCSSClass("chatot-info-head")
	head.SetHAlign(gtk.AlignCenter)

	avatar := buildAvatar(c, cache, jid, contactInitial(name), 76)
	avatar.SetHAlign(gtk.AlignCenter)
	head.Append(avatar)

	title := gtk.NewLabel(name)
	title.AddCSSClass("chatot-info-name")
	head.Append(title)

	if sub := contactInfoSub(jid); sub != "" {
		s := gtk.NewLabel(sub)
		s.AddCSSClass("chatot-info-sub")
		head.Append(s)
	}
	content.Append(head)

	// Every row dismisses the card first, as in the mockup: the action either
	// shows its result behind it (mute, media page) or opens its own dialog.
	closing := func(f func()) func() {
		if f == nil {
			return nil
		}
		return func() {
			dialog.Close()
			f()
		}
	}
	muteIcon := "🔇"
	if a.Muted {
		muteIcon = "🔔"
	}
	body := dialogBody(8)
	card := newSettingsCard()
	card.Add(newIconRow(muteIcon, muteInfoRowLabel(a.Muted), "", false, closing(a.Mute)))
	card.Add(newIconRow("⏱", "Disappearing messages", a.DisappearingValue, false, closing(a.Disappearing)))
	card.Add(newIconRow("🖼", "Media, links and docs", a.MediaCount, false, closing(a.Media)))
	card.Add(newIconRow("🚫", blockChatMenuLabel(blocked), "", true, closing(a.Block)))
	body.Append(card)
	content.Append(body)

	dialog.SetChild(content)
	dialog.Present()
}

// deviceIcon picks the mockup's per-device glyph. chatot itself is the
// desktop entry; a phone is the primary device.
func deviceIcon(platform string) string {
	switch strings.ToLower(platform) {
	case "phone", "android", "ios":
		return "📱"
	case "web", "browser":
		return "🌐"
	}
	return "🖥"
}

// showLinkedDevicesDialog opens the mockup's "Linked devices" card.
//
// Only this device is listed: WhatsApp's other-device roster needs a server
// query chatot's client doesn't expose yet, and inventing rows would be
// worse than an honest single entry. The note under the card says so, and
// Unlink is the same Logout the ⋮ menu offers.
func showLinkedDevicesDialog(parent *gtk.Window, c client.Client, onUnlink func()) {
	dialog := newCardDialog()
	dialog.SetTitle("Linked devices")
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(400, -1)

	body := dialogBody(12)

	card := newSettingsCard()
	meta := c.OwnJID()
	if meta == "" {
		meta = "not linked"
	}
	card.Add(newDeviceRow(deviceIcon("desktop"), "chatot (this device)", meta, onUnlink))
	body.Append(card)

	note := gtk.NewLabel("chatot is one of the linked devices on this account. Unlinking the phone signs every device out. Other devices aren't listed yet.")
	note.SetXAlign(0)
	note.SetWrap(true)
	note.AddCSSClass("chatot-card-sub")
	body.Append(note)

	dialog.SetChild(body)
	dialog.Present()
}

// muteChatAsync toggles a chat's mute off the main loop, for the contact-info
// card's Mute row.
func muteChatAsync(c client.Client, jid string, mute bool) {
	go func() {
		if err := c.MuteChat(context.Background(), jid, mute); err != nil {
			log.Printf("chatot: mute chat failed: %v", err)
		}
	}()
}
