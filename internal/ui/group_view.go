package ui

import (
	"context"
	"fmt"
	"sort"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// participantBadge returns the role label to show next to a group
// participant: the group's owner outranks admin/super-admin flags.
func participantBadge(p client.GroupParticipant, ownerJID string) string {
	switch {
	case ownerJID != "" && p.JID == ownerJID:
		return "Owner"
	case p.IsSuperAdmin, p.IsAdmin:
		return "Admin"
	default:
		return ""
	}
}

// orderParticipants sorts a group's participants for display: owner first,
// then admins, then everyone else, ties broken by JID.
func orderParticipants(parts []client.GroupParticipant, ownerJID string) []client.GroupParticipant {
	out := make([]client.GroupParticipant, len(parts))
	copy(out, parts)
	rank := func(p client.GroupParticipant) int {
		switch {
		case ownerJID != "" && p.JID == ownerJID:
			return 0
		case p.IsAdmin, p.IsSuperAdmin:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return out[i].JID < out[j].JID
	})
	return out
}

// showGroupInfoDialog opens a read-only modal with jid's name, topic and
// participant list, fetched via c.GroupInfo in a goroutine.
func showGroupInfoDialog(parent *gtk.Window, c client.Client, jid string) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Group info")
	dialog.SetDefaultSize(360, 420)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 6)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	status := gtk.NewLabel("Loading…")
	status.SetXAlign(0)
	box.Append(status)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	list := gtk.NewBox(gtk.OrientationVertical, 4)
	scroller.SetChild(list)
	box.Append(scroller)

	dialog.SetChild(box)
	dialog.Present()

	go func() {
		info, err := c.GroupInfo(context.Background(), jid)
		glib.IdleAdd(func() {
			box.Remove(status)
			if err != nil || info == nil {
				box.Append(gtk.NewLabel("Couldn't load group info"))
				return
			}
			populateGroupInfo(list, *info)
		})
	}()
}

// populateGroupInfo fills list with the group's name/topic header and one
// row per participant, ordered by orderParticipants.
func populateGroupInfo(list *gtk.Box, info client.GroupInfo) {
	name := gtk.NewLabel(info.Name)
	name.SetXAlign(0)
	name.AddCSSClass("chatot-conv-title")
	list.Append(name)

	if info.Topic != "" {
		topic := gtk.NewLabel(info.Topic)
		topic.SetXAlign(0)
		topic.SetWrap(true)
		topic.AddCSSClass("chatot-conv-subtitle")
		list.Append(topic)
	}

	list.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	count := gtk.NewLabel(participantsCountLabel(len(info.Participants)))
	count.SetXAlign(0)
	list.Append(count)

	for _, p := range orderParticipants(info.Participants, info.OwnerJID) {
		row := gtk.NewLabel(participantRowText(p, info.OwnerJID))
		row.SetXAlign(0)
		row.SetSelectable(true)
		list.Append(row)
	}
}

func participantsCountLabel(n int) string {
	if n == 1 {
		return "1 participant"
	}
	return fmt.Sprintf("%d participants", n)
}

// participantRowText renders one participant's line: jid, plus a role badge
// in parens when one applies.
func participantRowText(p client.GroupParticipant, ownerJID string) string {
	badge := participantBadge(p, ownerJID)
	if badge == "" {
		return p.JID
	}
	return p.JID + " (" + badge + ")"
}
