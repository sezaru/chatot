package ui

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// joinRequestBannerText is the pure formatter for the conversation banner's
// label: "" (hide the banner) for zero pending requests, otherwise a
// singular/plural count line.
func joinRequestBannerText(n int) string {
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return "1 person requested to join"
	default:
		return fmt.Sprintf("%d people requested to join", n)
	}
}

// sortJoinRequests orders pending requests oldest-first, ties broken by JID,
// for a stable review-dialog listing.
func sortJoinRequests(reqs []client.JoinRequest) []client.JoinRequest {
	out := make([]client.JoinRequest, len(reqs))
	copy(out, reqs)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].RequestedAt.Equal(out[j].RequestedAt) {
			return out[i].RequestedAt.Before(out[j].RequestedAt)
		}
		return out[i].JID < out[j].JID
	})
	return out
}

// joinRequestName is the row's title for a requester: the name chatot knows
// them by, else their phone number, else the raw JID.
func joinRequestName(jid string, names map[string]string) string {
	if name := names[jid]; name != "" {
		return name
	}
	if phone := contactInfoSub(jid); phone != "" {
		return phone
	}
	return jid
}

// joinRequestSub is the dim line under the name: the phone number when the
// title is a name, otherwise nothing (the title already is the number).
func joinRequestSub(jid string, names map[string]string) string {
	if names[jid] == "" {
		return ""
	}
	return contactInfoSub(jid)
}

// showJoinRequestsDialog opens the pending join requests for groupJID as one
// of the app's card dialogs: a bordered list of requester rows (avatar, name
// over number, Approve and Reject), or an empty state. onResolved is called
// after every successful action so the caller (the banner) can refresh its
// count. The mockup has no join-requests surface (its banner is a system
// line), so this follows its Linked devices / Send contact rows.
func showJoinRequestsDialog(parent *gtk.Window, c client.Client, groupJID string, onResolved func()) {
	dialog := newCardDialog()
	dialog.SetTitle("Join requests")
	dialog.SetDefaultSize(420, -1)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	body := dialogBody(10)
	content := gtk.NewBox(gtk.OrientationVertical, 0)
	body.Append(content)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	status.AddCSSClass("chatot-card-sub")
	body.Append(status)

	cache := newAvatarCache()
	names := map[string]string{}
	if chats, err := c.Chats(0); err == nil {
		for _, chat := range chats {
			names[chat.JID] = chat.Name
		}
	}

	var reload func()
	reload = func() {
		clearBox(content)
		status.SetText("")
		loading := gtk.NewLabel("Loading…")
		loading.AddCSSClass("chatot-card-sub")
		content.Append(loading)
		go func() {
			reqs, err := c.GroupJoinRequests(context.Background(), groupJID)
			glib.IdleAdd(func() {
				clearBox(content)
				if err != nil {
					content.Append(newPaneEmptyState("👥", "Couldn't load requests", "Check the connection and reopen this dialog."))
					return
				}
				if len(reqs) == 0 {
					content.Append(newPaneEmptyState("👥", "No pending requests", "People who ask to join this group appear here."))
					onResolved()
					return
				}
				card := newSettingsCard()
				for _, r := range sortJoinRequests(reqs) {
					card.Add(joinRequestRow(c, cache, groupJID, r, names, status, reload, onResolved))
				}
				content.Append(card)
			})
		}()
	}
	reload()
	dialog.SetChild(body)
	dialog.Present()
}

// joinRequestRow renders one pending requester: a 40px avatar, the name over
// the number, then Approve (green) and Reject (outlined red).
func joinRequestRow(c client.Client, cache *avatarCache, groupJID string, r client.JoinRequest, names map[string]string, status *gtk.Label, reload, onResolved func()) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	row.AddCSSClass("chatot-card-row")

	title := joinRequestName(r.JID, names)
	row.Append(buildAvatar(c, cache, r.JID, contactInitial(title), 40))

	col := gtk.NewBox(gtk.OrientationVertical, 2)
	col.SetHExpand(true)
	col.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(title)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.AddCSSClass("chatot-device-name")
	col.Append(name)
	if sub := joinRequestSub(r.JID, names); sub != "" {
		s := gtk.NewLabel(sub)
		s.SetXAlign(0)
		s.AddCSSClass("chatot-device-meta")
		col.Append(s)
	}
	row.Append(col)

	var approveBtn, rejectBtn *gtk.Button
	resolve := func(approve bool) {
		status.SetText("Working…")
		approveBtn.SetSensitive(false)
		rejectBtn.SetSensitive(false)
		go func() {
			err := c.ResolveGroupJoinRequest(context.Background(), groupJID, r.JID, approve)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: resolve join request for %s failed: %v", r.JID, err)
					status.SetText("Action failed, try again")
					approveBtn.SetSensitive(true)
					rejectBtn.SetSensitive(true)
					return
				}
				onResolved()
				reload()
			})
		}()
	}

	approveBtn = gtk.NewButtonWithLabel("Approve")
	approveBtn.AddCSSClass("chatot-primary-btn")
	approveBtn.AddCSSClass("chatot-row-btn")
	approveBtn.SetVAlign(gtk.AlignCenter)
	approveBtn.ConnectClicked(func() { resolve(true) })
	row.Append(approveBtn)

	rejectBtn = gtk.NewButtonWithLabel("Reject")
	rejectBtn.AddCSSClass("chatot-outline-btn")
	rejectBtn.AddCSSClass("chatot-outline-btn-danger")
	rejectBtn.SetVAlign(gtk.AlignCenter)
	rejectBtn.ConnectClicked(func() { resolve(false) })
	row.Append(rejectBtn)

	return row
}
