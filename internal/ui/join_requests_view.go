package ui

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

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

// showJoinRequestsDialog opens a modal listing groupJID's pending join
// requests, each with Approve/Reject buttons; onResolved is called after
// every successful action so the caller (the banner) can refresh its count.
func showJoinRequestsDialog(parent *gtk.Window, c client.Client, groupJID string, onResolved func()) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Join requests")
	dialog.SetDefaultSize(360, 420)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	content := gtk.NewBox(gtk.OrientationVertical, 6)
	content.SetMarginTop(12)
	content.SetMarginBottom(12)
	content.SetMarginStart(12)
	content.SetMarginEnd(12)
	scroller.SetChild(content)
	dialog.SetChild(scroller)

	status := gtk.NewLabel("")
	status.SetXAlign(0)

	var reload func()
	reload = func() {
		clearBox(content)
		content.Append(gtk.NewLabel("Loading…"))
		go func() {
			reqs, err := c.GroupJoinRequests(context.Background(), groupJID)
			glib.IdleAdd(func() {
				clearBox(content)
				if err != nil {
					content.Append(gtk.NewLabel("Couldn't load join requests"))
					return
				}
				if len(reqs) == 0 {
					content.Append(gtk.NewLabel("No pending requests"))
					onResolved()
					return
				}
				for _, r := range sortJoinRequests(reqs) {
					content.Append(joinRequestRow(c, groupJID, r, status, reload, onResolved))
				}
				content.Append(status)
			})
		}()
	}
	reload()
	dialog.Present()
}

// joinRequestRow renders one pending requester with Approve/Reject buttons.
func joinRequestRow(c client.Client, groupJID string, r client.JoinRequest, status *gtk.Label, reload, onResolved func()) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)

	name := gtk.NewLabel(r.JID)
	name.SetXAlign(0)
	name.SetHExpand(true)
	name.SetSelectable(true)
	row.Append(name)

	resolve := func(approve bool) {
		status.SetText("Working…")
		go func() {
			err := c.ResolveGroupJoinRequest(context.Background(), groupJID, r.JID, approve)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: resolve join request for %s failed: %v", r.JID, err)
					status.SetText("Action failed, try again")
					return
				}
				onResolved()
				reload()
			})
		}()
	}

	approveBtn := gtk.NewButtonWithLabel("Approve")
	approveBtn.AddCSSClass("flat")
	approveBtn.ConnectClicked(func() { resolve(true) })
	row.Append(approveBtn)

	rejectBtn := gtk.NewButtonWithLabel("Reject")
	rejectBtn.AddCSSClass("flat")
	rejectBtn.ConnectClicked(func() { resolve(false) })
	row.Append(rejectBtn)

	return row
}
